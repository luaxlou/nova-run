package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luaxlou/glow-ops/internal/artifact"
	"github.com/luaxlou/glow-ops/internal/project"
)

func TestPrepareDeployArtifactWrapsServiceDirectoryWithoutChangingIt(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "artifact.json"), []byte("immutable\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "service.mjs"), []byte("export {};\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := project.Target{
		App:     "api-server",
		Service: project.ServiceConfig{Command: "ev start api-server --prod"},
	}

	prepared, cleanup, err := prepareDeployArtifact(source, target)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if prepared == source {
		t.Fatal("service directory must be wrapped in Nova-owned staging")
	}
	if _, err := os.Stat(filepath.Join(source, "run")); !os.IsNotExist(err) {
		t.Fatalf("source artifact was changed: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(prepared, "artifact", "artifact.json")); err != nil || string(got) != "immutable\n" {
		t.Fatalf("staged artifact = %q, %v", got, err)
	}
	run, err := os.ReadFile(filepath.Join(prepared, "run"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(run), "cd 'artifact'") || !strings.Contains(string(run), "ev start api-server --prod") {
		t.Fatalf("run script does not enter the immutable artifact: %s", run)
	}
	manifest, ok, err := artifact.LoadManifest(prepared)
	if err != nil || !ok {
		t.Fatalf("load Nova manifest: ok=%t err=%v", ok, err)
	}
	if len(manifest.Artifact.Files) != 2 || manifest.Artifact.Files[0] != "artifact" || manifest.Artifact.Files[1] != "run" {
		t.Fatalf("packaged files = %#v", manifest.Artifact.Files)
	}
}
