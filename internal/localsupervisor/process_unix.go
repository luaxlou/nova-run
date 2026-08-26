//go:build darwin || linux

package localsupervisor

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func startApplication(startup Startup, output *os.File) (*exec.Cmd, error) {
	cmd := exec.Command("sh", "-lc", startup.Target.Start)
	cmd.Dir = startup.Target.Dir
	cmd.Env = os.Environ()
	cmd.Stdin = nil
	cmd.Stdout = output
	cmd.Stderr = output
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start application: %w", err)
	}
	return cmd, nil
}

func signalProcessGroup(pid int, signal syscall.Signal) error {
	if pid <= 0 {
		return fmt.Errorf("invalid application pid %d", pid)
	}
	if err := syscall.Kill(-pid, signal); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("signal application process group: %w", err)
	}
	return nil
}
