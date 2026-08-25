package localcommand

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

type Command struct {
	Target       string
	Action       string
	ShellCommand string
}

type Streams struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

type ExitError struct {
	Target  string
	Action  string
	Command string
	Code    int
	Err     error
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("%s %s command %q failed: %v", e.Target, e.Action, e.Command, e.Err)
}

func (e *ExitError) Unwrap() error {
	return e.Err
}

func (e *ExitError) ExitCode() int {
	if e.Code > 0 {
		return e.Code
	}
	return 1
}

func Run(ctx context.Context, commands []Command, dir string, streams Streams) error {
	if err := validate(commands, dir, streams); err != nil {
		return err
	}
	showCommands := len(commands) > 1
	for _, command := range commands {
		if showCommands {
			fmt.Fprintf(streams.Stdout, "[%s] $ %s: %s\n", command.Target, command.Action, command.ShellCommand)
		}
		cmd := exec.CommandContext(ctx, "sh", "-lc", command.ShellCommand)
		cmd.Dir = dir
		cmd.Stdin = streams.Stdin
		cmd.Stdout = streams.Stdout
		cmd.Stderr = streams.Stderr
		if err := cmd.Run(); err != nil {
			code := 1
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) && exitErr.ExitCode() > 0 {
				code = exitErr.ExitCode()
			}
			return &ExitError{Target: command.Target, Action: command.Action, Command: command.ShellCommand, Code: code, Err: err}
		}
	}
	return nil
}

func validate(commands []Command, dir string, streams Streams) error {
	if len(commands) == 0 {
		return fmt.Errorf("at least one local lifecycle command is required")
	}
	if strings.TrimSpace(dir) == "" {
		return fmt.Errorf("local lifecycle command directory is required")
	}
	if streams.Stdin == nil || streams.Stdout == nil || streams.Stderr == nil {
		return fmt.Errorf("local lifecycle command stdin, stdout, and stderr are required")
	}
	for _, command := range commands {
		if strings.TrimSpace(command.Target) == "" {
			return fmt.Errorf("local lifecycle command target is required")
		}
		if strings.TrimSpace(command.Action) == "" {
			return fmt.Errorf("%s local lifecycle action is required", command.Target)
		}
		if strings.TrimSpace(command.ShellCommand) == "" {
			return fmt.Errorf("%s %s command is required", command.Target, command.Action)
		}
	}
	return nil
}
