package appcenter

import (
	"encoding/json"
	"fmt"
	"log"
	"net"

	"github.com/luaxlou/glow-ops/internal/configmanager"
	"github.com/luaxlou/glow-ops/internal/statemanager"
	"github.com/luaxlou/glow-ops/pkg/api"
	"github.com/shirou/gopsutil/v3/process"
)

var (
// activeConns sync.Map // map[string]net.Conn (AppName -> Conn) -- Replaced by store.go
)

// mergeAppInfo preserves persisted fields when the incoming payload is partial.
//
// This is critical because apps connecting via TCP may only report runtime fields
// (pid/port/status/stats) and omit lifecycle fields (command/env/workingDir/etc).
func mergeAppInfo(existing api.AppInfo, incoming api.AppInfo) api.AppInfo {
	merged := existing

	// Prefer incoming for "runtime" / frequently updated fields when present.
	if incoming.Status != "" {
		merged.Status = incoming.Status
	}
	if incoming.Pid != 0 {
		merged.Pid = incoming.Pid
	}
	if incoming.Port != 0 {
		merged.Port = incoming.Port
	}
	if incoming.StartTime != 0 {
		merged.StartTime = incoming.StartTime
	}
	// Stats is a struct; treat non-zero fields as updates but avoid clobbering if app sent zeroes.
	if incoming.Stats.CPUPercent != 0 {
		merged.Stats.CPUPercent = incoming.Stats.CPUPercent
	}
	if incoming.Stats.MemoryUsage != 0 {
		merged.Stats.MemoryUsage = incoming.Stats.MemoryUsage
	}
	if incoming.Stats.IOReadBytes != 0 {
		merged.Stats.IOReadBytes = incoming.Stats.IOReadBytes
	}
	if incoming.Stats.IOWriteBytes != 0 {
		merged.Stats.IOWriteBytes = incoming.Stats.IOWriteBytes
	}

	// Preserve important lifecycle/config fields unless incoming explicitly provides them.
	merged.Name = incoming.Name // same key; keep explicit

	if incoming.Command != "" {
		merged.Command = incoming.Command
	}
	if incoming.Args != nil {
		merged.Args = incoming.Args
	}
	if incoming.WorkingDir != "" {
		merged.WorkingDir = incoming.WorkingDir
	}
	if incoming.Env != nil {
		merged.Env = incoming.Env
	}
	if incoming.Config != nil {
		merged.Config = incoming.Config
	}
	if incoming.Domain != "" {
		merged.Domain = incoming.Domain
	}

	// AutoRestart is server-side config; only promote "true" from incoming.
	if incoming.AutoRestart {
		merged.AutoRestart = true
	}
	// RestartCount should not be reset by app payloads.
	if incoming.RestartCount != 0 {
		merged.RestartCount = incoming.RestartCount
	}

	if incoming.ConfigHash != "" {
		merged.ConfigHash = incoming.ConfigHash
	}
	if incoming.BinaryHash != "" {
		merged.BinaryHash = incoming.BinaryHash
	}

	return merged
}

// enrichAppInfoFromPID attempts to infer missing fields for legacy clients.
// Some app clients only report runtime fields and omit Command/WorkingDir.
func enrichAppInfoFromPID(appInfo *api.AppInfo) {
	if appInfo == nil || appInfo.Pid == 0 {
		return
	}
	// Only fill blanks; never override what the client explicitly provided.
	if appInfo.Command != "" && appInfo.WorkingDir != "" {
		return
	}
	p, err := process.NewProcess(int32(appInfo.Pid))
	if err != nil {
		return
	}
	if appInfo.Command == "" {
		if exe, err := p.Exe(); err == nil && exe != "" {
			appInfo.Command = exe
		}
	}
	if appInfo.WorkingDir == "" {
		if cwd, err := p.Cwd(); err == nil && cwd != "" {
			appInfo.WorkingDir = cwd
		}
	}
}

func Start(port int) error {

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}
	log.Printf("App Center TCP Server listening on :%d", port)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Printf("Accept error: %v", err)
				continue
			}
			go handleConnection(conn)
		}
	}()
	return nil

}

