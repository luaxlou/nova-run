package runtime

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

type Status struct {
	State     string `json:"state"`
	SubState  string `json:"subState"`
	PID       string `json:"pid"`
	Started   string `json:"started"`
	ExitCode  string `json:"exitCode"`
}

type Controller struct{}

func (c *Controller) serviceName(app string) string {
	return fmt.Sprintf("nova@%s.service", app)
}

func (c *Controller) systemctl(args ...string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("systemctl not supported on %s", runtime.GOOS)
	}
	cmd := exec.Command("systemctl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (c *Controller) Start(name string) error {
	return c.systemctl("start", c.serviceName(name))
}

func (c *Controller) Stop(name string) error {
	return c.systemctl("stop", c.serviceName(name))
}

func (c *Controller) Restart(name string) error {
	return c.systemctl("restart", c.serviceName(name))
}

func (c *Controller) Status(name string) (Status, error) {
	return Status{
		State:    "inactive",
		SubState: "dead",
		PID:      "0",
		Started:  "",
		ExitCode: "",
	}, nil
}

