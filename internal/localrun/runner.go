package localrun

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"os/signal"
	"strings"
	"time"
)

type Command struct {
	Name         string
	ShellCommand string
}

type Streams struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

type ExitError struct {
	Name    string
	Command string
	Code    int
	Err     error
}

func (e *ExitError) Error() string {
	if e.Command == "" {
		return fmt.Sprintf("%s: %v", e.Name, e.Err)
	}
	return fmt.Sprintf("%s command %q exited with code %d: %v", e.Name, e.Command, e.Code, e.Err)
}

func (e *ExitError) Unwrap() error {
	return e.Err
}

func (e *ExitError) ExitCode() int {
	return e.Code
}

type processResult struct {
	index int
	err   error
}

func Run(ctx context.Context, commands []Command, dir string, streams Streams, gracePeriod time.Duration) error {
	if err := validateInputs(commands, dir, streams, gracePeriod); err != nil {
		return err
	}

	runCtx, stopSignals := signal.NotifyContext(ctx, terminationSignals()...)
	defer stopSignals()
	if runCtx.Err() != nil {
		return interruptedError(runCtx.Err())
	}

	results := make(chan processResult, len(commands))
	children := make([]*exec.Cmd, 0, len(commands))
	finished := make([]bool, len(commands))
	for index, spec := range commands {
		cmd := exec.Command("sh", "-lc", spec.ShellCommand)
		cmd.Dir = dir
		cmd.Stdout = streams.Stdout
		cmd.Stderr = streams.Stderr
		if len(commands) == 1 {
			cmd.Stdin = streams.Stdin
		}
		prepareProcess(cmd)
		if len(commands) > 1 {
			fmt.Fprintf(streams.Stdout, "[%s] $ %s\n", spec.Name, spec.ShellCommand)
		}
		if err := cmd.Start(); err != nil {
			cleanupErr := stopAndReap(children, finished[:len(children)], results, gracePeriod)
			startErr := fmt.Errorf("%s command %q failed to start: %w", spec.Name, spec.ShellCommand, err)
			return joinPrimary(startErr, cleanupErr)
		}
		children = append(children, cmd)
		go func(childIndex int, child *exec.Cmd) {
			results <- processResult{index: childIndex, err: child.Wait()}
		}(index, cmd)
	}

	select {
	case result := <-results:
		finished[result.index] = true
		primary := commandResult(commands[result.index], result.err)
		return joinPrimary(primary, stopAndReap(children, finished, results, gracePeriod))
	case <-runCtx.Done():
		primary := interruptedError(runCtx.Err())
		return joinPrimary(primary, stopAndReap(children, finished, results, gracePeriod))
	}
}

func validateInputs(commands []Command, dir string, streams Streams, gracePeriod time.Duration) error {
	if len(commands) == 0 {
		return fmt.Errorf("at least one local command is required")
	}
	if strings.TrimSpace(dir) == "" {
		return fmt.Errorf("local command directory is required")
	}
	if streams.Stdout == nil || streams.Stderr == nil {
		return fmt.Errorf("local command stdout and stderr are required")
	}
	if gracePeriod <= 0 {
		return fmt.Errorf("local command grace period must be positive")
	}
	for index, command := range commands {
		if strings.TrimSpace(command.Name) == "" {
			return fmt.Errorf("local command %d name is required", index)
		}
		if strings.TrimSpace(command.ShellCommand) == "" {
			return fmt.Errorf("%s command is required", command.Name)
		}
	}
	return nil
}

func commandResult(command Command, err error) error {
	if err == nil {
		return nil
	}
	code := 1
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() >= 0 {
		code = exitErr.ExitCode()
	}
	return &ExitError{Name: command.Name, Command: command.ShellCommand, Code: code, Err: err}
}

func interruptedError(err error) error {
	if err == nil {
		err = context.Canceled
	}
	return &ExitError{Name: "local run interrupted", Code: 130, Err: err}
}

func stopAndReap(children []*exec.Cmd, finished []bool, results <-chan processResult, gracePeriod time.Duration) error {
	remaining := 0
	var cleanupErrors []error
	for index, child := range children {
		if finished[index] {
			continue
		}
		remaining++
		if err := terminateProcess(child); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("terminate process %d: %w", child.Process.Pid, err))
		}
	}
	if remaining == 0 {
		return errors.Join(cleanupErrors...)
	}

	timer := time.NewTimer(gracePeriod)
	defer timer.Stop()
	forced := false
	for remaining > 0 {
		select {
		case result := <-results:
			if result.index >= len(finished) || finished[result.index] {
				continue
			}
			finished[result.index] = true
			remaining--
		case <-timer.C:
			if forced {
				continue
			}
			forced = true
			for index, child := range children {
				if finished[index] {
					continue
				}
				if err := killProcess(child); err != nil {
					cleanupErrors = append(cleanupErrors, fmt.Errorf("kill process %d: %w", child.Process.Pid, err))
				}
			}
		}
	}
	return errors.Join(cleanupErrors...)
}

func joinPrimary(primary, cleanup error) error {
	if primary == nil {
		return cleanup
	}
	if cleanup == nil {
		return primary
	}
	return errors.Join(primary, cleanup)
}
