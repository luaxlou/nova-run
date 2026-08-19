package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var appNameRE = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func ValidateAppName(name string) error {
	if !appNameRE.MatchString(name) {
		return fmt.Errorf("invalid app name %q, only A-Za-z0-9_- allowed", name)
	}
	return nil
}

func EnsureAppDirectory(root, name string) (string, error) {
	if err := ValidateAppName(name); err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(root, name), nil
}

