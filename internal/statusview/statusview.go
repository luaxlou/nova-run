package statusview

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

type Row struct {
	App       string
	State     string
	PID       string
	Ports     []int
	StartedAt time.Time
	Version   string
}

func Render(output io.Writer, rows []Row, includeVersion bool, now time.Time) error {
	w := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	header := "APP\tSTATE\tPID\tPORT\tAGE"
	if includeVersion {
		header += "\tVERSION"
	}
	if _, err := fmt.Fprintln(w, header); err != nil {
		return err
	}
	for _, row := range rows {
		pid := valueOrDash(row.PID)
		ports := formatPorts(row.Ports)
		version := valueOrDash(row.Version)
		if includeVersion {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", row.App, row.State, pid, ports, FormatAge(row.StartedAt, now), version)
		} else {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", row.App, row.State, pid, ports, FormatAge(row.StartedAt, now))
		}
	}
	return w.Flush()
}

func FormatAge(startedAt, now time.Time) string {
	if startedAt.IsZero() {
		return "-"
	}
	age := now.Sub(startedAt)
	if age < 0 {
		age = 0
	}
	switch {
	case age < time.Minute:
		return strconv.FormatInt(int64(age/time.Second), 10) + "s"
	case age < time.Hour:
		return strconv.FormatInt(int64(age/time.Minute), 10) + "m"
	case age < 24*time.Hour:
		return strconv.FormatInt(int64(age/time.Hour), 10) + "h"
	default:
		return strconv.FormatInt(int64(age/(24*time.Hour)), 10) + "d"
	}
}

func formatPorts(ports []int) string {
	if len(ports) == 0 {
		return "-"
	}
	normalized := append([]int(nil), ports...)
	sort.Ints(normalized)
	parts := make([]string, 0, len(normalized))
	previous := -1
	for _, port := range normalized {
		if port <= 0 || port == previous {
			continue
		}
		parts = append(parts, strconv.Itoa(port))
		previous = port
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ",")
}

func valueOrDash(value string) string {
	if strings.TrimSpace(value) == "" || value == "0" {
		return "-"
	}
	return value
}
