package main

import (
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func defaultStaticDir() string {
	if dir := os.Getenv("AI_GATEWAY_STATIC_DIR"); dir != "" {
		return dir
	}
	return "/srv/ai-gateway"
}

func (g *gateway) config(w http.ResponseWriter, _ *http.Request) {
	value := map[string]string{"projectId": g.projectID, "publicUrl": os.Getenv("AI_GATEWAY_PUBLIC_URL")}
	if dashboard := os.Getenv("PLATFORM_DASHBOARD_URL"); dashboard != "" {
		value["dashboardUrl"] = dashboard
	}
	writeJSON(w, http.StatusOK, value)
}

func staticHandler(root string) http.Handler {
	if root == "" {
		root = defaultStaticDir()
	}
	if info, err := os.Stat(filepath.Join(root, "index.html")); err != nil || !info.Mode().IsRegular() {
		log.Print("AI gateway SPA is unavailable; continuing without web assets")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || r.URL.Path == "/v1" || strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/v1/") {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")))
		info, err := os.Stat(name)
		asset := strings.HasPrefix(r.URL.Path, "/assets/")
		if err != nil || !info.Mode().IsRegular() {
			if asset {
				http.NotFound(w, r)
				return
			}
			name = filepath.Join(root, "index.html")
		}
		file, err := os.Open(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		info, err = file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if filepath.Base(name) == "index.html" {
			w.Header().Set("X-Content-Type-Options", "nosniff")
		} else if asset {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.ServeContent(w, r, filepath.Base(name), info.ModTime(), file)
	})
}
