package runtime

import (
	"strings"
	"testing"
	"time"
)

func TestWaitForStableServiceRequiresContinuousRunningState(t *testing.T) {
	statuses := []Status{
		{State: "active", SubState: "running", ExitCode: "0"},
		{State: "activating", SubState: "start", ExitCode: "0"},
		{State: "active", SubState: "running", ExitCode: "0"},
	}
	index := 0
	status := func(string) (Status, error) {
		if index < len(statuses)-1 {
			current := statuses[index]
			index++
			return current, nil
		}
		return statuses[len(statuses)-1], nil
	}

	if err := waitForStableService(status, "api", 100*time.Millisecond, 4*time.Millisecond, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if index < len(statuses)-1 {
		t.Fatal("activation returned before observing the interrupted running state")
	}
}

func TestWaitForStableServiceRejectsAutomaticRestart(t *testing.T) {
	status := func(string) (Status, error) {
		return Status{State: "activating", SubState: "auto-restart", ExitCode: "127"}, nil
	}

	err := waitForStableService(status, "api", time.Second, time.Millisecond, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "exit=127") {
		t.Fatalf("error = %v, want activation failure with exit code", err)
	}
}
