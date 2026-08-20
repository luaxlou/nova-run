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
	Apps     map[string]App `json:"apps,omitempty" yaml:"apps,omitempty"`
}

type App struct {
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
	hasDefault := strings.TrimSpace(cfg.App) != "" ||
		strings.TrimSpace(cfg.Artifact.Dir) != "" ||
		len(cfg.Build.Commands) > 0
	if hasDefault {
		if _, err := Resolve(cfg, ""); err != nil {
			return err
		}
	}
	if !hasDefault && len(cfg.Apps) == 0 {
		return fmt.Errorf("%s app or apps is required", ConfigFile)
	}
	for name, app := range cfg.Apps {
		if _, err := resolveApp(cfg, name, app); err != nil {
			return err
		}
	}
	return nil
}

type Target struct {
	Name     string
	App      string
	Build    BuildConfig
	Artifact ArtifactConfig
}

func Resolve(cfg Config, selector string) (Target, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		hasDefault := strings.TrimSpace(cfg.App) != "" ||
			strings.TrimSpace(cfg.Artifact.Dir) != "" ||
			len(cfg.Build.Commands) > 0
		if !hasDefault {
			return Target{}, fmt.Errorf("%s default app is not configured; choose one of apps", ConfigFile)
		}
		return resolveApp(cfg, "", App{
			App:      cfg.App,
			Build:    cfg.Build,
			Artifact: cfg.Artifact,
		})
	}
	app, ok := cfg.Apps[selector]
	if !ok {
		return Target{}, fmt.Errorf("%s app %q is not configured", ConfigFile, selector)
	}
	return resolveApp(cfg, selector, app)
}

func resolveApp(cfg Config, name string, app App) (Target, error) {
	target := Target{
		Name:     name,
		App:      firstNonEmpty(app.App, cfg.App, name),
		Build:    mergeBuild(cfg.Build, app.Build),
		Artifact: mergeArtifact(cfg.Artifact, app.Artifact),
	}
	label := ConfigFile
	if name != "" {
		label += " apps." + name
	}
	if strings.TrimSpace(target.App) == "" {
		return Target{}, fmt.Errorf("%s app is required", label)
	}
	if strings.TrimSpace(target.Artifact.Dir) == "" {
		return Target{}, fmt.Errorf("%s artifact.dir is required", label)
	}
	if err := validateCommands(label, "build.commands", target.Build.Commands); err != nil {
		return Target{}, err
	}
	return target, nil
}

func validateCommands(label, field string, commands []string) error {
	for i, command := range commands {
		if strings.TrimSpace(command) == "" {
			return fmt.Errorf("%s %s[%d] is empty", label, field, i)
		}
		if strings.Contains(command, "\x00") {
			return fmt.Errorf("%s %s[%d] contains invalid characters", label, field, i)
		}
	}
	return nil
}

func mergeBuild(base, override BuildConfig) BuildConfig {
	if len(override.Commands) > 0 {
		return override
	}
	return base
}

func mergeArtifact(base, override ArtifactConfig) ArtifactConfig {
	if strings.TrimSpace(override.Dir) != "" {
		return override
	}
	return base
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
