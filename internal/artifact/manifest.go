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
	App      string           `json:"app,omitempty" yaml:"app,omitempty"`
	Artifact ArtifactManifest `json:"artifact,omitempty" yaml:"artifact,omitempty"`
	Process  ProcessManifest  `json:"process,omitempty" yaml:"process,omitempty"`
	Runtime  RuntimeManifest  `json:"runtime,omitempty" yaml:"runtime,omitempty"`
}

type ArtifactManifest struct {
	Files []string `json:"files,omitempty" yaml:"files,omitempty"`
}

type ProcessManifest struct {
	Command string `json:"command,omitempty" yaml:"command,omitempty"`
}

type RuntimeManifest struct {
	HealthCommand string `json:"healthCommand,omitempty" yaml:"healthCommand,omitempty"`
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
	for _, file := range manifest.Artifact.Files {
		clean := filepath.Clean(strings.TrimSpace(file))
		if clean == "" || clean == "." || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			return fmt.Errorf("%s artifact.files contains an invalid path: %q", ManifestFile, file)
		}
		if _, err := os.Stat(filepath.Join(artifactDir, clean)); err != nil {
			return fmt.Errorf("%s artifact file %q not found: %w", ManifestFile, file, err)
		}
	}
	if command := strings.TrimSpace(manifest.Runtime.HealthCommand); command != "" {
		if strings.Contains(command, "\x00") || strings.Contains(command, "\n") || strings.Contains(command, "\r") {
			return fmt.Errorf("%s runtime.healthCommand contains invalid characters", ManifestFile)
		}
	}
	return nil
}

func DeploymentSummary(manifest Manifest) []string {
	lines := []string{}
	if manifest.App != "" {
		lines = append(lines, "app: "+manifest.App)
	}
	if len(manifest.Artifact.Files) > 0 {
		lines = append(lines, "artifact files: "+strings.Join(manifest.Artifact.Files, ", "))
	}
	if manifest.Process.Command != "" {
		lines = append(lines, "process command: "+manifest.Process.Command)
	}
	if manifest.Runtime.HealthCommand != "" {
		lines = append(lines, "health command: "+manifest.Runtime.HealthCommand)
	}
	return lines
}
