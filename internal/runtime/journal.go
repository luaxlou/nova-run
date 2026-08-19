package runtime

import (
	"fmt"
	"os/exec"
	"strconv"
)

func JournalTailCommand(app string, follow bool, lines int) *exec.Cmd {
	service := fmt.Sprintf("nova@%s.service", app)
	args := []string{"-u", service}
	if lines <= 0 {
		lines = 100
	}
	args = append(args, "-n", strconv.Itoa(lines))
	args = append(args, "--no-pager")
	if follow {
		args = append(args, "-f")
	}
	args = append(args, "-o", "cat")
	return exec.Command("journalctl", args...)
}
