package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInstallCLIInstallsDownloadedBinary(t *testing.T) {
	env := newInstallerEnv(t, "nova-v1")
	output, err := env.run()
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, output)
	}
	if got := env.installedContent(t); got != "nova-v1" {
		t.Fatalf("installed content=%q", got)
	}
}

func TestInstallCLISkipsMatchingPinnedVersionBeforeDownload(t *testing.T) {
	env := newInstallerEnv(t, "nova-v1")
	wantTime := time.Date(2000, 1, 2, 3, 4, 5, 0, time.UTC)
	env.version = "v1"
	env.failDownload = true
	env.writeVersionedNova(t, "1")
	if err := os.Chtimes(env.installedPath, wantTime, wantTime); err != nil {
		t.Fatal(err)
	}

	output, err := env.run()
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, output)
	}
	info, err := os.Stat(env.installedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(wantTime) {
		t.Fatalf("identical binary was overwritten: modtime=%s", info.ModTime())
	}
	if !strings.Contains(output, "already the latest") || !strings.Contains(output, "--force") {
		t.Fatalf("output=%q", output)
	}
}

func TestInstallCLISkipsLatestVersionBeforeBinaryDownload(t *testing.T) {
	env := newInstallerEnv(t, "new-binary")
	env.version = ""
	env.latestVersion = "v1.2.3"
	env.failDownload = true
	env.writeVersionedNova(t, "1.2.3")

	output, err := env.run()
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "already the latest") {
		t.Fatalf("output=%q", output)
	}
}

func TestInstallCLIForceFlagsOverwriteIdenticalExecutable(t *testing.T) {
	for _, forceFlag := range []string{"--force", "-f"} {
		t.Run(forceFlag, func(t *testing.T) {
			env := newInstallerEnv(t, "nova-v1")
			oldTime := time.Date(2000, 1, 2, 3, 4, 5, 0, time.UTC)
			env.version = "v1"
			env.writeVersionedNova(t, "1")
			if err := os.Chtimes(env.installedPath, oldTime, oldTime); err != nil {
				t.Fatal(err)
			}

			output, err := env.run(forceFlag)
			if err != nil {
				t.Fatalf("force install failed: %v\n%s", err, output)
			}
			info, err := os.Stat(env.installedPath)
			if err != nil {
				t.Fatal(err)
			}
			if info.ModTime().Equal(oldTime) {
				t.Fatalf("%s did not overwrite the identical binary", forceFlag)
			}
		})
	}
}

func TestInstallCLIRejectsUnknownArguments(t *testing.T) {
	env := newInstallerEnv(t, "nova-v1")
	output, err := env.run("--unknown")
	if err == nil {
		t.Fatalf("unknown argument unexpectedly succeeded: %s", output)
	}
}

type installerEnv struct {
	t              *testing.T
	scriptPath     string
	installDir     string
	installedPath  string
	downloadPath   string
	fakeCommandDir string
	version        string
	latestVersion  string
	failDownload   bool
}

func newInstallerEnv(t *testing.T, downloaded string) *installerEnv {
	t.Helper()
	root := t.TempDir()
	scriptPath, err := filepath.Abs("install-cli.sh")
	if err != nil {
		t.Fatal(err)
	}
	installDir := filepath.Join(root, "bin")
	fakeCommandDir := filepath.Join(root, "fake-bin")
	if err := os.MkdirAll(fakeCommandDir, 0o755); err != nil {
		t.Fatal(err)
	}
	downloadPath := filepath.Join(root, "downloaded-nova")
	if err := os.WriteFile(downloadPath, []byte(downloaded), 0o755); err != nil {
		t.Fatal(err)
	}
	fakeCurl := `#!/bin/sh
output=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    shift
    output="$1"
  fi
  shift
done
if [ -z "$output" ]; then
  printf '{"tag_name":"%s"}\n' "$NOVA_TEST_LATEST_VERSION"
  exit 0
fi
if [ "$NOVA_TEST_FAIL_DOWNLOAD" = "1" ]; then
  echo "binary download must not happen" >&2
  exit 42
fi
cp "$NOVA_TEST_DOWNLOAD" "$output"
`
	if err := os.WriteFile(filepath.Join(fakeCommandDir, "curl"), []byte(fakeCurl), 0o755); err != nil {
		t.Fatal(err)
	}
	return &installerEnv{
		t: t, scriptPath: scriptPath, installDir: installDir,
		installedPath: filepath.Join(installDir, "nova"),
		downloadPath:  downloadPath, fakeCommandDir: fakeCommandDir,
		version: "v1", latestVersion: "v1",
	}
}

func (e *installerEnv) run(args ...string) (string, error) {
	e.t.Helper()
	cmd := exec.Command("bash", append([]string{e.scriptPath}, args...)...)
	cmd.Env = append(os.Environ(),
		"NOVA_INSTALL_DIR="+e.installDir,
		"NOVA_VERSION="+e.version,
		"NOVA_TEST_DOWNLOAD="+e.downloadPath,
		"NOVA_TEST_LATEST_VERSION="+e.latestVersion,
		"NOVA_TEST_FAIL_DOWNLOAD="+boolEnv(e.failDownload),
		"PATH="+e.fakeCommandDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func (e *installerEnv) writeVersionedNova(t *testing.T, version string) {
	t.Helper()
	content := "#!/bin/sh\nif [ \"$1\" = version ] || [ \"$1\" = --version ]; then\n  echo 'nova " + version + "'\n  exit 0\nfi\nexit 1\n"
	e.writeInstalled(t, content, 0o755)
}

func boolEnv(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func (e *installerEnv) writeInstalled(t *testing.T, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(e.installDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(e.installedPath, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func (e *installerEnv) installedContent(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile(e.installedPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
