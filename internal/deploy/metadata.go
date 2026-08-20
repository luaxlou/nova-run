package deploy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const MetadataFile = ".nova-deploy.json"

type Metadata struct {
	Version string `json:"version"`
}

func LoadMetadata(appDir string) (Metadata, bool, error) {
	raw, err := os.ReadFile(filepath.Join(appDir, MetadataFile))
	if os.IsNotExist(err) {
		return Metadata{}, false, nil
	}
	if err != nil {
		return Metadata{}, false, fmt.Errorf("read %s: %w", MetadataFile, err)
	}
	var meta Metadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		return Metadata{}, true, fmt.Errorf("parse %s: %w", MetadataFile, err)
	}
	meta.Version = strings.TrimSpace(meta.Version)
	if meta.Version == "" {
		return Metadata{}, true, fmt.Errorf("%s version is required", MetadataFile)
	}
	return meta, true, nil
}

func SaveMetadata(appDir string, meta Metadata) error {
	meta.Version = strings.TrimSpace(meta.Version)
	if meta.Version == "" {
		return fmt.Errorf("%s version is required", MetadataFile)
	}
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return fmt.Errorf("create app dir: %w", err)
	}
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", MetadataFile, err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(filepath.Join(appDir, MetadataFile), raw, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", MetadataFile, err)
	}
	return nil
}

func CurrentVersion(appRoot, app string) (string, bool, error) {
	meta, ok, err := LoadMetadata(filepath.Join(appRoot, app))
	if err != nil {
		return "", false, err
	}
	return meta.Version, ok, nil
}
