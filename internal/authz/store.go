package authz

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const recordSchema = 1

var projectNamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

type Store struct {
	Dir string
}

type record struct {
	Schema    int    `json:"schema"`
	Project   string `json:"project"`
	Verifier  string `json:"verifier"`
	UpdatedAt string `json:"updatedAt"`
}

func ValidateProjectName(project string) error {
	if !projectNamePattern.MatchString(project) || strings.Contains(project, "--") {
		return fmt.Errorf("invalid project name %q: use lowercase letters, numbers, and single hyphens", project)
	}
	return nil
}

func (s Store) Set(project string) (string, error) {
	if err := ValidateProjectName(project); err != nil {
		return "", err
	}
	if strings.TrimSpace(s.Dir) == "" {
		return "", fmt.Errorf("auth directory is required")
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("generate project token: %w", err)
	}
	token := "nvp_" + hex.EncodeToString(secret)
	sum := sha256.Sum256([]byte(token))
	rec := record{
		Schema: recordSchema, Project: project, Verifier: hex.EncodeToString(sum[:]),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode project authorization: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return "", fmt.Errorf("create auth directory: %w", err)
	}
	if err := os.Chmod(s.Dir, 0o700); err != nil {
		return "", fmt.Errorf("secure auth directory: %w", err)
	}
	tmp, err := os.CreateTemp(s.Dir, ".auth-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temporary authorization: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("write project authorization: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("sync project authorization: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close project authorization: %w", err)
	}
	if err := os.Rename(tmpName, s.path(project)); err != nil {
		return "", fmt.Errorf("activate project authorization: %w", err)
	}
	return token, nil
}

func (s Store) Verify(project, token string) bool {
	if ValidateProjectName(project) != nil || !strings.HasPrefix(token, "nvp_") {
		return false
	}
	data, err := os.ReadFile(s.path(project))
	if err != nil {
		return false
	}
	var rec record
	if json.Unmarshal(data, &rec) != nil || rec.Schema != recordSchema || rec.Project != project {
		return false
	}
	want, err := hex.DecodeString(rec.Verifier)
	if err != nil || len(want) != sha256.Size {
		return false
	}
	got := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(got[:], want) == 1
}

func (s Store) path(project string) string {
	return filepath.Join(s.Dir, project+".json")
}
