package localsupervisor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func WriteState(path string, state State) error {
	if err := validateState(state); err != nil {
		return err
	}
	state.Schema = StateSchema
	payload, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode local supervisor state: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create local supervisor state dir: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("secure local supervisor state dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".state-*")
	if err != nil {
		return fmt.Errorf("create local supervisor state temp: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("secure local supervisor state temp: %w", err)
	}
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write local supervisor state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync local supervisor state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close local supervisor state: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace local supervisor state: %w", err)
	}
	dirFile, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open local supervisor state dir: %w", err)
	}
	defer dirFile.Close()
	if err := dirFile.Sync(); err != nil {
		return fmt.Errorf("sync local supervisor state dir: %w", err)
	}
	return nil
}

func ReadState(path string) (State, bool, error) {
	payload, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, fmt.Errorf("read local supervisor state: %w", err)
	}
	var state State
	if err := json.Unmarshal(payload, &state); err != nil {
		return State{}, false, fmt.Errorf("decode local supervisor state: %w", err)
	}
	if state.Schema != StateSchema {
		return State{}, false, fmt.Errorf("unsupported local supervisor state schema %d", state.Schema)
	}
	if err := validateState(state); err != nil {
		return State{}, false, err
	}
	return state, true, nil
}

func validateState(state State) error {
	if state.Schema != 0 && state.Schema != StateSchema {
		return fmt.Errorf("unsupported local supervisor state schema %d", state.Schema)
	}
	if state.ProjectPath == "" || state.App == "" || state.Nonce == "" {
		return fmt.Errorf("local supervisor state identity is incomplete")
	}
	switch state.Phase {
	case PhaseStarting, PhaseRunning, PhaseStopping, PhaseStopped, PhaseError:
		return nil
	default:
		return fmt.Errorf("invalid local supervisor state phase %q", state.Phase)
	}
}
