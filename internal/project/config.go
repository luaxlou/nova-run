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
	App       string         `json:"app,omitempty" yaml:"app,omitempty"`
	Build     BuildConfig    `json:"build,omitempty" yaml:"build,omitempty"`
	Artifacts []string       `json:"artifacts,omitempty" yaml:"artifacts,omitempty"`
	Service   ServiceConfig  `json:"service,omitempty" yaml:"service,omitempty"`
	Apps      map[string]App `json:"apps,omitempty" yaml:"apps,omitempty"`
	AppOrder  []string       `json:"-" yaml:"-"`
}

type App struct {
	App       string        `json:"app,omitempty" yaml:"app,omitempty"`
	Build     BuildConfig   `json:"build,omitempty" yaml:"build,omitempty"`
	Artifacts []string      `json:"artifacts,omitempty" yaml:"artifacts,omitempty"`
	Service   ServiceConfig `json:"service,omitempty" yaml:"service,omitempty"`
}

type BuildConfig struct {
	Commands []string `json:"commands,omitempty" yaml:"commands,omitempty"`
}

type ServiceConfig struct {
	Command       string            `json:"command,omitempty" yaml:"command,omitempty"`
	Env           map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	HealthCommand string            `json:"healthCommand,omitempty" yaml:"healthCommand,omitempty"`
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
	order, err := readAppOrder(raw)
	if err != nil {
		return Config{}, "", err
	}
	cfg.AppOrder = order
	if err := Validate(cfg); err != nil {
		return Config{}, "", err
	}
	return cfg, path, nil
}

func readAppOrder(raw []byte) ([]string, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("parse %s: %w", ConfigFile, err)
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return nil, nil
	}
	doc := root.Content[0]
	for i := 0; i+1 < len(doc.Content); i += 2 {
		key := doc.Content[i]
		value := doc.Content[i+1]
		if key.Value != "apps" || value.Kind != yaml.MappingNode {
			continue
		}
		order := []string{}
		for j := 0; j+1 < len(value.Content); j += 2 {
			name := strings.TrimSpace(value.Content[j].Value)
			if name != "" {
				order = append(order, name)
			}
		}
		return order, nil
	}
	return nil, nil
}

func Validate(cfg Config) error {
	hasDefault := strings.TrimSpace(cfg.App) != "" ||
		len(cfg.Artifacts) > 0 ||
		strings.TrimSpace(cfg.Service.Command) != "" ||
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
		if strings.TrimSpace(name) == "all" {
			return fmt.Errorf("%s apps.all is reserved for targeting all apps", ConfigFile)
		}
		if _, err := resolveApp(cfg, name, app); err != nil {
			return err
		}
	}
	return nil
}

type Target struct {
	Name      string
	App       string
	Build     BuildConfig
	Artifacts []string
	Service   ServiceConfig
}

func Resolve(cfg Config, selector string) (Target, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		hasDefault := strings.TrimSpace(cfg.App) != "" ||
			len(cfg.Artifacts) > 0 ||
			strings.TrimSpace(cfg.Service.Command) != "" ||
			len(cfg.Build.Commands) > 0
		if hasDefault {
			return resolveApp(cfg, "", App{
				App:       cfg.App,
				Build:     cfg.Build,
				Artifacts: cfg.Artifacts,
				Service:   cfg.Service,
			})
		}
		name, app, ok := firstConfiguredApp(cfg)
		if !ok {
			return Target{}, fmt.Errorf("%s default app is not configured", ConfigFile)
		}
		return resolveApp(cfg, name, app)
	}
	app, ok := cfg.Apps[selector]
	if !ok {
		return Target{}, fmt.Errorf("%s app %q is not configured", ConfigFile, selector)
	}
	return resolveApp(cfg, selector, app)
}

