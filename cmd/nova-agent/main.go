package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/luaxlou/glow-ops/internal/agent"
)

func readToken(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}

func main() {
	listen := flag.String("listen", ":32102", "Nova agent listen address")
	appRoot := flag.String("app-root", "/var/lib/nova/apps", "Artifact storage root")
	tokenFile := flag.String("token-file", "/etc/nova-agent/token", "Agent bootstrap token file")
	flag.Parse()

	mux := agent.NewServer(*appRoot, readToken(*tokenFile)).Handler()
	srv := &http.Server{
		Addr:    *listen,
		Handler: mux,
	}
	log.Printf("nova-agent listening on %s (apps=%s)\n", *listen, filepath.Clean(*appRoot))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

