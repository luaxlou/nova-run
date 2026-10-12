package agent

import (
	"net/http"
	"strings"
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