func Close() {

}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	var connectedAppName string
	defer func() {
		if connectedAppName != "" {
			UnregisterActiveApp(connectedAppName)
		}
	}()

	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	// Keep connection open for multiple requests?
	// Or one request per connection?
	// MQ style usually keeps connection open.
	for {
		var req api.TCPRequest
		if err := decoder.Decode(&req); err != nil {
			// EOF is expected when client closes
			return
		}

		// Security Check: Deny any request attempting to act as or query "nova-server"
		if req.AppName == "nova-server" {
			log.Printf("Security Alert: Attempt to access reserved app name 'nova-server' via TCP from %s", conn.RemoteAddr())
			resp := api.Response{Success: false, Message: "Access denied: 'nova-server' is a reserved system application."}
			encoder.Encode(resp)
			return // Close connection immediately
		}

		var resp api.Response
		switch req.Action {
		case api.ActionGetConfig:
			// Deprecated active get, but keeping for compatibility or direct config fetch
			config, err := configmanager.Get(req.AppName)
			if err != nil {
				// Not found is not necessarily an error to log loudly, but we return false
				resp = api.Response{Success: false, Message: err.Error()}
			} else {
				// No need to sanitize here if we strictly enforce AppName != nova-server
				// and assume app configs don't contain sensitive system keys.
				// But sanitizing is still a good defense-in-depth if we had other shared keys.
				// For now, based on user instruction "not a filter issue", we return as is.
				resp = api.Response{Success: true, Data: config}
			}

		case api.ActionRegister:
			var appInfo api.AppInfo
			if err := json.Unmarshal(req.Payload, &appInfo); err != nil {
				resp = api.Response{Success: false, Message: "Invalid payload"}
			} else {
				// Additional check inside payload
				if appInfo.Name == "nova-server" {
					resp = api.Response{Success: false, Message: "Registration denied: 'nova-server' is reserved."}
				} else {
					enrichAppInfoFromPID(&appInfo)
					// Merge with existing persisted info to avoid clobbering required fields.
					if existing, err := statemanager.GetApp(appInfo.Name); err == nil && existing != nil {
						appInfo = mergeAppInfo(*existing, appInfo)
					}

					if err := statemanager.SaveApp(appInfo); err != nil {
						resp = api.Response{Success: false, Message: err.Error()}
					} else {
						resp = api.Response{Success: true, Message: "Registered"}
					}
				}
			}

		case api.ActionAppStart:
			var appInfo api.AppInfo
			if err := json.Unmarshal(req.Payload, &appInfo); err != nil {
				resp = api.Response{Success: false, Message: "Invalid payload"}
			} else {
				// Additional check inside payload
				if appInfo.Name == "nova-server" {
					resp = api.Response{Success: false, Message: "Start denied: 'nova-server' is reserved."}
				} else {
					connectedAppName = appInfo.Name
					RegisterActiveApp(appInfo, conn)

					// 1. Record state (memory & local file state store)
					enrichAppInfoFromPID(&appInfo)
					if existing, err := statemanager.GetApp(appInfo.Name); err == nil && existing != nil {
						appInfo = mergeAppInfo(*existing, appInfo)
					}
					if err := statemanager.SaveApp(appInfo); err != nil {
						log.Printf("Error saving app state for %s: %v", appInfo.Name, err)
						// We continue even if save fails, to try to return config
					}

					// 2. Get config from local state store (via configmanager)
					config, err := configmanager.Get(appInfo.Name)
					if err != nil {
						// Config might not exist yet, which is fine
						log.Printf("No config found for app %s: %v", appInfo.Name, err)
						// We send empty success so client knows we handled the start
						resp = api.Response{Success: true, Message: "App started, no config found", Data: nil}
					} else {
						// 3. Pass config to application
						// Direct return, no sanitization needed as per new requirement
						resp = api.Response{Success: true, Data: config}
					}
				}
			}

		case api.ActionProvision:
			resp = HandleProvision(req)

		default:
			resp = api.Response{Success: false, Message: "Unknown action"}
		}

		if err := encoder.Encode(resp); err != nil {
			log.Printf("Encode error: %v", err)
			return
		}
	}
}
