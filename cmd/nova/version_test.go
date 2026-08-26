package main

import (
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
