package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/appthrust/ai-gateway/cmd/server/internal/provider"
	"github.com/appthrust/ai-gateway/cmd/server/internal/sse"
	"github.com/appthrust/appserver-go/aipolicy"
	"github.com/appthrust/appserver-go/apphealth"
	"github.com/appthrust/appserver-go/appserver/health"
	"github.com/appthrust/appserver-go/appserver/identity"
	"github.com/appthrust/appserver-go/appserver/releaseidentity"
)

const maxRequestBytes = 4 << 20

type authenticator interface {
	Authenticate(context.Context, string) (identity.Identity, string, error)
}

type gateway struct {
	auth            authenticator
	policy          aipolicy.Document
	providers       *registry
	defaultProvider string
	defaultModel    string
	recorder        *recorder
	invocationsRing invocationRing
	staticDir       string
	projectID       string
	launcher        http.Handler
}

type invocationHandler func(http.ResponseWriter, *http.Request, *aipolicy.InvocationRecord)

func (g *gateway) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/chat/completions", g.authenticated(g.complete))
	mux.HandleFunc("POST /v1/embeddings", g.authenticated(g.complete))
	mux.HandleFunc("GET /v1/models", g.authenticated(g.models))
	mux.HandleFunc("GET /config", g.config)
	mux.HandleFunc("GET /api/ai-gateway/v1/status", g.authenticatedRead(g.status))
	mux.HandleFunc("GET /api/ai-gateway/v1/providers", g.authenticatedRead(g.providerViews))
	mux.HandleFunc("POST /api/ai-gateway/v1/providers/{name}/check", g.authenticatedRead(g.checkProvider))
	mux.HandleFunc("GET /api/ai-gateway/v1/invocations", g.authenticatedRead(g.invocations))
	if g.launcher != nil {
		mux.Handle("GET /launcher", g.launcher)
	}
	mux.HandleFunc("/api", http.NotFound)
	mux.HandleFunc("/v1", http.NotFound)
	mux.HandleFunc("/api/", http.NotFound)
	mux.HandleFunc("/v1/", http.NotFound)
	mux.Handle("/", staticHandler(g.staticDir))
	mountProbes(mux)
	var applicationHealth health.Registry
	applicationHealth.Capability("chat-completions", true, health.CheckFunc(g.checkHealth))
	mux.Handle("GET "+health.Path, applicationHealth.Handler(releaseidentity.HealthIdentity("AI_GATEWAY")))
	return mux
}

func (g *gateway) authenticated(next invocationHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		record := aipolicy.InvocationRecord{Policy: g.policy.Policy, Outcome: aipolicy.OutcomeAllowed, Status: http.StatusOK}
		if logging := g.policy.Logging; logging != nil {
			record.Logging = &aipolicy.InvocationLogging{Conversation: logging.Conversation, ToolCalls: logging.ToolCalls}
		}
		caller, bearer, ok := g.authenticate(w, r)
		if !ok {
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), callerContextKey{}, caller))
		stream := false
		r = r.WithContext(context.WithValue(r.Context(), streamContextKey{}, &stream))
		route := "chat"
		if r.URL.Path == "/v1/embeddings" {
			route = "embeddings"
		}
		if r.URL.Path == "/v1/models" {
			route = "models"
		}
		id := g.invocationsRing.begin(caller, route, started)
		w.Header().Set("X-Request-ID", id)
		// Unauthenticated requests have no actor; the Platform API requires a Project member bearer.
		defer func() {
			record.DurationMs = time.Since(started).Milliseconds()
			g.invocationsRing.finish(id, record, stream)
			g.recorder.enqueue(bearer, record, invocationDelivery{id: id, report: g.invocationsRing.report})
			if r.URL.Path == "/v1/chat/completions" {
				g.recordHealth(record)
			}
		}()
		switch {
		case g.policy.Mode == aipolicy.ModePending:
			fail(w, &record, http.StatusForbidden, "ai_policy_pending", "The Project AI policy is not ready")
			return
		case g.policy.Mode == aipolicy.ModeEnforced && g.policy.Logging != nil && g.policy.Logging.Conversation == "Full":
			fail(w, &record, http.StatusServiceUnavailable, "ai_policy_logging_unavailable", "Full conversation logging is unavailable")
			return
		}
		next(w, r, &record)
	}
}

