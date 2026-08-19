package deploy

import (
	"fmt"
	"io"
	"os"
)

func ReplaceArtifact(dst, src string) error {
	source, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer source.Close()

	dest, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create destination: %w", err)
	}
	defer dest.Close()

	_, err = io.Copy(dest, source)
	if err != nil {
		return fmt.Errorf("copy artifact: %w", err)
}
	return nil
}

