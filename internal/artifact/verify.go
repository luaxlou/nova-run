package artifact

import (
	"fmt"
	"os"
	"path/filepath"
)

func HasRunBinary(appDir string) bool {
	info, err := os.Stat(filepath.Join(appDir, "run"))
	return err == nil && info.Mode()&0o111 != 0
}

func EnsureRunBinary(appDir string) error {
	runPath := filepath.Join(appDir, "run")
	info, err := os.Stat(runPath)
	if err != nil {
		return fmt.Errorf("run not found: %w", err)
	}
	if info.Mode()&0o111 == 0 {
		return fmt.Errorf("run is not executable")
	}
	return nil
}
