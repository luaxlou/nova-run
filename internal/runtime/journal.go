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
	if follow {
		args = append(args, "-f")
	}
	return exec.Command("journalctl", args...)
}

