package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"

	"github.com/Al-Pragliola/ai-pipelines/internal/dashboard"
	"github.com/Al-Pragliola/ai-pipelines/internal/issuehistory"
)

//go:embed dist/*
var frontendFS embed.FS

func main() {
	addr := flag.String("addr", ":9090", "listen address")
	historyDBPath := flag.String("history-db-path", "/var/lib/ai-pipelines/history.db",
		"path to the SQLite database for tracking completed issues")
	logFile := flag.String("log-file", "",
		"path to operator log file (fallback when controller is not running as a pod)")
	flag.Parse()

	// Strip the "dist/" prefix so files are served from root
	frontend, err := fs.Sub(frontendFS, "dist")
	if err != nil {
		log.Fatalf("failed to create sub fs: %v", err)
	}

	history, err := issuehistory.New(*historyDBPath)
	if err != nil {
		log.Fatalf("failed to open history database: %v", err)
	}
	defer history.Close() //nolint:errcheck

	srv, err := dashboard.NewServer(frontend, history, *logFile)
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}

	fmt.Printf("Dashboard listening on %s\n", *addr)
	if err := http.ListenAndServe(*addr, srv.Handler()); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
