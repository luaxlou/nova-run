package api

import (
	"encoding/json"
	"net/http"
)

// TypeMeta describes an individual object in an API response or request
// with strings representing the type of the object and its API schema version.
type TypeMeta struct {
	Kind       string `json:"kind,omitempty" yaml:"kind,omitempty"`
	APIVersion string `json:"apiVersion,omitempty" yaml:"apiVersion,omitempty"`
}

// ObjectMeta is metadata that all persisted resources must have.
type ObjectMeta struct {
	Name        string            `json:"name,omitempty" yaml:"name,omitempty"`
	Labels      map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

// --- Deployment (App) ---

// Deployment represents an application deployment.
// It maps to the concept of an "App" in Glow.
type Deployment struct {
	TypeMeta   `json:",inline" yaml:",inline"`
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Spec       AppSpec   `json:"spec,omitempty" yaml:"spec,omitempty"`
	Status     AppStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

// AppSpec defines the desired state of an application.
// Replaces old AppSpec/StartAppRequest mix.
type AppSpec struct {
	Command     string            `json:"command" yaml:"command"`
	Args        []string          `json:"args,omitempty" yaml:"args,omitempty"`
	WorkingDir  string            `json:"workingDir,omitempty" yaml:"workingDir,omitempty"`
	Env         map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	Port        int               `json:"port,omitempty" yaml:"port,omitempty"`
	Domain      string            `json:"domain,omitempty" yaml:"domain,omitempty"`
	Replicas    int               `json:"replicas,omitempty" yaml:"replicas,omitempty"` // For future scaling
	AutoRestart bool              `json:"autoRestart,omitempty" yaml:"autoRestart,omitempty"`
	Config      map[string]any    `json:"config,omitempty" yaml:"config,omitempty"`
}

// AppStatus defines the observed state of an application.
type AppStatus struct {
	Phase        string   `json:"phase,omitempty" yaml:"phase,omitempty"` // RUNNING, STOPPED, ERROR
	Pid          int      `json:"pid,omitempty" yaml:"pid,omitempty"`
	RestartCount int      `json:"restartCount,omitempty" yaml:"restartCount,omitempty"`
	Stats        AppStats `json:"stats,omitempty" yaml:"stats,omitempty"`
	ConfigHash   string   `json:"configHash,omitempty" yaml:"configHash,omitempty"`
	BinaryHash   string   `json:"binaryHash,omitempty" yaml:"binaryHash,omitempty"`
}

// AppStateResponse represents the state hashes of an application
type AppStateResponse struct {
	ConfigHash string `json:"configHash"`
	BinaryHash string `json:"binaryHash"`
}

// --- Node (Host) ---

// Node represents a host machine.
type Node struct {
	TypeMeta   `json:",inline" yaml:",inline"`
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Status     NodeStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

type NodeStatus struct {
	Hostname  string        `json:"hostname,omitempty" yaml:"hostname,omitempty"`
	OS        string        `json:"os,omitempty" yaml:"os,omitempty"`
	Arch      string        `json:"arch,omitempty" yaml:"arch,omitempty"`
	Kernel    string        `json:"kernel,omitempty" yaml:"kernel,omitempty"`
	CPUUsage  float64       `json:"cpuUsage,omitempty" yaml:"cpuUsage,omitempty"`
	MemUsage  float64       `json:"memUsage,omitempty" yaml:"memUsage,omitempty"`   // Percent
	DiskUsage float64       `json:"diskUsage,omitempty" yaml:"diskUsage,omitempty"` // Percent
	Resources []ResourceRef `json:"resources,omitempty" yaml:"resources,omitempty"`
}

type ResourceRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	Port int    `json:"port,omitempty"`
}

// --- Ingress ---

type Ingress struct {
	TypeMeta   `json:",inline" yaml:",inline"`
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Spec       IngressSpec `json:"spec,omitempty" yaml:"spec,omitempty"`
}

type IngressSpec struct {
	Domain  string `json:"domain" yaml:"domain"`
	Service string `json:"service" yaml:"service"` // App Name
	Port    int    `json:"port" yaml:"port"`
}

// --- Config ---

type Config struct {
	TypeMeta   `json:",inline" yaml:",inline"`
	ObjectMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Data       map[string]any `json:"data,omitempty" yaml:"data,omitempty"`
}

// --- Legacy & Shared Types ---

type AppStats struct {
	CPUPercent   float64 `json:"cpu_percent"`
	MemoryUsage  uint64  `json:"memory_usage"` // bytes
	IOReadBytes  uint64  `json:"io_read_bytes"`
	IOWriteBytes uint64  `json:"io_write_bytes"`
}

// Helper types for API communication (Requests/Responses)
type Response struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

// Deprecated: StartAppRequest (Migrate to Deployment)
type StartAppRequest struct {
	Name        string            `json:"name"`
	Command     string            `json:"command"`
	Args        []string          `json:"args"`
	WorkingDir  string            `json:"working_dir"`
	Env         map[string]string `json:"env"`
	Config      map[string]any    `json:"config"`
	Domain      string            `json:"domain"`
	AutoRestart bool              `json:"auto_restart"`
	SkipIngress bool              `json:"skip_ingress,omitempty"`
}

// Deprecated: StopAppRequest
type StopAppRequest struct {
	Name        string `json:"name"`
	KeepIngress bool   `json:"keep_ingress,omitempty"`
}

// Deprecated: AppInfo (Use Deployment.Status instead)
type AppInfo struct {
	Name         string            `json:"name"`
	Command      string            `json:"command"`
	Args         []string          `json:"args"`
	WorkingDir   string            `json:"working_dir"`
	Env          map[string]string `json:"env"`
	Config       map[string]any    `json:"config"`
	Port         int               `json:"port"`
	Domain       string            `json:"domain"`
	AutoRestart  bool              `json:"auto_restart"`
	RestartCount int               `json:"restart_count"`
	StartTime    int64             `json:"start_time"` // Unix timestamp
	Status       string            `json:"status"`
	Pid          int               `json:"pid"`
	Stats        AppStats          `json:"stats"`
	ConfigHash   string            `json:"configHash,omitempty"`
	BinaryHash   string            `json:"binaryHash,omitempty"`
}

type ProvisionRequest struct {
	AppName      string `json:"app_name"`
	ResourceType string `json:"resource_type"`
	ResourceName string `json:"resource_name"`
}

type IngressUpdateRequest struct {
	AppName string `json:"app_name"`
	Domain  string `json:"domain"`
	Port    int    `json:"port"`
}

type IngressDeleteRequest struct {
	AppName string `json:"app_name"`
}

// --- System Resource Configs ---

// DatabaseInfo stores information about a database
type DatabaseInfo struct {
	Name    string `json:"name" yaml:"name"`
	Charset string `json:"charset" yaml:"charset"`
}

// MySQLConfig stores the collected MySQL configuration
type MySQLConfig struct {
	Host      string         `json:"host" yaml:"host"`
	Port      int            `json:"port" yaml:"port"`
	User      string         `json:"user" yaml:"user"`
	Password  string         `json:"password" yaml:"password"`
	Databases []DatabaseInfo `json:"databases" yaml:"databases"`
	UpdatedAt interface{}    `json:"updated_at" yaml:"updated_at"`
}

// RedisConfig stores the collected Redis configuration
type RedisConfig struct {
	Host      string      `json:"host" yaml:"host"`
	Port      int         `json:"port" yaml:"port"`
	Password  string      `json:"password" yaml:"password"`
	UpdatedAt interface{} `json:"updated_at" yaml:"updated_at"`
}

// NginxSystemConfig stores the collected Nginx configuration
type NginxSystemConfig struct {
	BinaryPath string      `json:"binary_path" yaml:"binary_path"`
	ConfPath   string      `json:"conf_path" yaml:"conf_path"`
	Version    string      `json:"version" yaml:"version"`
	UpdatedAt  interface{} `json:"updated_at" yaml:"updated_at"`
}

// ServerInfo contains information about the glow-server instance
type ServerInfo struct {
	PID         int    `json:"pid"`
	DataDir     string `json:"data_dir"`
	LogDir      string `json:"log_dir"`
	ConfigPath  string `json:"config_path"`
	Version     string `json:"version"`
	Uptime      int64  `json:"uptime"` // uptime in seconds
}

// Host Manifest (Old) - Keep for backward compatibility or refactor
type Host struct {
	TypeMeta `yaml:",inline"`
	Metadata ObjectMeta `yaml:"metadata" json:"metadata"`
	Spec     HostSpec   `yaml:"spec" json:"spec"`
}

type HostSpec struct {
	PublicIP string                 `yaml:"publicIP" json:"publicIP"`
	Services map[string]ServiceSpec `yaml:"services" json:"services"`
}

type ServiceSpec struct {
	Port          int    `yaml:"port" json:"port"`
	AdminUser     string `yaml:"adminUser" json:"adminUser"`
	AdminPassword string `yaml:"adminPassword" json:"adminPassword"`
}

// App Manifest (Old)
type App struct {
	TypeMeta `yaml:",inline"`
	Metadata ObjectMeta `yaml:"metadata" json:"metadata"`
	Spec     AppSpecOld `yaml:"spec" json:"spec"`
}

type AppSpecOld struct {
	Binary       string             `yaml:"binary" json:"binary"`
	BinaryPath   string             `yaml:"binaryPath,omitempty" json:"binaryPath,omitempty"` // Local path for upload
	Command      string             `yaml:"command" json:"command"`
	Args         []string           `yaml:"args" json:"args"`
	WorkingDir   string             `yaml:"workingDir" json:"workingDir"`
	Domain       string             `yaml:"domain" json:"domain"`
	Dependencies map[string]DepSpec `yaml:"dependencies" json:"dependencies"`
}

type DepSpec struct {
	DBName string `yaml:"dbName,omitempty" json:"dbName,omitempty"`
	DB     int    `yaml:"db,omitempty" json:"db,omitempty"`
}

type TCPAction string

const (
	ActionGetConfig TCPAction = "get_config"
	ActionProvision TCPAction = "provision"
	ActionRegister  TCPAction = "register"
	ActionAppStart  TCPAction = "app_start"
)

type TCPRequest struct {
	Action  TCPAction       `json:"action"`
	AppName string          `json:"app_name"`
	APIKey  string          `json:"api_key"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

func RenderJSON(w http.ResponseWriter, response any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func RenderJSONError(w http.ResponseWriter, code int, message string) {
	w.WriteHeader(code)
	RenderJSON(w, Response{
		Success: false,
		Message: message,
	})
}
