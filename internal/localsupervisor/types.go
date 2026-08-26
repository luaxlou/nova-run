package localsupervisor

import "time"

const StateSchema = 1

const (
	PhaseStarting = "starting"
	PhaseRunning  = "running"
	PhaseStopping = "stopping"
	PhaseStopped  = "stopped"
	PhaseError    = "error"
)

type Target struct {
	ProjectPath string
	Name        string
	Dir         string
	Start       string
}

type Paths struct {
	Dir     string
	Lock    string
	State   string
	Socket  string
	Output  string
	Startup string
}

type State struct {
	Schema             int    `json:"schema"`
	ProjectPath        string `json:"projectPath"`
	App                string `json:"app"`
	Phase              string `json:"phase"`
	SupervisorPID      int    `json:"supervisorPid,omitempty"`
	AppPID             int    `json:"appPid,omitempty"`
	CommandFingerprint string `json:"commandFingerprint,omitempty"`
	StartedAt          string `json:"startedAt,omitempty"`
	ExitedAt           string `json:"exitedAt,omitempty"`
	ExitCode           *int   `json:"exitCode,omitempty"`
	Nonce              string `json:"nonce"`
}

type Startup struct {
	Schema    int           `json:"schema"`
	Target    Target        `json:"target"`
	Paths     Paths         `json:"paths"`
	Nonce     string        `json:"nonce"`
	StartedAt time.Time     `json:"startedAt"`
	StopGrace time.Duration `json:"stopGrace"`
}

type Status struct {
	App       string
	State     string
	PID       int
	StartedAt time.Time
	ExitedAt  time.Time
	ExitCode  *int
}

type readyMessage struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	State State  `json:"state,omitempty"`
}
