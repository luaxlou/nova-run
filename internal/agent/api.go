package agent

import (
	"fmt"
	"bufio"
	"io"
	"net/http"
	"os"
	"sort"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/luaxlou/glow-ops/internal/deploy"
	"github.com/luaxlou/glow-ops/internal/runtime"
	"github.com/luaxlou/glow-ops/pkg/api"
)

type Server struct {
	Token   string
	AppRoot string
}

func NewServer(appRoot, token string) *Server {
	return &Server{Token: token, AppRoot: filepath.Clean(appRoot)}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)

	protected := func(next http.HandlerFunc) http.HandlerFunc {
		return RequireToken(s.Token)(next)
	}

	mux.HandleFunc("/v1/apps", protected(s.handleListApps))
	mux.HandleFunc("/v1/apps/", protected(s.handleApps))
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		_ = api.RenderJSONError(w, http.StatusMethodNotAllowed, "health endpoint uses GET")
		return
	}
	_ = api.RenderJSON(w, api.Response{Success: true, Data: map[string]string{"status": "ok"}})
}

func (s *Server) handleListApps(w http.ResponseWriter, _ *http.Request) {
	items := []string{}
	if _, err := os.Stat(s.AppRoot); err != nil {
		if os.IsNotExist(err) {
			_ = api.RenderJSON(w, api.Response{Success: true, Data: items})
			return
		}
		_ = api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	items, err := os.ReadDir(s.AppRoot)
	if err != nil {
		_ = api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var names []string
	for _, item := range items {
		if item.IsDir() {
			names = append(names, item.Name())
		}
	}
	sort.Strings(names)
	_ = api.RenderJSON(w, api.Response{Success: true, Data: names})
}

func (s *Server) handleApps(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/apps/")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 || parts[0] == "" {
		_ = api.RenderJSONError(w, http.StatusBadRequest, "app name required")
		return
	}

	name := parts[0]
	if err := deploy.ValidateAppName(name); err != nil {
		_ = api.RenderJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodPut:
			s.handleDeploy(w, r, name)
		case http.MethodDelete:
			s.handleDelete(w, r, name)
		default:
			_ = api.RenderJSONError(w, http.StatusMethodNotAllowed, "unsupported method")
		}
		return
	}

	sub := parts[1]
	ctrl := runtime.Controller{}
	switch r.Method {
	case http.MethodPost:
		switch sub {
		case "start":
			if err := ctrl.Start(name); err != nil {
				_ = api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			_ = api.RenderJSON(w, api.Response{Success: true, Message: "start requested"})
		case "stop":
			if err := ctrl.Stop(name); err != nil {
				_ = api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			_ = api.RenderJSON(w, api.Response{Success: true, Message: "stop requested"})
		case "restart":
			if err := ctrl.Restart(name); err != nil {
				_ = api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			_ = api.RenderJSON(w, api.Response{Success: true, Message: "restart requested"})
		default:
			_ = api.RenderJSONError(w, http.StatusNotFound, "unknown endpoint")
		}
	case http.MethodGet:
		switch sub {
		case "status":
			status, err := ctrl.Status(name)
			if err != nil {
				_ = api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			_ = api.RenderJSON(w, api.Response{Success: true, Data: status})
		case "logs":
			query := r.URL.Query()
			lines := 100
			if raw := query.Get("lines"); raw != "" {
				if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
					lines = parsed
				}
			}
			if strings.EqualFold(query.Get("follow"), "1") || strings.EqualFold(query.Get("follow"), "true") {
				s.handleLogsFollow(w, r, name, lines)
				return
			}
			cmd := runtime.JournalTailCommand(name, false, lines)
			output, err := cmd.CombinedOutput()
			if err != nil && len(output) == 0 {
				_ = api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			lineText := strings.Split(strings.TrimRight(string(output), "\n"), "\n")
			if len(lineText) == 1 && lineText[0] == "" {
				lineText = []string{}
			}
			_ = api.RenderJSON(w, api.Response{Success: true, Data: lineText})
		default:
			_ = api.RenderJSONError(w, http.StatusNotFound, "unknown endpoint")
		}
	default:
		_ = api.RenderJSONError(w, http.StatusMethodNotAllowed, "unsupported method")
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
		_ = api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := cmd.Start(); err != nil {
		_ = api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		_ = cmd.Process.Kill()
		_ = api.RenderJSONError(w, http.StatusInternalServerError, "response writer does not support streaming")
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
	if r.Method != http.MethodPut {
		_ = api.RenderJSONError(w, http.StatusMethodNotAllowed, "unsupported method")
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		_ = api.RenderJSONError(w, http.StatusBadRequest, "invalid multipart payload")
		return
	}
	file, _, err := r.FormFile("artifact")
	if err != nil {
		_ = api.RenderJSONError(w, http.StatusBadRequest, "artifact file missing in form field `artifact`")
		return
	}
	defer file.Close()

	tmp, err := os.CreateTemp("", "nova-artifact-*.tar.gz")
	if err != nil {
		_ = api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	if _, err := io.Copy(tmp, file); err != nil {
		_ = api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	appDir := filepath.Join(s.AppRoot, name)
	if err := os.MkdirAll(s.AppRoot, 0o755); err != nil {
		_ = api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := deploy.ReplaceArtifact(appDir, tmp.Name()); err != nil {
		_ = api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = api.RenderJSON(w, api.Response{Success: true, Message: fmt.Sprintf("app %s deployed", name)})
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodDelete {
		_ = api.RenderJSONError(w, http.StatusMethodNotAllowed, "unsupported method")
		return
	}
	ctrl := runtime.Controller{}
	if err := ctrl.Stop(name); err != nil {
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "could not be found") && !strings.Contains(msg, "not found") {
			_ = api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	target := filepath.Join(s.AppRoot, name)
	if err := os.RemoveAll(target); err != nil {
		_ = api.RenderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = api.RenderJSON(w, api.Response{Success: true, Message: fmt.Sprintf("app %s removed", name)})
}
