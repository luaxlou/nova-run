#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION_FILE="${ROOT}/VERSION"
OUTPUT_DIR="${NOVA_PLUGIN_OUTPUT_DIR:-${ROOT}/dist}"

if [ ! -f "$VERSION_FILE" ]; then
  echo "Missing product version: ${VERSION_FILE}" >&2
  exit 1
fi

VERSION="$(sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' "$VERSION_FILE")"
if [ -z "$VERSION" ]; then
  echo "Product version is empty: ${VERSION_FILE}" >&2
  exit 1
fi

for command_name in python3; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "Plugin packaging requires ${command_name}." >&2
    exit 1
  fi
done

python3 - "$ROOT" "$VERSION" <<'PY'
import json
import pathlib
import re
import sys

root = pathlib.Path(sys.argv[1])
expected = sys.argv[2]

if re.fullmatch(r"(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)(?:[-+][0-9A-Za-z.-]+)?", expected) is None:
    raise SystemExit(f"VERSION must be a semantic version, got {expected!r}")


def read_json(relative_path):
    path = root / relative_path
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError:
        raise SystemExit(f"Missing Plugin package file: {relative_path}")
    except json.JSONDecodeError as error:
        raise SystemExit(f"Invalid JSON in {relative_path}: {error}")


def require_version(relative_path, actual, field="version"):
    if actual != expected:
        raise SystemExit(
            f"Version drift in {relative_path} ({field}): expected {expected!r}, got {actual!r}"
        )


for manifest_path in (".codex-plugin/plugin.json", ".claude-plugin/plugin.json"):
    manifest = read_json(manifest_path)
    require_version(manifest_path, manifest.get("version"))
    if manifest.get("name") != "nova-run" or manifest.get("skills") != "./skills/":
        raise SystemExit(f"{manifest_path} must expose nova-run from ./skills/")

codex_manifest = read_json(".codex-plugin/plugin.json")
capabilities = codex_manifest.get("interface", {}).get("capabilities")
if not isinstance(capabilities, list) or "skills" not in capabilities:
    raise SystemExit(".codex-plugin/plugin.json must declare the skills capability")

marketplace_path = ".claude-plugin/marketplace.json"
marketplace = read_json(marketplace_path)
require_version(marketplace_path, marketplace.get("version"))
plugins = marketplace.get("plugins")
if marketplace.get("name") != "nova-run" or not isinstance(plugins, list) or len(plugins) != 1:
    raise SystemExit(f"{marketplace_path} must declare exactly one nova-run Plugin")
for index, plugin in enumerate(plugins):
    if not isinstance(plugin, dict):
        raise SystemExit(f"{marketplace_path} plugins[{index}] must be an object")
    require_version(marketplace_path, plugin.get("version"), f"plugins[{index}].version")
    if plugin.get("name") != "nova-run" or plugin.get("source") != "./":
        raise SystemExit(f"{marketplace_path} plugins[{index}] must resolve nova-run from ./")

# The portable Agents marketplace intentionally has no release version, but it
# is required in every Plugin archive and must remain valid JSON.
agents_marketplace_path = ".agents/plugins/marketplace.json"
agents_marketplace = read_json(agents_marketplace_path)
agents_plugins = agents_marketplace.get("plugins")
if agents_marketplace.get("name") != "nova-run" or not isinstance(agents_plugins, list) or len(agents_plugins) != 1:
    raise SystemExit(f"{agents_marketplace_path} must declare exactly one nova-run Plugin")
agents_plugin = agents_plugins[0]
agents_source = agents_plugin.get("source") if isinstance(agents_plugin, dict) else None
if (
    not isinstance(agents_plugin, dict)
    or agents_plugin.get("name") != "nova-run"
    or not isinstance(agents_source, dict)
    or agents_source.get("source") != "local"
    or agents_source.get("path") != "./"
):
    raise SystemExit(f"{agents_marketplace_path} must resolve nova-run from the repository root")

skill_root = root / "skills"
skill_names = sorted(path.name for path in skill_root.iterdir() if path.is_dir()) if skill_root.is_dir() else []
expected_skills = ["nova-run-doctor", "nova-run-init"]
if skill_names != expected_skills:
    raise SystemExit(f"Plugin must expose exactly these Skills: {', '.join(expected_skills)}")