func (g *gateway) complete(w http.ResponseWriter, r *http.Request, record *aipolicy.InvocationRecord) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	decoder := json.NewDecoder(r.Body)
	var body map[string]json.RawMessage
	err := decoder.Decode(&body)
	if err == nil {
		var extra json.RawMessage
		if next := decoder.Decode(&extra); next != io.EOF {
			err = next
			if err == nil {
				err = errors.New("multiple JSON values")
			}
		}
	}
	if err != nil || body == nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			fail(w, record, http.StatusRequestEntityTooLarge, "ai_request_too_large", "AI request body must not exceed 4 MiB")
		} else {
			fail(w, record, http.StatusBadRequest, "ai_invalid_request", "AI request must be one JSON object")
		}
		return
	}
	var stream bool
	for _, field := range []struct {
		name   string
		target any
	}{
		{"provider", &record.Provider}, {"model", &record.Model}, {"stream", &stream},
	} {
		if value, ok := body[field.name]; ok && json.Unmarshal(value, field.target) != nil {
			fail(w, record, http.StatusBadRequest, "ai_invalid_request", "Invalid provider, model, or stream field")
			return
		}
	}
	if value, ok := r.Context().Value(streamContextKey{}).(*bool); ok {
		*value = stream
	}
	if record.Logging != nil && record.Logging.Conversation == "HashOnly" {
		record.RequestDigest = requestDigest(body)
	}
	if g.policy.Mode == aipolicy.ModeEnforced {
		g.resolveEnforced(record)
		if !g.policy.Allows(record.Provider, record.Model).Allowed || record.Model == "" {
			fail(w, record, http.StatusForbidden, "ai_model_denied", "The requested provider/model is not allowed")
			return
		}
	}
	selected, err := g.resolveProvider(r.Context(), record)
	if err != nil {
		fail(w, record, http.StatusBadGateway, "ai_provider_error", "Provider discovery failed")
		return
	}
	if selected == nil {
		fail(w, record, http.StatusServiceUnavailable, "ai_provider_unavailable", "No selected AI provider is available")
		return
	}
	delete(body, "provider")
	if record.Model != "" {
		body["model"], _ = json.Marshal(record.Model)
	} else {
		delete(body, "model")
	}
	if stream {
		options := map[string]json.RawMessage{}
		if raw, ok := body["stream_options"]; ok && string(raw) != "null" {
			if json.Unmarshal(raw, &options) != nil {
				fail(w, record, http.StatusBadRequest, "ai_invalid_request", "stream_options must be an object")
				return
			}
		}
		options["include_usage"] = json.RawMessage("true")
		body["stream_options"], _ = json.Marshal(options)
	}
	in := provider.Request{Body: body, Accept: r.Header.Get("Accept")}
	var response *http.Response
	if r.URL.Path == "/v1/embeddings" {
		embedder, ok := selected.(provider.Embedder)
		if !ok {
			fail(w, record, http.StatusNotFound, "ai_unsupported", "The selected provider does not support embeddings")
			return
		}
		response, err = embedder.Embeddings(r.Context(), in)
	} else {
		response, err = selected.ChatCompletions(r.Context(), in, stream)
	}
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		fail(w, record, http.StatusBadGateway, "ai_provider_error", "The AI provider could not complete this request")
		return
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		fail(w, record, http.StatusBadGateway, "ai_provider_error", "The AI provider rejected this request")
		return
	}
	observer := newObserver(record)
	defer observer.finish()
	if stream {
		g.proxyStream(w, response, observer)
		return
	}
	payload, err := io.ReadAll(response.Body)
	if err != nil || observer.observe(payload) != nil {
		fail(w, record, http.StatusBadGateway, "ai_provider_error", "The AI provider returned an invalid response")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	record.Status = response.StatusCode
	w.WriteHeader(response.StatusCode)
	if _, err := w.Write(payload); err != nil {
		record.Outcome, record.Reason = aipolicy.OutcomeError, "ai_response_interrupted"
	}
}

func (g *gateway) resolveEnforced(record *aipolicy.InvocationRecord) {
	decideEnforced(g.policy, record)
}

func (g *gateway) resolveProvider(ctx context.Context, record *aipolicy.InvocationRecord) (provider.Provider, error) {
	if g.policy.Mode == aipolicy.ModeUnrestricted && record.Provider == "" {
		record.Provider = g.defaultProvider
	}
	if p := g.providers.configured(record.Provider); p != nil {
		return decideProvider(g.policy.Mode, g.defaultProvider, g.defaultModel, []provider.Provider{p}, record), nil
	}
	candidates, err := g.providers.providers(ctx)
	if err != nil {
		return nil, err
	}
	return decideProvider(g.policy.Mode, g.defaultProvider, g.defaultModel, candidates, record), nil
}

