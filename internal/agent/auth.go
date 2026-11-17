package agent

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/luaxlou/glow-ops/pkg/api"
)

func ParseBearerToken(r *http.Request) string {
	authz := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(strings.ToLower(authz), "bearer ") {
		return ""
	}
	parts := strings.Fields(authz)
	if len(parts) != 2 {
		return ""
	}
	return parts[1]
}

func RequireToken(expected string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if expected == "" {
				api.RenderJSONError(w, http.StatusServiceUnavailable, "agent token is not configured")
				return
			}
			got := ParseBearerToken(r)
			if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
				api.RenderJSONError(w, http.StatusUnauthorized, "invalid token")
				return
			}
			next(w, r)
		}
	}
}