if (root / ".agents/skills").exists():
    raise SystemExit("Keep product Skills in skills/; .agents/skills would create duplicate discovery")

for skill_name in ("nova-run-init", "nova-run-doctor"):
    relative_path = f"skills/{skill_name}/SKILL.md"
    path = root / relative_path
    try:
        source = path.read_text(encoding="utf-8")
    except FileNotFoundError:
        raise SystemExit(f"Missing Plugin package file: {relative_path}")
    frontmatter = source.split("---", 2)
    if len(frontmatter) < 3 or frontmatter[0].strip():
        raise SystemExit(f"Invalid YAML frontmatter in {relative_path}")
    match = re.search(
        r'^\s+novaRunRelease:\s*["\']?([^"\'\s]+)["\']?\s*$',
        frontmatter[1],
        flags=re.MULTILINE,
    )
    actual = match.group(1) if match else None
    require_version(relative_path, actual, "metadata.novaRunRelease")
    name_match = re.search(r"^name:\s*([^\s]+)\s*$", frontmatter[1], flags=re.MULTILINE)
    if name_match is None or name_match.group(1) != skill_name:
        raise SystemExit(f"Skill name in {relative_path} must be {skill_name!r}")
    agent_metadata = root / "skills" / skill_name / "agents" / "openai.yaml"
    if not agent_metadata.is_file():
        raise SystemExit(f"Missing Plugin package file: skills/{skill_name}/agents/openai.yaml")

documentation = root / "docs/agent-plugin.md"
if not documentation.is_file():
    raise SystemExit("Missing Plugin package file: docs/agent-plugin.md")
PY

PACKAGE_PATHS=(
  ".codex-plugin"
  ".claude-plugin"
  ".agents/plugins/marketplace.json"
  "skills"
  "docs/agent-plugin.md"
)

mkdir -p "$OUTPUT_DIR"
ARCHIVE="${OUTPUT_DIR}/nova-run-plugin-${VERSION}.tgz"
TEMP_ARCHIVE="${ARCHIVE}.tmp.$$"
trap 'rm -f "$TEMP_ARCHIVE"' EXIT

python3 - "$ROOT" "$TEMP_ARCHIVE" "${PACKAGE_PATHS[@]}" <<'PY'
import gzip
import pathlib
import sys
import tarfile

root = pathlib.Path(sys.argv[1])
destination = pathlib.Path(sys.argv[2])
package_paths = [pathlib.PurePosixPath(value) for value in sys.argv[3:]]

members = {}
for package_path in package_paths:
    source = root / pathlib.Path(package_path)
    if not source.exists() or source.is_symlink():
        raise SystemExit(f"Plugin package path must exist and cannot be a symlink: {package_path}")
    members[package_path.as_posix()] = source
    if source.is_dir():
        for child in source.rglob("*"):
            relative = pathlib.PurePosixPath(child.relative_to(root).as_posix())
            if child.is_symlink():
                raise SystemExit(f"Plugin package cannot contain symlinks: {relative}")
            if not child.is_dir() and not child.is_file():
                raise SystemExit(f"Plugin package contains an unsupported file: {relative}")
            members[relative.as_posix()] = child

with destination.open("wb") as raw:
    with gzip.GzipFile(filename="", mode="wb", fileobj=raw, compresslevel=9, mtime=0) as compressed:
        with tarfile.open(fileobj=compressed, mode="w", format=tarfile.PAX_FORMAT) as archive:
            for relative in sorted(members):
                source = members[relative]
                info = archive.gettarinfo(str(source), arcname=relative)
                info.uid = 0
                info.gid = 0
                info.uname = ""
                info.gname = ""
                info.mtime = 0
                info.mode = 0o755 if source.is_dir() or source.stat().st_mode & 0o111 else 0o644
                info.pax_headers = {}
                if source.is_file():
                    with source.open("rb") as content:
                        archive.addfile(info, content)
                else:
                    archive.addfile(info)
PY
mv "$TEMP_ARCHIVE" "$ARCHIVE"
trap - EXIT

echo "Nova Run Plugin packaged at ${ARCHIVE}"
