package project

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
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

func TestLoadForLifecycleAllowsLifecycleOnlyConfigWithoutDeployFields(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ConfigFile), []byte("start: printf start\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, path, err := LoadForLifecycle(dir)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, ConfigFile) {
		t.Fatalf("path = %q", path)
	}
	if cfg.Start != "printf start" {
		t.Fatalf("lifecycle start = %q", cfg.Start)
	}
	if _, _, err := Load(dir); err == nil {
		t.Fatal("lifecycle-only config must not pass deployment validation")
	}
}

func TestResolveLifecycleUsesTopLevelAndAppCommands(t *testing.T) {
	cfg := Config{
		Start: "printf root-start",
		Apps: map[string]App{
			"api":    {Start: "printf api-start"},
			"worker": {},
		},
		AppOrder: []string{"api", "worker"},
	}

	tests := []struct {
		selector string
		want     LifecycleTarget
	}{
		{selector: "", want: LifecycleTarget{Name: "default", Start: "printf root-start"}},
		{selector: "api", want: LifecycleTarget{Name: "api", Start: "printf api-start"}},
		{selector: "worker", want: LifecycleTarget{Name: "worker", Start: "printf root-start"}},
	}
	for _, test := range tests {
		t.Run(test.selector, func(t *testing.T) {
			got, err := ResolveLifecycle(cfg, test.selector, true)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("target = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestResolveLifecycleDefaultsToFirstDeclaredApp(t *testing.T) {
	cfg := Config{
		Apps: map[string]App{
			"api":    {Start: "start-api"},
			"worker": {Start: "start-worker"},
		},
		AppOrder: []string{"worker", "api"},
	}

	target, err := ResolveLifecycle(cfg, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if want := (LifecycleTarget{Name: "worker", Start: "start-worker"}); target != want {
		t.Fatalf("target = %#v, want %#v", target, want)
	}
}

func TestResolveAllLifecyclesPreflightsMissingCommandWithoutPartialTargets(t *testing.T) {
	cfg := Config{
		Apps: map[string]App{
			"api":    {Start: "start-api"},
			"worker": {},
		},
		AppOrder: []string{"api", "worker"},
	}

	targets, err := ResolveAllLifecycles(cfg, true)
	if err == nil || !strings.Contains(err.Error(), "nova.yaml apps.worker start is required") {
		t.Fatalf("err = %v", err)
	}
	if targets != nil {
		t.Fatalf("targets = %#v, want nil", targets)
	}
}

func TestResolveAllLifecyclesUsesTopLevelOnceWithoutApps(t *testing.T) {
	targets, err := ResolveAllLifecycles(Config{Start: "start"}, true)
	if err != nil {
		t.Fatal(err)
	}
	want := []LifecycleTarget{{Name: "default", Start: "start"}}
	if !slices.Equal(targets, want) {
		t.Fatalf("targets = %#v, want %#v", targets, want)
	}
}

func TestResolveLifecycleAllowsIdentityOnlyLookup(t *testing.T) {
	cfg := Config{Apps: map[string]App{"api": {}}, AppOrder: []string{"api"}}
	target, err := ResolveLifecycle(cfg, "api", false)
	if err != nil {
		t.Fatal(err)
	}
	if target.Name != "api" || target.Start != "" {
		t.Fatalf("target = %#v", target)
	}
	if _, err := ResolveLifecycle(cfg, "api", true); err == nil || !strings.Contains(err.Error(), "start is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadForLifecycleRejectsInvalidCommandCharacters(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ConfigFile), []byte("start: |\n  printf one\n  printf two\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := LoadForLifecycle(dir)
	if err == nil || !strings.Contains(err.Error(), "nova.yaml start contains invalid characters") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadForLifecycleRejectsFormerRunField(t *testing.T) {
	for _, config := range []string{"run: printf old\n", "run:\n", "apps:\n  api:\n    run:\n"} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ConfigFile), []byte(config), 0o644); err != nil {
			t.Fatal(err)
		}
		_, _, err := LoadForLifecycle(dir)
		if err == nil || !strings.Contains(err.Error(), "run is no longer supported") {
			t.Fatalf("config %q err = %v", config, err)
		}
	}
}

func TestLoadForLifecycleRejectsStopField(t *testing.T) {
	for _, config := range []string{
		"start: sleep 30\nstop: kill-app\n",
		"apps:\n  api:\n    start: sleep 30\n    stop:\n",
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ConfigFile), []byte(config), 0o644); err != nil {
			t.Fatal(err)
		}
		_, _, err := LoadForLifecycle(dir)
		if err == nil || !strings.Contains(err.Error(), "stop is no longer supported") {
			t.Fatalf("config %q err = %v", config, err)
		}
	}
}
