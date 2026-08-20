package artifact

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const ManifestFile = "nova.app.yaml"

type Manifest struct {
	App     string          `json:"app,omitempty" yaml:"app,omitempty"`
	Process ProcessManifest `json:"process,omitempty" yaml:"process,omitempty"`
	Static  StaticManifest  `json:"static,omitempty" yaml:"static,omitempty"`
	Backend BackendManifest `json:"backend,omitempty" yaml:"backend,omitempty"`
}

type ProcessManifest struct {
	Command string `json:"command,omitempty" yaml:"command,omitempty"`
}

type StaticManifest struct {
	Root string `json:"root,omitempty" yaml:"root,omitempty"`
	SPA  bool   `json:"spa,omitempty" yaml:"spa,omitempty"`
}

type BackendManifest struct {
	Port      int      `json:"port,omitempty" yaml:"port,omitempty"`
	Health    string   `json:"health,omitempty" yaml:"health,omitempty"`
	Ready     string   `json:"ready,omitempty" yaml:"ready,omitempty"`
	APIPrefix []string `json:"apiPrefix,omitempty" yaml:"apiPrefix,omitempty"`
}

func LoadManifest(artifactDir string) (Manifest, bool, error) {
	path := filepath.Join(artifactDir, ManifestFile)
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Manifest{}, false, nil
	}
	if err != nil {
		return Manifest{}, false, fmt.Errorf("read %s: %w", ManifestFile, err)
	}
	var manifest Manifest
	if err := yaml.Unmarshal(raw, &manifest); err != nil {
		return Manifest{}, false, fmt.Errorf("parse %s: %w", ManifestFile, err)
	}
	if err := ValidateManifest(artifactDir, manifest); err != nil {
		return Manifest{}, true, err
	}
	return manifest, true, nil
}

func ValidateManifest(artifactDir string, manifest Manifest) error {
	if strings.TrimSpace(manifest.Process.Command) != "" && strings.TrimSpace(manifest.Process.Command) != "./run" {
		return fmt.Errorf("%s process.command must be ./run; Nova executes the artifact run file", ManifestFile)
	}
	if root := strings.TrimSpace(manifest.Static.Root); root != "" {
		clean := filepath.Clean(root)
		if clean == "." || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			return fmt.Errorf("%s static.root must be a relative path inside the artifact", ManifestFile)
		}
		info, err := os.Stat(filepath.Join(artifactDir, clean))
		if err != nil {
			return fmt.Errorf("%s static.root not found: %w", ManifestFile, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("%s static.root is not a directory", ManifestFile)
		}
	}
	if manifest.Backend.Port < 0 || manifest.Backend.Port > 65535 {
		return fmt.Errorf("%s backend.port is invalid", ManifestFile)
	}
	return nil
}

func DeploymentAdvice(appName string, manifest Manifest) []string {
	lines := []string{}
	appRoot := "/var/lib/nova/apps/" + appName
	if manifest.Static.Root != "" {
		staticRoot := filepath.ToSlash(filepath.Join(appRoot, filepath.Clean(manifest.Static.Root)))
		lines = append(lines, "static files: "+staticRoot)
	}
	if manifest.Backend.Port > 0 {
		target := fmt.Sprintf("127.0.0.1:%d", manifest.Backend.Port)
		prefixes := manifest.Backend.APIPrefix
		if len(prefixes) == 0 {
			prefixes = []string{"/api/*"}
		}
		if manifest.Backend.Health != "" {
			prefixes = append(prefixes, manifest.Backend.Health)
		}
		if manifest.Backend.Ready != "" {
			prefixes = append(prefixes, manifest.Backend.Ready)
		}
		lines = append(lines, "backend proxy: "+strings.Join(prefixes, " ")+" -> "+target)
	}
	if manifest.Static.Root != "" && manifest.Static.SPA {
		lines = append(lines, "spa fallback: serve index.html for non-file routes")
	}
	return lines
}
