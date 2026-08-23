//go:build linux || darwin

package localrun

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func prepareProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func signalProcessGroup(cmd *exec.Cmd, signal os.Signal) error {
	unixSignal, ok := signal.(syscall.Signal)
	if !ok {
		return syscall.EINVAL
	}
	return sendSignalToProcessGroup(cmd, unixSignal)
}

func killProcessGroup(cmd *exec.Cmd) error {
	return sendSignalToProcessGroup(cmd, syscall.SIGKILL)
}

func sendSignalToProcessGroup(cmd *exec.Cmd, signal syscall.Signal) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	err := syscall.Kill(-cmd.Process.Pid, signal)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func processGroupAlive(cmd *exec.Cmd) (bool, error) {
	if cmd == nil || cmd.Process == nil {
		return false, nil
	}
	err := syscall.Kill(-cmd.Process.Pid, 0)
	if err == nil || errors.Is(err, syscall.EPERM) {
		return true, nil
	}
	if errors.Is(err, syscall.ESRCH) {
		return false, nil
	}
	return false, err
}

func processExitCode(exitErr *exec.ExitError) int {
	if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return 128 + int(status.Signal())
	}
	if code := exitErr.ExitCode(); code >= 0 {
		return code
	}
	return 1
}

func defaultTerminationSignal() os.Signal {
	return syscall.SIGTERM
}

func contextCancellationSignal() os.Signal {
	return syscall.SIGINT
}

func signalExitCode(signal os.Signal) int {
	if unixSignal, ok := signal.(syscall.Signal); ok {
		return 128 + int(unixSignal)
	}
	return 1
}

func terminationSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}
