//go:build windows

package localsupervisor

import (
	"context"
	"fmt"
	"os"
)

// Manager is present on Windows so the cross-platform CLI can expose remote
// deployment and lifecycle commands. Local process supervision currently
// relies on Unix process groups, file locks, and Unix sockets.
type Manager struct{}

func NewManager() (*Manager, error) {
	return nil, unsupportedWindowsError()
}

func (m *Manager) Start(context.Context, Target) (Result, error) {
	return Result{}, unsupportedWindowsError()
}

func (m *Manager) Stop(context.Context, Target) (Result, error) {
	return Result{}, unsupportedWindowsError()
}

func (m *Manager) Status(context.Context, Target) (Status, error) {
	return Status{}, unsupportedWindowsError()
}

func (m *Manager) StartAll(context.Context, []Target) ([]Result, error) {
	return nil, unsupportedWindowsError()
}

func (m *Manager) StopAll(context.Context, []Target) ([]Result, error) {
	return nil, unsupportedWindowsError()
}

func (m *Manager) RestartAll(context.Context, []Target) ([]Result, error) {
	return nil, unsupportedWindowsError()
}

func RunSupervisor(context.Context, string, *os.File, *os.File) error {
	return unsupportedWindowsError()
}

func unsupportedWindowsError() error {
	return fmt.Errorf("local lifecycle commands are not supported on Windows; use --remote to control Nova Agent applications")
}
