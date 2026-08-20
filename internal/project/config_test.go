package project

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestLoadProjectConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ConfigFile), []byte(`app: demo
build:
  commands:
    - npm run build
artifacts:
  - dist/nova
service:
  command: ./app
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
	if !slices.Equal(cfg.Artifacts, []string{"dist/nova"}) {
		t.Fatalf("artifacts = %#v", cfg.Artifacts)
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

func TestValidateAllowsStaticArtifactWithoutService(t *testing.T) {
	err := Validate(Config{App: "docs", Artifacts: []string{"dist/docs"}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestResolveSubAppOverridesProjectDefaults(t *testing.T) {
	cfg := Config{
		App:       "demo",
		Build:     BuildConfig{Commands: []string{"npm run build"}},
		Artifacts: []string{"dist/default"},
		Apps: map[string]App{
			"backend": {
				App:       "demo-backend",
				Build:     BuildConfig{Commands: []string{"go build ./cmd/api"}},
				Artifacts: []string{"backend/dist/api"},
				Service:   ServiceConfig{Command: "./api"},
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
	if !slices.Equal(target.Artifacts, []string{"backend/dist/api"}) {
		t.Fatalf("artifacts = %#v", target.Artifacts)
	}
}

func TestResolveSubAppDefaultsAppNameToSelector(t *testing.T) {
	cfg := Config{
		Apps: map[string]App{
			"sbom-platform": {
				Build:     BuildConfig{Commands: []string{"scripts/build.sh"}},
				Artifacts: []string{"backend/dist/sbom-platform"},
				Service:   ServiceConfig{Command: "./sbom-api"},
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

func TestResolveDefaultsToFirstDeclaredApp(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ConfigFile), []byte(`apps:
  sbom-platform:
    build:
      commands:
        - scripts/build-platform.sh
    artifacts:
      - backend/dist/platform
    service:
      command: ./api
  sbom-platform-api:
    build:
      commands:
        - scripts/build-api.sh
    artifacts:
      - backend/dist/api
    service:
      command: ./api
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	target, err := Resolve(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if target.Name != "sbom-platform" {
		t.Fatalf("default target = %q", target.Name)
	}

	targets, err := ResolveAll(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{targets[0].Name, targets[1].Name}; !slices.Equal(got, []string{"sbom-platform", "sbom-platform-api"}) {
		t.Fatalf("targets = %#v", got)
	}
}

func TestValidateRejectsReservedAllApp(t *testing.T) {
	err := Validate(Config{
		Apps: map[string]App{
			"all": {
				Build:     BuildConfig{Commands: []string{"scripts/build.sh"}},
				Artifacts: []string{"dist/app"},
			},
		},
	})
	if err == nil {
		t.Fatal("expected reserved all app error")
	}
}
