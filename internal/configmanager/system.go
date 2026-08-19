package configmanager

import (
	"encoding/json"

	"github.com/luaxlou/glow-ops/internal/storage"
)

func GetSystemConfig(key string) (string, error) {
	return storage.GetSystemConfig(key)
}

func GetSystemConfigJSON(key string, v interface{}) error {
	val, err := GetSystemConfig(key)
	if err != nil {
		return err
	}
	if val == "" {
		return nil
	}
	return json.Unmarshal([]byte(val), v)
}

func SetSystemConfig(key, value string) error {
	return storage.SetSystemConfig(key, value)
}

func SetSystemConfigJSON(key string, v interface{}) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return SetSystemConfig(key, string(b))
}

func DeleteSystemConfig(key string) error {
	return storage.DeleteSystemConfig(key)
}
