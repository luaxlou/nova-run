package statusview

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestRenderAlignsStatusColumnsAndOmitsEmptyDetails(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	rows := []Row{
		{App: "api", State: "running", PID: "12345", Ports: []int{8080, 3000, 8080}, StartedAt: now.Add(-2 * time.Minute)},
		{App: "worker", State: "stopped", StartedAt: now.Add(-3 * time.Hour)},
		{App: "jobs", State: "error(7)", StartedAt: now.Add(-5 * 24 * time.Hour)},
	}
	var output bytes.Buffer
	if err := Render(&output, rows, false, now); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSuffix(output.String(), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("output=%q", output.String())
	}
	for _, column := range []string{"STATE", "PID", "PORT", "AGE"} {
		want := strings.Index(lines[0], column)
		if want < 0 {
			t.Fatalf("missing %s in %q", column, lines[0])
		}
		for _, line := range lines[1:] {
			if len(line) <= want || line[want] == ' ' {
				t.Fatalf("column %s is not aligned in %q", column, line)
			}
		}
	}
	if !strings.Contains(lines[1], "3000,8080") || !strings.Contains(lines[2], "-      -") {
		t.Fatalf("output=%q", output.String())
	}
	for _, noise := range []string{"started=", "exited=", "exit=", "sub="} {
		if strings.Contains(output.String(), noise) {
			t.Fatalf("unexpected noise %q in %q", noise, output.String())
		}
	}
}

func TestRenderRemoteIncludesVersionColumn(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	var output bytes.Buffer
	err := Render(&output, []Row{{
		App: "api", State: "running", PID: "42", StartedAt: now.Add(-8 * time.Minute), Version: "abc123",
	}}, true, now)
	if err != nil {
		t.Fatal(err)
	}
	if got := output.String(); !strings.Contains(got, "APP") || !strings.Contains(got, "VERSION") || !strings.Contains(got, "abc123") {
		t.Fatalf("output=%q", got)
	}
}

func TestFormatAgeUsesKubernetesStyleUnits(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		started time.Time
		want    string
	}{
		{"missing", time.Time{}, "-"},
		{"future", now.Add(time.Second), "0s"},
		{"seconds", now.Add(-12 * time.Second), "12s"},
		{"minutes", now.Add(-8 * time.Minute), "8m"},
		{"hours", now.Add(-6 * time.Hour), "6h"},
		{"days", now.Add(-5 * 24 * time.Hour), "5d"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatAge(tt.started, now); got != tt.want {
				t.Fatalf("FormatAge()=%q want=%q", got, tt.want)
			}
		})
	}
}