func ResolveAll(cfg Config) ([]Target, error) {
	if len(cfg.Apps) == 0 {
		target, err := Resolve(cfg, "")
		if err != nil {
			return nil, err
		}
		return []Target{target}, nil
	}
	names := orderedAppNames(cfg)
	targets := make([]Target, 0, len(names))
	for _, name := range names {
		target, err := Resolve(cfg, name)
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	return targets, nil
}

func firstConfiguredApp(cfg Config) (string, App, bool) {
	for _, name := range orderedAppNames(cfg) {
		app, ok := cfg.Apps[name]
		if ok {
			return name, app, true
		}
	}
	return "", App{}, false
}

func orderedAppNames(cfg Config) []string {
	names := []string{}
	seen := map[string]bool{}
	for _, name := range cfg.AppOrder {
		if _, ok := cfg.Apps[name]; ok && !seen[name] {
			names = append(names, name)
			seen[name] = true
		}
	}
	for name := range cfg.Apps {
		if !seen[name] {
			names = append(names, name)
		}
	}
	return names
}

func resolveApp(cfg Config, name string, app App) (Target, error) {
	target := Target{
		Name:      name,
		App:       firstNonEmpty(app.App, cfg.App, name),
		Build:     mergeBuild(cfg.Build, app.Build),
		Artifacts: mergeArtifacts(cfg.Artifacts, app.Artifacts),
		Service:   mergeService(cfg.Service, app.Service),
	}
	label := ConfigFile
	if name != "" {
		label += " apps." + name
	}
	if strings.TrimSpace(target.App) == "" {
		return Target{}, fmt.Errorf("%s app is required", label)
	}
	if len(target.Artifacts) == 0 {
		return Target{}, fmt.Errorf("%s artifacts is required", label)
	}
	if len(target.Artifacts) != 1 {
		return Target{}, fmt.Errorf("%s artifacts must contain exactly one deployable artifact path", label)
	}
	for i, artifactPath := range target.Artifacts {
		if strings.TrimSpace(artifactPath) == "" {
			return Target{}, fmt.Errorf("%s artifacts[%d] is empty", label, i)
		}
		if strings.Contains(artifactPath, "\x00") {
			return Target{}, fmt.Errorf("%s artifacts[%d] contains invalid characters", label, i)
		}
	}
	if err := validateCommands(label, "build.commands", target.Build.Commands); err != nil {
		return Target{}, err
	}
	if hasService(target.Service) {
		if err := validateService(label, target.Service); err != nil {
			return Target{}, err
		}
	}
	return target, nil
}

func validateService(label string, service ServiceConfig) error {
	command := strings.TrimSpace(service.Command)
	if command == "" {
		return fmt.Errorf("%s service.command is required", label)
	}
	if strings.Contains(command, "\x00") || strings.Contains(command, "\n") || strings.Contains(command, "\r") {
		return fmt.Errorf("%s service.command contains invalid characters", label)
	}
	for key, value := range service.Env {
		if !isEnvName(key) {
			return fmt.Errorf("%s service.env contains invalid name: %s", label, key)
		}
		if strings.Contains(value, "\x00") || strings.Contains(value, "\n") || strings.Contains(value, "\r") {
			return fmt.Errorf("%s service.env.%s contains invalid characters", label, key)
		}
	}
	if health := strings.TrimSpace(service.HealthCommand); health != "" {
		if strings.Contains(health, "\x00") || strings.Contains(health, "\n") || strings.Contains(health, "\r") {
			return fmt.Errorf("%s service.healthCommand contains invalid characters", label)
		}
	}
	return nil
}

func hasService(service ServiceConfig) bool {
	return strings.TrimSpace(service.Command) != "" || len(service.Env) > 0 || strings.TrimSpace(service.HealthCommand) != ""
}

func isEnvName(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		if r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || i > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	first := value[0]
	return first == '_' || first >= 'A' && first <= 'Z' || first >= 'a' && first <= 'z'
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

func mergeArtifacts(base, override []string) []string {
	if len(override) > 0 {
		return override
	}
	return base
}

func mergeService(base, override ServiceConfig) ServiceConfig {
	if strings.TrimSpace(override.Command) != "" || len(override.Env) > 0 || strings.TrimSpace(override.HealthCommand) != "" {
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
