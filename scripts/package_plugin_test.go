package scripts

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestPackagePluginIncludesOnlyPluginSurfaces(t *testing.T) {
	root := newPluginPackageFixture(t)

	output, err := runPluginPackager(root)
	if err != nil {
		t.Fatalf("package Plugin: %v\n%s", err, output)
	}
	archive := filepath.Join(root, "dist", "nova-run-plugin-1.2.3.tgz")
	listing, err := exec.Command("tar", "-tzf", archive).CombinedOutput()
	if err != nil {
		t.Fatalf("list Plugin archive: %v\n%s", err, listing)
	}

	var files []string
	for _, entry := range strings.Fields(string(listing)) {
		entry = strings.TrimPrefix(entry, "./")
		if !strings.HasSuffix(entry, "/") {
			files = append(files, entry)
		}
	}
	sort.Strings(files)
	want := []string{
		".agents/plugins/marketplace.json",
		".claude-plugin/marketplace.json",
		".claude-plugin/plugin.json",
		".codex-plugin/plugin.json",
		"docs/agent-plugin.md",
		"skills/doctor/SKILL.md",
		"skills/doctor/agents/openai.yaml",
		"skills/init/SKILL.md",
		"skills/init/agents/openai.yaml",
	}
	sort.Strings(want)
	if strings.Join(files, "\n") != strings.Join(want, "\n") {
		t.Fatalf("archive files:\n%s\n\nwant:\n%s", strings.Join(files, "\n"), strings.Join(want, "\n"))
	}
}

func TestPackagePluginRejectsVersionDrift(t *testing.T) {
	tests := map[string]func(string){
		"Codex manifest": func(root string) {
			writePluginJSON(t, filepath.Join(root, ".codex-plugin/plugin.json"), map[string]any{"version": "9.9.9"})
		},
		"Claude manifest": func(root string) {
			writePluginJSON(t, filepath.Join(root, ".claude-plugin/plugin.json"), map[string]any{"version": "9.9.9"})
		},
		"marketplace": func(root string) {
			writePluginJSON(t, filepath.Join(root, ".claude-plugin/marketplace.json"), map[string]any{
				"name": "nova", "version": "9.9.9",
				"plugins": []map[string]any{{"name": "nova", "source": "./", "version": "1.2.3"}},
			})
		},
		"marketplace Plugin": func(root string) {
			writePluginJSON(t, filepath.Join(root, ".claude-plugin/marketplace.json"), map[string]any{
				"name": "nova", "version": "1.2.3",
				"plugins": []map[string]any{{"name": "nova", "source": "./", "version": "9.9.9"}},
			})
		},
		"init Skill": func(root string) {
			writePluginSkill(t, root, "init", "9.9.9")
		},
		"doctor Skill": func(root string) {
			writePluginSkill(t, root, "doctor", "9.9.9")
		},
	}

	for name, introduceDrift := range tests {
		t.Run(name, func(t *testing.T) {
			root := newPluginPackageFixture(t)
			introduceDrift(root)

			output, err := runPluginPackager(root)
			if err == nil {
				t.Fatalf("version drift unexpectedly packaged the Plugin:\n%s", output)
			}
			if !strings.Contains(output, "Version drift") || !strings.Contains(output, "9.9.9") {
				t.Fatalf("version drift output=%q", output)
			}
			if _, statErr := os.Stat(filepath.Join(root, "dist", "nova-run-plugin-1.2.3.tgz")); !os.IsNotExist(statErr) {
				t.Fatalf("version drift left an archive behind: %v", statErr)
			}
		})
	}
}

func TestPackagePluginIsReproducible(t *testing.T) {
	root := newPluginPackageFixture(t)
	if output, err := runPluginPackager(root); err != nil {
		t.Fatalf("first package Plugin: %v\n%s", err, output)
	}
	archive := filepath.Join(root, "dist", "nova-run-plugin-1.2.3.tgz")
	first, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}

	changed := time.Unix(1_800_000_000, 0)
	for _, relative := range []string{
		".codex-plugin/plugin.json",
		".claude-plugin/marketplace.json",
		"skills/init/SKILL.md",
		"docs/agent-plugin.md",
	} {
		path := filepath.Join(root, relative)
		if err := os.Chtimes(path, changed, changed); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(filepath.Join(root, "docs/agent-plugin.md"), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := runPluginPackager(root); err != nil {
		t.Fatalf("second package Plugin: %v\n%s", err, output)
	}
	second, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("Plugin archives differ after source mtimes changed")
	}
}

func newPluginPackageFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, directory := range []string{
		"scripts", ".codex-plugin", ".claude-plugin", ".agents/plugins",
		"skills/init", "skills/doctor", "docs",
	} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	script, err := os.ReadFile("package-plugin.sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts/package-plugin.sh"), script, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "VERSION"), []byte("1.2.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writePluginJSON(t, filepath.Join(root, ".codex-plugin/plugin.json"), map[string]any{
		"name": "nova", "version": "1.2.3", "skills": "./skills/",
		"interface": map[string]any{"capabilities": []string{"skills"}},
	})
	writePluginJSON(t, filepath.Join(root, ".claude-plugin/plugin.json"), map[string]any{
		"name": "nova", "version": "1.2.3", "skills": "./skills/",
	})
	writePluginJSON(t, filepath.Join(root, ".claude-plugin/marketplace.json"), map[string]any{
		"name": "nova", "version": "1.2.3",
		"plugins": []map[string]any{{"name": "nova", "source": "./", "version": "1.2.3"}},
	})
	writePluginJSON(t, filepath.Join(root, ".agents/plugins/marketplace.json"), map[string]any{
		"name": "nova", "plugins": []map[string]any{{
			"name": "nova", "source": map[string]any{"source": "local", "path": "./"},
		}},
	})
	writePluginSkill(t, root, "init", "1.2.3")
	writePluginSkill(t, root, "doctor", "1.2.3")
	if err := os.WriteFile(filepath.Join(root, "docs/agent-plugin.md"), []byte("# Nova Run Plugin\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// These repository files must never leak into the Plugin artifact.
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("not packaged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs/internal.md"), []byte("not packaged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func writePluginJSON(t *testing.T, path string, value any) {
	t.Helper()
	content, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(content, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writePluginSkill(t *testing.T, root, name, version string) {
	t.Helper()
	source := "---\nname: " + name + "\ndescription: Test fixture.\nmetadata:\n  novaRunRelease: \"" + version + "\"\n---\n\n# Fixture\n"
	skillRoot := filepath.Join(root, "skills", name)
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(skillRoot, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	metadata := "interface:\n  display_name: \"Fixture\"\n  short_description: \"Fixture skill for packaging tests\"\n  default_prompt: \"Use $" + name + " for this fixture.\"\n"
	if err := os.WriteFile(filepath.Join(skillRoot, "agents", "openai.yaml"), []byte(metadata), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runPluginPackager(root string) (string, error) {
	command := exec.Command("bash", filepath.Join(root, "scripts/package-plugin.sh"))
	command.Dir = root
	output, err := command.CombinedOutput()
	return string(output), err
}
