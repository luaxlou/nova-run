package artifact

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func PackDir(sourceDir, destPath string) error {
	sourceDir = filepath.Clean(sourceDir)
	info, err := os.Stat(sourceDir)
	if err != nil {
		return fmt.Errorf("source dir invalid: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("source path is not a directory")
	}

	target, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create archive: %w", err)
	}
	defer target.Close()

	gw := gzip.NewWriter(target)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	walkErr := filepath.WalkDir(sourceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return fmt.Errorf("resolve relative path: %w", err)
		}
		relPath = filepath.ToSlash(relPath)
		if relPath == "." {
			return nil
		}
		if strings.HasPrefix(relPath, "..") {
			return fmt.Errorf("invalid path in source: %s", relPath)
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("header for %s: %w", relPath, err)
		}
		header.Name = relPath

		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("write header for %s: %w", relPath, err)
		}
		if d.IsDir() {
			return nil
		}
		src, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open %s: %w", relPath, err)
		}
		if _, err := io.Copy(tw, src); err != nil {
			_ = src.Close()
			return fmt.Errorf("copy %s: %w", relPath, err)
		}
		if err := src.Close(); err != nil {
			return fmt.Errorf("close %s: %w", relPath, err)
		}
		return nil
	})
	if walkErr != nil {
		return walkErr
	}
	return nil
}

func PackPath(sourcePath, destPath string) error {
	sourcePath = filepath.Clean(sourcePath)
	info, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("source path invalid: %w", err)
	}
	if info.IsDir() {
		return PackDir(sourcePath, destPath)
	}

	target, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create archive: %w", err)
	}
	defer target.Close()

	gw := gzip.NewWriter(target)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return fmt.Errorf("header for %s: %w", filepath.Base(sourcePath), err)
	}
	header.Name = filepath.ToSlash(filepath.Base(sourcePath))
	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("write header for %s: %w", header.Name, err)
	}
	src, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open %s: %w", sourcePath, err)
	}
	defer src.Close()
	if _, err := io.Copy(tw, src); err != nil {
		return fmt.Errorf("copy %s: %w", sourcePath, err)
	}
	return nil
}
