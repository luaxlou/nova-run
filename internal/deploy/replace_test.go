package deploy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/luaxlou/glow-ops/internal/artifact"
)

func TestReplaceArtifactMakesActivatedAppDirectoryReadable(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "index.html"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(root, "artifact.tar.gz")
	if err := artifact.PackDir(source, archive); err != nil {
		t.Fatal(err)
	}

	appDir := filepath.Join(root, "apps", "frontend")
	if err := ReplaceArtifact(appDir, archive); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(appDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Fatalf("app dir mode = %o, want 755", got)
	}
}
