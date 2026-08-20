package deploy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeployMetadataRoundTrip(t *testing.T) {
	dir := t.TempDir()

	if err := SaveMetadata(dir, Metadata{Version: "abc123"}); err != nil {
		t.Fatal(err)
	}
	meta, ok, err := LoadMetadata(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected metadata")
	}
	if meta.Version != "abc123" {
		t.Fatalf("version = %q", meta.Version)
	}
}

func TestLoadMetadataMissing(t *testing.T) {
	_, ok, err := LoadMetadata(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected missing metadata")
	}
}

func TestSaveMetadataRejectsEmptyVersion(t *testing.T) {
	if err := SaveMetadata(t.TempDir(), Metadata{}); err == nil {
		t.Fatal("expected empty version error")
	}
}

func TestCurrentVersionReadsDeployedMetadata(t *testing.T) {
	root := t.TempDir()
	appDir := filepath.Join(root, "demo")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SaveMetadata(appDir, Metadata{Version: "abc123"}); err != nil {
		t.Fatal(err)
	}

	version, ok, err := CurrentVersion(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || version != "abc123" {
		t.Fatalf("version = %q ok=%v", version, ok)
	}
}
