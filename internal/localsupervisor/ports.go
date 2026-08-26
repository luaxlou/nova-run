package localsupervisor

import (
	"sort"
	"strings"

	gopsutilnet "github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

func discoverListeningPorts(pid int) []int {
	if pid <= 0 {
		return nil
	}
	root, err := process.NewProcess(int32(pid))
	if err != nil {
		return nil
	}
	seen := make(map[int32]bool)
	var ports []int
	var visit func(*process.Process)
	visit = func(current *process.Process) {
		if current == nil || seen[current.Pid] {
			return
		}
		seen[current.Pid] = true
		connections, _ := current.Connections()
		ports = append(ports, listeningPortsFromConnections(connections)...)
		children, _ := current.Children()
		for _, child := range children {
			visit(child)
		}
	}
	visit(root)
	return normalizePorts(ports)
}

func listeningPortsFromConnections(connections []gopsutilnet.ConnectionStat) []int {
	ports := make([]int, 0, len(connections))
	for _, connection := range connections {
		if strings.EqualFold(connection.Status, "LISTEN") {
			ports = append(ports, int(connection.Laddr.Port))
		}
	}
	return normalizePorts(ports)
}

func normalizePorts(ports []int) []int {
	sort.Ints(ports)
	normalized := make([]int, 0, len(ports))
	previous := -1
	for _, port := range ports {
		if port <= 0 || port == previous {
			continue
		}
		normalized = append(normalized, port)
		previous = port
	}
	return normalized
}
