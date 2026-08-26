package localsupervisor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTryLockIsExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app", "lock")
	first, owned, err := TryLock(path)
	if err != nil || !owned || first == nil {
		t.Fatalf("first=%v owned=%v err=%v", first, owned, err)
	}
	defer first.Close()

	second, owned, err := TryLock(path)
	if err != nil || owned || second != nil {
		t.Fatalf("second=%v owned=%v err=%v", second, owned, err)
	}
}

func TestTryLockSecuresExistingLockFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	lock, owned, err := TryLock(path)
	if err != nil || !owned {
		t.Fatalf("owned=%v err=%v", owned, err)
	}
	defer lock.Close()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("lock mode=%v", info.Mode().Perm())
	}
}

func TestLockCanBeReacquiredAfterClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	first, owned, err := TryLock(path)
	if err != nil || !owned {
		t.Fatalf("owned=%v err=%v", owned, err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, owned, err := TryLock(path)
	if err != nil || !owned {
		t.Fatalf("reacquire owned=%v err=%v", owned, err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
}
