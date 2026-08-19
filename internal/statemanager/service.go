package statemanager

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/luaxlou/glow-ops/pkg/api"
	"github.com/luaxlou/glow-ops/internal/storage"
)

var (
	mu   sync.Mutex
)

// SaveApp saves or updates the application information.
func SaveApp(app api.AppInfo) error {
	infoJSON, err := json.Marshal(app)
	if err != nil {
		return err
	}

	if err := storage.SetAppState(app.Name, infoJSON); err != nil {
		return err
	}
	return nil
}

// GetApp retrieves application information by name.
func GetApp(name string) (*api.AppInfo, error) {
	mu.Lock()
	defer mu.Unlock()

	infoJSON, found, err := storage.GetAppState(name)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("app not found: %s", name)
	}

	var app api.AppInfo
	if err := json.Unmarshal(infoJSON, &app); err != nil {
		return nil, err
	}
	return &app, nil
}

// DeleteApp removes an application from the database.
func DeleteApp(name string) error {
	mu.Lock()
	defer mu.Unlock()
	return storage.DeleteAppState(name)
}

// ListApps retrieves all applications.
func ListApps() ([]api.AppInfo, error) {
	mu.Lock()
	defer mu.Unlock()

	rawStates, err := storage.ListAppStates()
	if err != nil {
		return nil, err
	}

	var apps []api.AppInfo
	for _, infoJSON := range rawStates {
		var app api.AppInfo
		if err := json.Unmarshal(infoJSON, &app); err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}
	return apps, nil
}
