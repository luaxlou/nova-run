package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGitDeploymentVersionRequiresCleanWorktree(t *testing.T) {
	dir := initGitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := gitDeploymentVersion(dir)
	if err == nil {
		t.Fatal("expected dirty worktree error")
	}
}

func TestGitDeploymentVersionReturnsHead(t *testing.T) {
	dir := initGitRepo(t)

	version, err := gitDeploymentVersion(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := gitOutput(t, dir, "rev-parse", "--short=12", "HEAD")
	if version != want {
		t.Fatalf("version = %q, want %q", version, want)
	}
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init")
	git(t, dir, "config", "user.email", "nova@example.test")
	git(t, dir, "config", "user.name", "Nova Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", "README.md")
	git(t, dir, "commit", "-m", "init")
	return dir
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v failed: %v", args, err)
	}
	return string(out[:len(out)-1])
}
