package artifact

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManifestValidatesDeclaredArtifactFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "run"), []byte("#!/usr/bin/env sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app"), []byte("binary"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(`app: demo
artifact:
  files:
    - run
    - app
    - dist
process:
  command: ./app
runtime:
  healthCommand: ./run --health
`), 0o644); err != nil {
		t.Fatal(err)
	}

	manifest, ok, err := LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected manifest")
	}
	if len(manifest.Artifact.Files) != 3 {
		t.Fatalf("artifact files = %#v", manifest.Artifact.Files)
	}
}

func TestDeploymentSummaryDescribesArtifactAndRuntime(t *testing.T) {
	lines := DeploymentSummary(Manifest{
		App:      "demo",
		Artifact: ArtifactManifest{Files: []string{"run", "app", "dist"}},
		Process:  ProcessManifest{Command: "./app"},
		Runtime:  RuntimeManifest{HealthCommand: "./run --health"},
	})

	expected := []string{
		"app: demo",
		"artifact files: run, app, dist",
		"process command: ./app",
		"health command: ./run --health",
	}
	if len(lines) != len(expected) {
		t.Fatalf("lines = %#v", lines)
	}
	for i := range expected {
		if lines[i] != expected[i] {
			t.Fatalf("line %d = %q, want %q", i, lines[i], expected[i])
		}
	}
}
