package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const ConfigFile = "nova.yaml"

type Config struct {
	App      string         `json:"app,omitempty" yaml:"app,omitempty"`
	Build    BuildConfig    `json:"build,omitempty" yaml:"build,omitempty"`
	Artifact ArtifactConfig `json:"artifact,omitempty" yaml:"artifact,omitempty"`
}

type BuildConfig struct {
	Commands []string `json:"commands,omitempty" yaml:"commands,omitempty"`
}

type ArtifactConfig struct {
	Dir string `json:"dir,omitempty" yaml:"dir,omitempty"`
}

func Load(dir string) (Config, string, error) {
	path := filepath.Join(dir, ConfigFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, "", fmt.Errorf("read %s: %w", ConfigFile, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return Config{}, "", fmt.Errorf("parse %s: %w", ConfigFile, err)
	}
	if err := Validate(cfg); err != nil {
		return Config{}, "", err
	}
	return cfg, path, nil
}

func Validate(cfg Config) error {
	if strings.TrimSpace(cfg.App) == "" {
		return fmt.Errorf("%s app is required", ConfigFile)
	}
	if strings.TrimSpace(cfg.Artifact.Dir) == "" {
		return fmt.Errorf("%s artifact.dir is required", ConfigFile)
	}
	for i, command := range cfg.Build.Commands {
		if strings.TrimSpace(command) == "" {
			return fmt.Errorf("%s build.commands[%d] is empty", ConfigFile, i)
		}
		if strings.Contains(command, "\x00") {
			return fmt.Errorf("%s build.commands[%d] contains invalid characters", ConfigFile, i)
		}
	}
	return nil
}
