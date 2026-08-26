package localsupervisor

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPathsForHashesUntrustedIdentity(t *testing.T) {
	cacheRoot := t.TempDir()
	target := Target{ProjectPath: "/tmp/example/nova.yaml", Name: "../../api"}
	paths, err := PathsFor(cacheRoot, target)
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(cacheRoot, paths.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(rel, "..") || strings.Contains(paths.Dir, target.Name) {
		t.Fatalf("unsafe runtime dir = %q", paths.Dir)
	}
	if filepath.Dir(paths.Lock) != paths.Dir || filepath.Dir(paths.Socket) != paths.Dir || filepath.Dir(paths.State) != paths.Dir {
		t.Fatalf("paths do not share runtime dir: %#v", paths)
	}
}

func TestPathsForRequiresCanonicalIdentityInputs(t *testing.T) {
	tests := []Target{
		{Name: "api"},
		{ProjectPath: "/tmp/nova.yaml"},
	}
	for _, target := range tests {
		if _, err := PathsFor(t.TempDir(), target); err == nil {
			t.Fatalf("target %#v unexpectedly accepted", target)
		}
	}
	if _, err := PathsFor("", Target{ProjectPath: "/tmp/nova.yaml", Name: "api"}); err == nil {
		t.Fatal("empty cache root unexpectedly accepted")
	}
}
