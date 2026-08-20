package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProjectConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ConfigFile), []byte(`app: demo
build:
  commands:
    - npm run build
artifact:
  dir: .nova/artifact
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, path, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, ConfigFile) {
		t.Fatalf("path = %q", path)
	}
	if cfg.App != "demo" {
		t.Fatalf("app = %q", cfg.App)
	}
	if cfg.Artifact.Dir != ".nova/artifact" {
		t.Fatalf("artifact dir = %q", cfg.Artifact.Dir)
	}
}

func TestValidateRequiresAppAndArtifactDir(t *testing.T) {
	if err := Validate(Config{}); err == nil {
		t.Fatal("expected missing app error")
	}
	if err := Validate(Config{App: "demo"}); err == nil {
		t.Fatal("expected missing artifact dir error")
	}
}

func TestResolveSubAppOverridesProjectDefaults(t *testing.T) {
	cfg := Config{
		App:      "demo",
		Build:    BuildConfig{Commands: []string{"npm run build"}},
		Artifact: ArtifactConfig{Dir: ".nova/default"},
		Apps: map[string]App{
			"backend": {
				App:      "demo-backend",
				Build:    BuildConfig{Commands: []string{"go build ./cmd/api"}},
				Artifact: ArtifactConfig{Dir: ".nova/backend"},
			},
		},
	}

	target, err := Resolve(cfg, "backend")
	if err != nil {
		t.Fatal(err)
	}
	if target.App != "demo-backend" {
		t.Fatalf("app = %q", target.App)
	}
	if target.Artifact.Dir != ".nova/backend" {
		t.Fatalf("artifact = %q", target.Artifact.Dir)
	}
}
