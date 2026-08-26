package localsupervisor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateRoundTripUsesPrivatePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app", "state.json")
	want := State{
		Schema: StateSchema, ProjectPath: "/p/nova.yaml", App: "api",
		Phase: PhaseRunning, Nonce: "abc",
	}
	if err := WriteState(path, want); err != nil {
		t.Fatal(err)
	}
	got, ok, err := ReadState(path)
	if err != nil || !ok || got.Nonce != want.Nonce || got.Phase != want.Phase {
		t.Fatalf("state=%#v ok=%v err=%v", got, ok, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state mode = %v", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("state dir mode = %v", dirInfo.Mode().Perm())
	}
}

func TestReadStateDistinguishesMissingAndInvalidState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if _, ok, err := ReadState(path); err != nil || ok {
		t.Fatalf("missing state ok=%v err=%v", ok, err)
	}
	if err := os.WriteFile(path, []byte(`{"schema":99,"projectPath":"/p/nova.yaml","app":"api","phase":"running","nonce":"abc"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadState(path); err == nil {
		t.Fatal("unsupported schema unexpectedly accepted")
	}
}
