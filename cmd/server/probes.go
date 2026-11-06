package main

import (
	"github.com/appthrust/appserver-go/appserver/releaseidentity"
	"net/http"
)

func mountProbes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	// Startup has already validated identity/policy configuration. Pending and
	// Full are ready to serve their intentional fail-closed policy responses.
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.Handle("GET /releasez", releaseidentity.NewHandler("AI_GATEWAY", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusServiceUnavailable, "release_unavailable", "AI gateway release status unavailable")
	}))
}
