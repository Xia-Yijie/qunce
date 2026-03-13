package main

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

//go:embed embedded-dist/*
var embeddedFS embed.FS

func registerStaticRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/ws") {
			http.NotFound(w, r)
			return
		}

		if r.URL.Path == "" || r.URL.Path == "/" {
			serveSPAFile(w, r, "index.html")
			return
		}
		serveSPAFile(w, r, strings.TrimPrefix(r.URL.Path, "/"))
	})
}

func serveSPAFile(w http.ResponseWriter, r *http.Request, requestPath string) {
	requestPath = path.Clean("/" + requestPath)
	requestPath = strings.TrimPrefix(requestPath, "/")

	if requestPath == "" || strings.Contains(requestPath, "..") {
		requestPath = "index.html"
	}

	if data, err := fs.ReadFile(embeddedFS, filepath.ToSlash(filepath.Join("embedded-dist", requestPath))); err == nil {
		http.ServeContent(w, r, requestPath, time.Now(), bytes.NewReader(data))
		return
	}

	distPath := filepath.FromSlash(filepath.Join("console", "dist", requestPath))
	if fileInfo, err := os.Stat(distPath); err == nil && !fileInfo.IsDir() {
		http.ServeFile(w, r, distPath)
		return
	}

	if fileInfo, err := os.Stat(filepath.FromSlash(filepath.Join("console", "dist", "index.html"))); err == nil && !fileInfo.IsDir() {
		http.ServeFile(w, r, filepath.FromSlash(filepath.Join("console", "dist", "index.html")))
		return
	}

	if data, err := fs.ReadFile(embeddedFS, filepath.ToSlash(filepath.Join("embedded-dist", "index.html"))); err == nil {
		http.ServeContent(w, r, "index.html", time.Now(), bytes.NewReader(data))
		return
	}

	http.NotFound(w, r)
}
