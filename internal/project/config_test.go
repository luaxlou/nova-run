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
artifacts: dist/nova
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
	if cfg.Artifacts != "dist/nova" {
		t.Fatalf("artifacts = %q", cfg.Artifacts)
	}
}

func TestValidateRequiresAppAndArtifacts(t *testing.T) {
	if err := Validate(Config{}); err == nil {
		t.Fatal("expected missing app error")
	}
	if err := Validate(Config{App: "demo"}); err == nil {
		t.Fatal("expected missing artifacts error")
	}
}

func TestResolveSubAppOverridesProjectDefaults(t *testing.T) {
	cfg := Config{
		App:       "demo",
		Build:     BuildConfig{Commands: []string{"npm run build"}},
		Artifacts: "dist/default",
		Apps: map[string]App{
			"backend": {
				App:       "demo-backend",
				Build:     BuildConfig{Commands: []string{"go build ./cmd/api"}},
				Artifacts: "backend/dist/api",
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
	if target.Artifacts != "backend/dist/api" {
		t.Fatalf("artifacts = %q", target.Artifacts)
	}
}

func TestResolveSubAppDefaultsAppNameToSelector(t *testing.T) {
	cfg := Config{
		Apps: map[string]App{
			"sbom-platform": {
				Build:     BuildConfig{Commands: []string{"scripts/build.sh"}},
				Artifacts: "backend/dist/sbom-platform",
			},
		},
	}

	target, err := Resolve(cfg, "sbom-platform")
	if err != nil {
		t.Fatal(err)
	}
	if target.Name != "sbom-platform" {
		t.Fatalf("name = %q", target.Name)
	}
	if target.App != "sbom-platform" {
		t.Fatalf("app = %q", target.App)
	}
}
