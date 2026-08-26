package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestListIsRejectedBeforeBootstrap(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "nova")
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build nova: %v\n%s", err, output)
	}

	command := exec.Command(binary, "list")
	command.Env = append(os.Environ(), "HOME="+t.TempDir())
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("nova list unexpectedly succeeded:\n%s", output)
	}
	text := string(output)
	if !strings.Contains(text, "Usage:") {
		t.Fatalf("removed command did not show usage:\n%s", output)
	}
	if strings.Contains(text, "bootstrap failed") || strings.Contains(text, "list failed") {
		t.Fatalf("removed command performed list setup:\n%s", output)
	}
}
