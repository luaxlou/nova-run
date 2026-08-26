package localsupervisor

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
)

func PathsFor(cacheRoot string, target Target) (Paths, error) {
	if strings.TrimSpace(cacheRoot) == "" {
		return Paths{}, fmt.Errorf("local supervisor cache root is required")
	}
	if strings.TrimSpace(target.ProjectPath) == "" {
		return Paths{}, fmt.Errorf("local supervisor project path is required")
	}
	if strings.TrimSpace(target.Name) == "" {
		return Paths{}, fmt.Errorf("local supervisor app name is required")
	}
	projectPath, err := filepath.Abs(target.ProjectPath)
	if err != nil {
		return Paths{}, fmt.Errorf("resolve project path: %w", err)
	}
	dir := filepath.Join(cacheRoot, "nova", "run", shortHash(filepath.Clean(projectPath)), shortHash(target.Name))
	return Paths{
		Dir:     dir,
		Lock:    filepath.Join(dir, "lock"),
		State:   filepath.Join(dir, "state.json"),
		Socket:  filepath.Join(dir, "control.sock"),
		Output:  filepath.Join(dir, "output.log"),
		Startup: filepath.Join(dir, "startup.json"),
	}, nil
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:16])
}