func (g *gateway) proxyStream(w http.ResponseWriter, response *http.Response, observer *responseObserver) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		fail(w, observer.record, http.StatusBadGateway, "ai_provider_error", "Streaming is unavailable")
		return
	}
	if !strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		fail(w, observer.record, http.StatusBadGateway, "ai_provider_error", "The provider did not return an event stream")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	observer.record.Status = response.StatusCode
	w.WriteHeader(response.StatusCode)
	flusher.Flush()
	err := sse.Scan(response.Body, func(event, data []byte) error {
		if bytes.Equal(data, []byte("[DONE]")) {
			observer.done = true
		} else if len(data) != 0 {
			if err := observer.observe(data); err != nil {
				return err
			}
		}
		if _, err := w.Write(event); err != nil {
			return err
		}
		flusher.Flush()
		if observer.done {
			return io.EOF
		}
		return nil
	})
	if err != nil && !errors.Is(err, io.EOF) || !observer.done {
		// Headers have already been sent. End the stream without manufacturing
		// a [DONE] marker; the record preserves the actual HTTP status.
		observer.record.Outcome, observer.record.Reason = aipolicy.OutcomeError, "ai_provider_error"
	}
}

// models returns the available union. Provider failures are explicit partial
// results; discovery failures still reject the request because the provider set
// itself is unknown. Policy entries alone are not provider readiness evidence.
func (g *gateway) models(w http.ResponseWriter, r *http.Request, record *aipolicy.InvocationRecord) {
	models := []provider.ModelEntry{}
	failures := []map[string]string{}
	seen := make(map[string]bool)
	add := func(entry provider.ModelEntry) {
		if entry.ID != "" && !seen[entry.ID] {
			seen[entry.ID] = true
			models = append(models, entry)
		}
	}
	list := func(p provider.Provider) ([]provider.ModelEntry, bool) {
		entries, err := p.Models(r.Context())
		if err != nil {
			failures = append(failures, map[string]string{
				"provider": p.Name(), "code": "ai_provider_error", "message": "The AI provider could not list models",
			})
			return nil, false
		}
		return entries, true
	}
	if g.policy.Mode == aipolicy.ModeEnforced {
		for _, candidate := range g.policy.Providers {
			p, err := g.providers.lookup(r.Context(), candidate.Name)
			if err != nil {
				fail(w, record, http.StatusBadGateway, "ai_provider_error", "Provider discovery failed")
				return
			}
			if p == nil {
				continue
			}
			entries, ok := list(p)
			if !ok {
				continue
			}
			listed := make(map[string]provider.ModelEntry, len(entries))
			for _, entry := range entries {
				if _, exists := listed[entry.ID]; !exists {
					listed[entry.ID] = entry
				}
			}
			for _, rule := range candidate.ModelRules {
				for _, pattern := range rule.Patterns {
					if pattern == "" || seen[pattern] || strings.ContainsAny(pattern, "*?") || !g.policy.Allows(candidate.Name, pattern).Allowed {
						continue
					}
					entry, exists := listed[pattern]
					if !exists {
						entry = provider.NewModelEntry(pattern, candidate.Name)
					}
					add(entry)
				}
			}
		}
	} else {
		providers, err := g.providers.providers(r.Context())
		if err != nil {
			fail(w, record, http.StatusBadGateway, "ai_provider_error", "Provider discovery failed")
			return
		}
		for _, p := range providers {
			entries, ok := list(p)
			if !ok {
				continue
			}
			for _, entry := range entries {
				add(entry)
			}
		}
	}
	response := map[string]any{"object": "list", "data": models}
	if len(failures) != 0 {
		response["errors"] = failures
	}
	writeJSON(w, http.StatusOK, response)
}

func fail(w http.ResponseWriter, record *aipolicy.InvocationRecord, status int, code, message string) {
	record.Status, record.Reason, record.Outcome = status, code, aipolicy.OutcomeError
	if status == http.StatusForbidden || status == http.StatusUnauthorized || code == "ai_policy_logging_unavailable" {
		record.Outcome = aipolicy.OutcomeDenied
	}
	writeError(w, status, code, message)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (g *gateway) recordHealth(record aipolicy.InvocationRecord) {
	status := health.Status{State: apphealth.StateServing}
	switch record.Reason {
	case "ai_unauthorized", "ai_invalid_request", "ai_request_too_large", "ai_unsupported", "ai_response_interrupted":
		return
	case "ai_model_denied", "ai_policy_pending":
		status = health.Status{State: apphealth.StateDegraded, Reason: apphealth.ReasonUpstreamDenied}
	case "ai_provider_unavailable", "ai_policy_logging_unavailable":
		status = health.Status{State: apphealth.StateFailing, Reason: apphealth.ReasonDependencyUnavailable}
	default:
		if record.Outcome != aipolicy.OutcomeAllowed {
			status = providerHealthStatus(0, nil)
		}
	}
	g.providers.observeHealth(status)
}

func (g *gateway) checkHealth(ctx context.Context) health.Status {
	if status, blocked := decidePolicyHealth(g.policy); blocked {
		return status
	}
	// Health never retains caller credentials.
	return g.providers.healthStatus(ctx)
}
