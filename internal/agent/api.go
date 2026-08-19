package agent

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	_ = api.RenderJSON(w, api.Response{Success: true, Message: "ok"})
}

func (s *Server) handleListApps(w http.ResponseWriter, _ *http.Request) {
	if _, err := os.ReadDir(s.AppRoot); err != nil {
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
	_ = api.RenderJSON(w, api.Response{Success: true, Data: names})
}

func (s *Server) handleApps(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/apps/")
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
			_ = api.RenderJSON(w, api.Response{Success: true, Message: fmt.Sprintf("deployed %s (placeholder)", name)})
		case http.MethodDelete:
			_ = api.RenderJSON(w, api.Response{Success: true, Message: fmt.Sprintf("removed %s (placeholder)", name)})
		default:
			_ = api.RenderJSONError(w, http.StatusMethodNotAllowed, "unsupported method")
		}
		return
	}

	sub := strings.Join(parts[1:], "/")
	ctrl := runtime.Controller{}
	switch r.Method {
	case http.MethodPost:
		switch sub {
		case "start":
			_ = ctrl.Start(name)
			_ = api.RenderJSON(w, api.Response{Success: true, Message: "start requested"})
		case "stop":
			_ = ctrl.Stop(name)
			_ = api.RenderJSON(w, api.Response{Success: true, Message: "stop requested"})
		case "restart":
			_ = ctrl.Restart(name)
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
			lines := []string{
				fmt.Sprintf("nova runtime for %s at %s", name, nextRequestID()),
			}
			_ = api.RenderJSON(w, api.Response{Success: true, Data: lines})
		default:
			_ = api.RenderJSONError(w, http.StatusNotFound, "unknown endpoint")
		}
	default:
		_ = api.RenderJSONError(w, http.StatusMethodNotAllowed, "unsupported method")
	}
}

func nextRequestID() string {
	buf := make([]byte, 4)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
