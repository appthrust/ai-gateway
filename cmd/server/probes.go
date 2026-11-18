package main

import (
	"net/http"

	"github.com/appthrust/appserver-go/aipolicy"
	"github.com/appthrust/appserver-go/appserver/releaseidentity"
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
	mux.Handle("GET /releasez", releaseidentity.NewHandler("AI_GATEWAY", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, http.StatusServiceUnavailable, aipolicy.OutcomeError, "release_unavailable", "AI gateway release status unavailable")
	}))
}
