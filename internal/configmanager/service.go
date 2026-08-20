package configmanager

import (
	"fmt"
	"sync"

	"github.com/luaxlou/glow-ops/internal/storage"
)

var (
	mu    sync.Mutex
	cache map[string]any
)

func Init() {
	cache = make(map[string]any)
}

// EnsureInitialized ensures that the service is initialized and cache is loaded.
func EnsureInitialized() error {
	mu.Lock()
	defer mu.Unlock()

	if cache == nil {
		cache = make(map[string]any)
	}

	if len(cache) > 0 {
		return nil
	}
	return loadCache()
}

func loadCache() error {
	all, err := storage.GetAllAppConfigs()
	if err != nil {
		return err
	}

	cache = make(map[string]any, len(all))
	for appName, config := range all {
		cache[appName] = config
	}

	return nil
}

// Get returns the configuration for a given app.
func Get(appName string) (map[string]any, error) {
	if err := EnsureInitialized(); err != nil {
		return nil, err
	}

	mu.Lock()
	defer mu.Unlock()

	if config, ok := cache[appName]; ok {
		return config.(map[string]any), nil
	}
	return nil, fmt.Errorf("config not found for app: %s", appName)
}

// GetValue retrieves a specific key from an app's configuration.
func GetValue(appName, key string) (any, bool) {
	cfg, err := Get(appName)
	if err != nil {
		return nil, false
	}
	val, ok := cfg[key]
	return val, ok
}

// Set updates the configuration for a given app.
// It merges with existing config if merge is true, otherwise overwrites.
func Set(appName string, newConfig map[string]any, merge bool) error {
	if err := EnsureInitialized(); err != nil {
		return err
	}

	mu.Lock()
	defer mu.Unlock()

	var finalConfig map[string]any

	if merge {
		finalConfig = make(map[string]any)
		// Start with existing
		if existing, ok := cache[appName]; ok {
			if existingMap, ok := existing.(map[string]any); ok {
				for k, v := range existingMap {
					finalConfig[k] = v
				}
			}
		}
		// Merge new
		for k, v := range newConfig {
			finalConfig[k] = v
		}
	} else {
		finalConfig = newConfig
	}

	if err := storage.SetAppConfig(appName, finalConfig); err != nil {
		return err
	}

	// Update Cache
	cache[appName] = finalConfig
	return nil
}
