package deploy

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func ReplaceArtifact(dst, src string) error {
	artifactPath := filepath.Clean(src)
	targetPath := filepath.Clean(dst)
	parent := filepath.Dir(targetPath)

	if _, err := os.Stat(artifactPath); err != nil {
		return fmt.Errorf("artifact not found: %w", err)
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create app root: %w", err)
	}

	tmpDir, err := os.MkdirTemp(parent, ".nova-app-*.tmp")
	if err != nil {
		return fmt.Errorf("create staging directory: %w", err)
	}
	staged := false
	defer func() {
		if !staged {
			_ = os.RemoveAll(tmpDir)
		}
	}()

	if err := extractArchive(artifactPath, tmpDir); err != nil {
		return err
	}
	if err := EnsureRunBinary(tmpDir); err != nil {
		return fmt.Errorf("invalid artifact: %w", err)
	}

	if err := os.RemoveAll(targetPath); err != nil {
		return fmt.Errorf("remove old app directory: %w", err)
	}
	if err := os.Rename(tmpDir, targetPath); err != nil {
		return fmt.Errorf("activate artifact: %w", err)
	}
	staged = true
	return nil
}

func extractArchive(src string, dst string) error {
	file, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer file.Close()

	gr, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("read gzip archive: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read archive: %w", err)
		}

		if h.Name == "" || h.Name == "." {
			continue
		}
		if strings.HasPrefix(h.Name, "..") {
			return fmt.Errorf("invalid archive entry: %s", h.Name)
		}
		targetPath := filepath.Join(dst, filepath.FromSlash(h.Name))
		if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(dst)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid archive entry: %s", h.Name)
		}

		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return fmt.Errorf("create dir %s: %w", h.Name, err)
			}
			if err := os.Chmod(targetPath, os.FileMode(h.Mode)); err != nil {
				return fmt.Errorf("chmod dir %s: %w", h.Name, err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return fmt.Errorf("create dir for %s: %w", h.Name, err)
			}
			out, err := os.Create(targetPath)
			if err != nil {
				return fmt.Errorf("create file %s: %w", h.Name, err)
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return fmt.Errorf("write file %s: %w", h.Name, err)
			}
			if err := out.Chmod(os.FileMode(h.Mode)); err != nil {
				_ = out.Close()
				return fmt.Errorf("chmod file %s: %w", h.Name, err)
			}
			if err := out.Close(); err != nil {
				return fmt.Errorf("close file %s: %w", h.Name, err)
			}
		default:
			continue
		}
	}
	return nil
}
