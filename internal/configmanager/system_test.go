package configmanager

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/luaxlou/glow-ops/internal/storage"
)

func TestDeleteSystemConfig_Idempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "nova.db")
	storage.Init(dbPath)
	storage.Reload()

	if err := SetSystemConfig("k1", "v1"); err != nil {
		t.Fatalf("set: %v", err)
	}

	if err := DeleteSystemConfig("k1"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err := GetSystemConfig("k1")
	if err == nil {
		t.Fatalf("expected error")
	}
	if err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows, got: %v", err)
	}

	if err := DeleteSystemConfig("k1"); err != nil {
		t.Fatalf("delete again: %v", err)
	}
}
