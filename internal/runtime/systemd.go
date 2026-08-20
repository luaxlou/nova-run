package runtime

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

type Status struct {
	State    string `json:"state"`
	SubState string `json:"subState"`
	PID      string `json:"pid"`
	Started  string `json:"started"`
	ExitCode string `json:"exitCode"`
	Version  string `json:"version,omitempty"`
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

func (c *Controller) systemctlShow(name string) (map[string]string, error) {
	out, err := exec.Command(
		"systemctl",
		"show",
		"--no-pager",
		"--property=ActiveState",
		"--property=SubState",
		"--property=MainPID",
		"--property=ActiveEnterTimestamp",
		"--property=ExecMainStatus",
		c.serviceName(name),
	).CombinedOutput()
	props := parseSystemdProperties(string(out))
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "not found") || strings.Contains(msg, "could not be found") {
			return props, nil
		}
		return props, fmt.Errorf("%s: %s", err, strings.TrimSpace(string(out)))
	}
	return props, nil
}

func parseSystemdProperties(output string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return result
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
	props, err := c.systemctlShow(name)
	if err != nil {
		return Status{}, err
	}
	status := Status{
		State:    "inactive",
		SubState: "dead",
		PID:      "0",
		Started:  "n/a",
		ExitCode: "0",
	}
	if active := strings.TrimSpace(props["ActiveState"]); active != "" {
		status.State = active
	}
	if sub := strings.TrimSpace(props["SubState"]); sub != "" {
		status.SubState = sub
	}
	if pid := strings.TrimSpace(props["MainPID"]); pid != "" {
		if _, e := strconv.Atoi(pid); e == nil {
			status.PID = pid
		}
	}
	if started := strings.TrimSpace(props["ActiveEnterTimestamp"]); started != "" && started != "n/a" {
		status.Started = started
	}
	if code := strings.TrimSpace(props["ExecMainStatus"]); code != "" {
		status.ExitCode = code
	}
	return status, nil
}
