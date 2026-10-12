package agent

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/luaxlou/glow-ops/internal/artifact"
	"github.com/luaxlou/glow-ops/internal/authz"
	"github.com/luaxlou/glow-ops/internal/deploy"
	"github.com/luaxlou/glow-ops/internal/runtime"
	"github.com/luaxlou/glow-ops/pkg/api"
)

type Server struct {
	AppRoot        string
	AuthStore      *authz.Store
	restartService func(string) error
}

func NewProjectServer(appRoot, authDir string) *Server {
	cleanRoot := filepath.Clean(appRoot)
	if err := runtime.EnsureAppServiceTemplate(cleanRoot); err != nil {
		log.Printf("nova app service template setup failed: %v", err)
	}
	store := &authz.Store{Dir: filepath.Clean(authDir)}
	return &Server{AppRoot: cleanRoot, AuthStore: store}
}

func (s *Server) restart(name string) error {
	if s.restartService != nil {
		return s.restartService(name)
	}
	ctrl := runtime.Controller{}
	return ctrl.Restart(name)
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)

	if s.AuthStore != nil {
		mux.HandleFunc("/v2/projects/", s.handleProjectAPI)
	}
	return mux
}

func (s *Server) handleProjectAPI(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v2/projects/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] != "apps" {
		api.RenderJSONError(w, http.StatusNotFound, "unknown endpoint")
		return
	}
	project := parts[0]
	if err := authz.ValidateProjectName(project); err != nil {
		api.RenderJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if token := ParseBearerToken(r); token == "" || !s.AuthStore.Verify(project, token) {
		api.RenderJSONError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	if len(parts) == 2 {
		if r.Method != http.MethodGet {
			api.RenderJSONError(w, http.StatusMethodNotAllowed, "unsupported method")
			return
		}
		s.handleProjectListApps(w, project)
		return
	}
	if len(parts) > 4 {
		api.RenderJSONError(w, http.StatusNotFound, "unknown endpoint")
		return
	}
	app := parts[2]
	if err := deploy.ValidateAppName(app); err != nil || strings.Contains(app, "--") {
		api.RenderJSONError(w, http.StatusBadRequest, "invalid app name")
		return
	}
	sub := ""
	if len(parts) == 4 {
		sub = parts[3]
	}
	s.handleScopedApp(w, r, project+"--"+app, app, sub)
}

func (s *Server) handleProjectListApps(w http.ResponseWriter, project string) {
	prefix := project + "--"
	names := []string{}
	items, err := os.ReadDir(s.AppRoot)
	if os.IsNotExist(err) {
		api.RenderJSON(w, api.Response{Success: true, Data: names})
		return
	}
	if err != nil {
		api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, item := range items {
		if item.IsDir() && strings.HasPrefix(item.Name(), prefix) {
			names = append(names, strings.TrimPrefix(item.Name(), prefix))
		}
	}
	sort.Strings(names)
	api.RenderJSON(w, api.Response{Success: true, Data: names})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.RenderJSONError(w, http.StatusMethodNotAllowed, "health endpoint uses GET")
		return
	}
	api.RenderJSON(w, api.Response{Success: true, Data: map[string]string{"status": "ok"}})
}

func (s *Server) handleScopedApp(w http.ResponseWriter, r *http.Request, runtimeName, displayName, sub string) {
	if sub == "" {
		switch r.Method {
		case http.MethodPut:
			s.handleDeployAs(w, r, runtimeName, displayName)
		case http.MethodDelete:
			s.handleDeleteAs(w, r, runtimeName, displayName)
		default:
			api.RenderJSONError(w, http.StatusMethodNotAllowed, "unsupported method")
		}
		return
	}

	ctrl := runtime.Controller{}
	switch r.Method {
	case http.MethodPost:
		switch sub {
		case "start":
			if !artifact.HasRunBinary(filepath.Join(s.AppRoot, runtimeName)) {
				api.RenderJSONError(w, http.StatusBadRequest, "app is static and has no service command")
				return
			}
			if err := ctrl.Start(runtimeName); err != nil {
				api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			api.RenderJSON(w, api.Response{Success: true, Message: "start requested"})
		case "stop":
			if err := ctrl.Stop(runtimeName); err != nil {
				api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			api.RenderJSON(w, api.Response{Success: true, Message: "stop requested"})
		case "restart":
			if !artifact.HasRunBinary(filepath.Join(s.AppRoot, runtimeName)) {
				api.RenderJSONError(w, http.StatusBadRequest, "app is static and has no service command")
				return
			}
			if err := ctrl.Restart(runtimeName); err != nil {
				api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			api.RenderJSON(w, api.Response{Success: true, Message: "restart requested"})
		default:
			api.RenderJSONError(w, http.StatusNotFound, "unknown endpoint")
		}
	case http.MethodGet:
		switch sub {
		case "status":
			status, err := resolveAppStatus(s.AppRoot, runtimeName, ctrl.Status)
			if err != nil {
				api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			api.RenderJSON(w, api.Response{Success: true, Data: status})
		case "logs":
			query := r.URL.Query()
			lines := 100
			if raw := query.Get("lines"); raw != "" {
				if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
					lines = parsed
				}
			}
			if strings.EqualFold(query.Get("follow"), "1") || strings.EqualFold(query.Get("follow"), "true") {
				s.handleLogsFollow(w, r, runtimeName, lines)
				return
			}
			cmd := runtime.JournalTailCommand(runtimeName, false, lines)
			output, err := cmd.CombinedOutput()
			if err != nil && len(output) == 0 {
				api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			lineText := strings.Split(strings.TrimRight(string(output), "\n"), "\n")
			if len(lineText) == 1 && lineText[0] == "" {
				lineText = []string{}
			}
			api.RenderJSON(w, api.Response{Success: true, Data: lineText})
		default:
			api.RenderJSONError(w, http.StatusNotFound, "unknown endpoint")
		}
	default:
		api.RenderJSONError(w, http.StatusMethodNotAllowed, "unsupported method")
	}
}

func (s *Server) handleLogsFollow(w http.ResponseWriter, r *http.Request, name string, lines int) {
	ctx := r.Context()
	if lines <= 0 {
		lines = 100
	}
	cmd := runtime.JournalTailCommand(name, true, lines)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := cmd.Start(); err != nil {
		api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		_ = cmd.Process.Kill()
		api.RenderJSONError(w, http.StatusInternalServerError, "response writer does not support streaming")
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	stream := streamWriter{writer: w, flusher: flusher}
	go func() {
		defer func() { _ = stderr.Close() }()
		s := bufio.NewScanner(stderr)
		for s.Scan() {
			_, _ = stream.Write(append(s.Bytes(), '\n'))
		}
	}()
	go func() {
		<-ctx.Done()
		_ = cmd.Process.Kill()
	}()

	_, _ = io.Copy(stream, stdout)
	_ = stdout.Close()
	_ = cmd.Wait()
}

type streamWriter struct {
	writer  io.Writer
	flusher http.Flusher
}

func (s streamWriter) Write(p []byte) (int, error) {
	n, err := s.writer.Write(p)
	if s.flusher != nil {
		s.flusher.Flush()
	}
	return n, err
}

func (s *Server) handleDeploy(w http.ResponseWriter, r *http.Request, name string) {
	s.handleDeployAs(w, r, name, name)
}

func (s *Server) handleDeployAs(w http.ResponseWriter, r *http.Request, name, displayName string) {
	if r.Method != http.MethodPut {
		api.RenderJSONError(w, http.StatusMethodNotAllowed, "unsupported method")
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		api.RenderJSONError(w, http.StatusBadRequest, "invalid multipart payload")
		return
	}
	file, _, err := r.FormFile("artifact")
	if err != nil {
		api.RenderJSONError(w, http.StatusBadRequest, "artifact file missing in form field `artifact`")
		return
	}
	defer file.Close()

	tmp, err := os.CreateTemp("", "nova-artifact-*.tar.gz")
	if err != nil {
		api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	if _, err := io.Copy(tmp, file); err != nil {
		api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	version := strings.TrimSpace(r.FormValue("version"))
	if version == "" {
		api.RenderJSONError(w, http.StatusBadRequest, "version field is required")
		return
	}

	appDir := filepath.Join(s.AppRoot, name)
	if err := os.MkdirAll(s.AppRoot, 0o755); err != nil {
		api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if current, ok, err := deploy.CurrentVersion(s.AppRoot, name); err != nil {
		api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	} else if ok && current == version {
		api.RenderJSON(w, api.Response{
			Success: true,
			Message: fmt.Sprintf("app %s already at version %s", displayName, version),
			Data: map[string]any{
				"app":     displayName,
				"version": version,
				"skipped": true,
				"message": "already latest",
			},
		})
		return
	}
	if err := deploy.ReplaceArtifact(appDir, tmp.Name()); err != nil {
		api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := deploy.SaveMetadata(appDir, deploy.Metadata{Version: version}); err != nil {
		api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if artifact.HasRunBinary(appDir) {
		if err := s.restart(name); err != nil {
			api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	api.RenderJSON(w, api.Response{
		Success: true,
		Message: fmt.Sprintf("app %s deployed version %s", displayName, version),
		Data: map[string]any{
			"app":     displayName,
			"version": version,
			"skipped": false,
			"message": "deployed",
		},
	})
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request, name string) {
	s.handleDeleteAs(w, r, name, name)
}

func (s *Server) handleDeleteAs(w http.ResponseWriter, r *http.Request, name, displayName string) {
	if r.Method != http.MethodDelete {
		api.RenderJSONError(w, http.StatusMethodNotAllowed, "unsupported method")
		return
	}
	ctrl := runtime.Controller{}
	if err := ctrl.Stop(name); err != nil {
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "could not be found") && !strings.Contains(msg, "not found") {
			api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	target := filepath.Join(s.AppRoot, name)
	if err := os.RemoveAll(target); err != nil {
		api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	api.RenderJSON(w, api.Response{Success: true, Message: fmt.Sprintf("app %s removed", displayName)})
}
