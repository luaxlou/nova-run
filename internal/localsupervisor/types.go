package localsupervisor

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
