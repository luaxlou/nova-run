package artifact

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManifestValidatesStaticRoot(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(`app: demo
process:
  command: ./run
static:
  root: dist
  spa: true
backend:
  port: 8080
  health: /healthz
  ready: /readyz
  apiPrefix:
    - /api/*
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
	if manifest.Static.Root != "dist" {
		t.Fatalf("static root = %q", manifest.Static.Root)
	}
}

func TestDeploymentAdviceDescribesStaticAndBackendEdges(t *testing.T) {
	lines := DeploymentAdvice("demo", Manifest{
		Static:  StaticManifest{Root: "dist", SPA: true},
		Backend: BackendManifest{Port: 8080, Health: "/healthz", Ready: "/readyz", APIPrefix: []string{"/api/*"}},
	})

	expected := []string{
		"static files: /var/lib/nova/apps/demo/dist",
		"backend proxy: /api/* /healthz /readyz -> 127.0.0.1:8080",
		"spa fallback: serve index.html for non-file routes",
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
