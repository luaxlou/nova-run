package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionCommandsReportInjectedBuildVersion(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "nova")
	build := exec.Command("go", "build", "-ldflags", "-X main.version=1.2.3", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build versioned nova: %v\n%s", err, output)
	}
	for _, arg := range []string{"version", "--version", "-v"} {
		t.Run(arg, func(t *testing.T) {
			command := exec.Command(binary, arg)
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("nova %s: %v\n%s", arg, err, output)
			}
			if got := strings.TrimSpace(string(output)); got != "nova 1.2.3" {
				t.Fatalf("output=%q", got)
			}
		})
	}
}

func TestRuntimeConfigPathIsGlobal(t *testing.T) {
	t.Setenv("NOVA_CLIENT_ENV", "")
	home := t.TempDir()
	if got, want := runtimeConfigPath(home), filepath.Join(home, ".nova", "client.env"); got != want {
		t.Fatalf("path=%q want=%q", got, want)
	}
}

func TestRuntimeConfigReadyReusesExistingGlobalTarget(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "client.env")
	t.Setenv("NOVA_CLIENT_ENV", configPath)
	t.Setenv("NOVA_ENDPOINT", "")
	t.Setenv("NOVA_AGENT_ENDPOINT", "")
	t.Setenv("NOVA_TOKEN", "")
	t.Setenv("NOVA_AGENT_TOKEN", "")
	if err := writeConfig(configPath, "https://agent.example", "machine-token"); err != nil {
		t.Fatal(err)
	}
	if !runtimeConfigReady() {
		t.Fatal("existing global target was not reusable")
	}
	if os.Getenv("NOVA_ENDPOINT") != "https://agent.example" || os.Getenv("NOVA_TOKEN") != "machine-token" {
		t.Fatal("global target was not loaded into the client environment")
	}
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o want=600", info.Mode().Perm())
	}
}
