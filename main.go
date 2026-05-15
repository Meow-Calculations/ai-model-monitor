package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var (
	Version   = "dev"
	BuildTime = "unknown"
)

var staticDir string

func main() {
	exePath, _ := os.Executable()
	baseDir := filepath.Dir(exePath)

	dataDir := filepath.Join(baseDir, "data")
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		dataDir = "data"
	}
	os.MkdirAll(dataDir, 0755)

	// Initialize SQLite database
	if err := InitDB(dataDir); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer CloseDB()

	// Migrate existing config.yaml if present
	yamlPath := filepath.Join(baseDir, "config.yaml")
	if _, err := os.Stat(yamlPath); os.IsNotExist(err) {
		yamlPath = "config.yaml"
	}
	MigrateYAMLToDB(yamlPath)

	// Load config from SQLite
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	SaveConfig(cfg)

	staticDir = filepath.Join(baseDir, "static")
	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		staticDir = "static"
	}

	authToken := os.Getenv("AI_MODEL_MONITOR_TOKEN")
	mux := http.NewServeMux()
	registerAPIRoutes(mux, authToken)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			mux.ServeHTTP(w, r)
			return
		}
		serveStatic(w, r)
	})

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("AI Model Monitor starting on http://localhost%s", addr)
	log.Printf("Data directory: %s", dataDir)
	log.Printf("Static directory: %s", staticDir)
	if authToken != "" {
		log.Printf("API authentication enabled")
	}

	if cfg.AutoCheckInterval > 0 {
		go autoCheckLoop(cfg.AutoCheckInterval)
		log.Printf("Auto check enabled: every %d seconds", cfg.AutoCheckInterval)
	}

	server := &http.Server{Addr: addr, Handler: corsMiddleware(handler)}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down server...")
		server.Close()
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}

func serveStatic(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/" {
		path = "/index.html"
	}

	cleanPath := filepath.Clean(strings.TrimPrefix(path, "/"))
	filePath := filepath.Join(staticDir, cleanPath)

	if !strings.HasPrefix(filePath, staticDir) {
		http.NotFound(w, r)
		return
	}

	f, err := os.Open(filePath)
	if err != nil {
		indexPath := filepath.Join(staticDir, "index.html")
		indexFile, err2 := os.Open(indexPath)
		if err2 != nil {
			http.NotFound(w, r)
			return
		}
		defer indexFile.Close()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.Copy(w, indexFile)
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil || stat.IsDir() {
		indexPath := filepath.Join(staticDir, "index.html")
		indexFile, err2 := os.Open(indexPath)
		if err2 != nil {
			http.NotFound(w, r)
			return
		}
		defer indexFile.Close()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.Copy(w, indexFile)
		return
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".js":
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	case ".json":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".ico":
		w.Header().Set("Content-Type", "image/x-icon")
	}

	http.ServeContent(w, r, filepath.Base(filePath), stat.ModTime(), f)
}

func autoCheckLoop(intervalSeconds int) {
	ticker := time.NewTicker(time.Duration(intervalSeconds) * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		probingMu.Lock()
		if isProbing {
			probingMu.Unlock()
			continue
		}
		isProbing = true
		probingMu.Unlock()

		report := runProbe()
		latestReportMu.Lock()
		latestReport = report
		latestReportMu.Unlock()

		probingMu.Lock()
		isProbing = false
		probingMu.Unlock()
	}
}
