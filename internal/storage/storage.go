package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type persistedState struct {
	AppConfigs    map[string]json.RawMessage `json:"app_configs"`
	SystemConfigs map[string]string          `json:"system_configs"`
	AppStates     map[string]json.RawMessage `json:"app_states"`
}

var (
	mu        sync.Mutex
	statePath = "nova.db"
	loaded    bool
	state     persistedState
)

func Init(path string) {
	mu.Lock()
	defer mu.Unlock()
	if path == "" {
		path = "nova.db"
	}
	statePath = path
	loaded = false
	state = persistedState{}
}

func Reload() {
	mu.Lock()
	defer mu.Unlock()
	loaded = false
	state = persistedState{}
}

func GetSystemConfig(key string) (string, error) {
	mu.Lock()
	defer mu.Unlock()
	if err := loadLocked(); err != nil {
		return "", err
	}

	val, ok := state.SystemConfigs[key]
	if !ok {
		return "", sql.ErrNoRows
	}
	return val, nil
}

func SetSystemConfig(key, value string) error {
	mu.Lock()
	defer mu.Unlock()
	if err := loadLocked(); err != nil {
		return err
	}

	if state.SystemConfigs == nil {
		state.SystemConfigs = make(map[string]string)
	}
	state.SystemConfigs[key] = value
	return persistLocked()
}

func DeleteSystemConfig(key string) error {
	mu.Lock()
	defer mu.Unlock()
	if err := loadLocked(); err != nil {
		return err
	}

	delete(state.SystemConfigs, key)
	return persistLocked()
}

func GetAppConfig(appName string) (map[string]any, bool, error) {
	mu.Lock()
	defer mu.Unlock()
	if err := loadLocked(); err != nil {
		return nil, false, err
	}

	raw, ok := state.AppConfigs[appName]
	if !ok {
		return nil, false, nil
	}

	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, false, err
	}
	return cfg, true, nil
}

func GetAllAppConfigs() (map[string]map[string]any, error) {
	mu.Lock()
	defer mu.Unlock()
	if err := loadLocked(); err != nil {
		return nil, err
	}

	result := make(map[string]map[string]any, len(state.AppConfigs))
	for appName, raw := range state.AppConfigs {
		var cfg map[string]any
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal app config %q: %w", appName, err)
		}
		result[appName] = cfg
	}
	return result, nil
}

func SetAppConfig(appName string, cfg map[string]any) error {
	mu.Lock()
	defer mu.Unlock()
	if err := loadLocked(); err != nil {
		return err
	}

	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	if state.AppConfigs == nil {
		state.AppConfigs = make(map[string]json.RawMessage)
	}
	state.AppConfigs[appName] = raw
	return persistLocked()
}

func GetAppState(appName string) ([]byte, bool, error) {
	mu.Lock()
	defer mu.Unlock()
	if err := loadLocked(); err != nil {
		return nil, false, err
	}

	raw, ok := state.AppStates[appName]
	if !ok {
		return nil, false, nil
	}
	return append([]byte(nil), raw...), true, nil
}

func SetAppState(appName string, payload []byte) error {
	mu.Lock()
	defer mu.Unlock()
	if err := loadLocked(); err != nil {
		return err
	}

	if state.AppStates == nil {
		state.AppStates = make(map[string]json.RawMessage)
	}
	state.AppStates[appName] = append([]byte(nil), payload...)
	return persistLocked()
}

func DeleteAppState(appName string) error {
	mu.Lock()
	defer mu.Unlock()
	if err := loadLocked(); err != nil {
		return err
	}

	delete(state.AppStates, appName)
	return persistLocked()
}

func ListAppStates() ([][]byte, error) {
	mu.Lock()
	defer mu.Unlock()
	if err := loadLocked(); err != nil {
		return nil, err
	}

	out := make([][]byte, 0, len(state.AppStates))
	for _, payload := range state.AppStates {
		out = append(out, append([]byte(nil), payload...))
	}
	return out, nil
}

func loadLocked() error {
	if loaded {
		return nil
	}

	state = persistedState{
		AppConfigs:    make(map[string]json.RawMessage),
		SystemConfigs: make(map[string]string),
		AppStates:     make(map[string]json.RawMessage),
	}

	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			loaded = true
			return nil
		}
		return err
	}
	if len(data) == 0 {
		loaded = true
		return nil
	}

	var loadedState persistedState
	if err := json.Unmarshal(data, &loadedState); err != nil {
		return err
	}
	if loadedState.AppConfigs != nil {
		state.AppConfigs = loadedState.AppConfigs
	}
	if loadedState.SystemConfigs != nil {
		state.SystemConfigs = loadedState.SystemConfigs
	}
	if loadedState.AppStates != nil {
		state.AppStates = loadedState.AppStates
	}

	loaded = true
	return nil
}

func persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(statePath), 0755); err != nil {
		return fmt.Errorf("failed to create state dir: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := statePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmpPath, statePath)
}

