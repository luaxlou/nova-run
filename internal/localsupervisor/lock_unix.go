//go:build darwin || linux

package localsupervisor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/unix"
)

type Lock struct {
	file *os.File
	once sync.Once
	err  error
}

func TryLock(path string) (*Lock, bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, false, fmt.Errorf("create local supervisor lock dir: %w", err)
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return nil, false, fmt.Errorf("secure local supervisor lock dir: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, fmt.Errorf("open local supervisor lock: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, false, fmt.Errorf("secure local supervisor lock: %w", err)
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("lock local supervisor runtime: %w", err)
	}
	return &Lock{file: file}, true, nil
}

func (l *Lock) File() *os.File {
	if l == nil {
		return nil
	}
	return l.file
}

func (l *Lock) Close() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		if l.file == nil {
			return
		}
		// Closing, rather than explicitly unlocking, preserves ownership when
		// this open file description has been inherited by a supervisor child.
		if err := l.file.Close(); err != nil {
			l.err = fmt.Errorf("close local supervisor lock: %w", err)
		}
	})
	return l.err
}
