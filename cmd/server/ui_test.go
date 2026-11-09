package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/appthrust/ai-gateway/cmd/server/internal/provider"
	"github.com/appthrust/appserver-go/accesskey"
	"github.com/appthrust/appserver-go/aipolicy"
	"github.com/appthrust/appserver-go/appserver/identity"
	"github.com/stretchr/testify/require"
)

type uiAuth struct {
	caller identity.Identity
	err    error
}

func (a uiAuth) Authenticate(context.Context, string) (identity.Identity, string, error) {
	return a.caller, "Bearer caller", a.err
}

func TestOperationalAPIAuthenticationAndNoInvocationSideEffects(t *testing.T) {
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/ai-gateway/v1/status"},
		{http.MethodGet, "/api/ai-gateway/v1/providers"},
		{http.MethodPost, "/api/ai-gateway/v1/providers/openai/check"},
		{http.MethodGet, "/api/ai-gateway/v1/invocations"},
	} {
		t.Run(route.path, func(t *testing.T) {
			g := &gateway{auth: testAuth{}, providers: &registry{}, policy: aipolicy.Document{Mode: aipolicy.ModePending}}
			p := testRecorder(t, recorderTransport(func(*http.Request) (*http.Response, error) {
				t.Error("operational read enqueued an invocation")
				return recorderResponse(202), nil
			}))
			g.recorder = p
			w := httptest.NewRecorder()
			g.handler().ServeHTTP(w, httptest.NewRequest(route.method, route.path, nil))
			require.Equal(t, http.StatusUnauthorized, w.Code)
			require.Contains(t, w.Body.String(), "ai_unauthorized")
			w = callGateway(g, route.method, route.path, "")
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			require.True(t, g.providers.healthObserved.IsZero())
			require.False(t, g.providers.healthStarted)
			require.Empty(t, g.invocationsRing.page(invocationFilter{}).Items)
			closeRecorder(t, p)
			require.Zero(t, p.DeliveredCount())
			g.auth = uiAuth{err: identity.ErrUnavailable}
			w = callGateway(g, route.method, route.path, "")
			require.Equal(t, http.StatusServiceUnavailable, w.Code)
			require.Contains(t, w.Body.String(), "identity_unavailable")
		})
	}
	g := &gateway{auth: uiAuth{err: identity.ErrUnavailable}, providers: &registry{}}
	w := callGateway(g, http.MethodGet, "/v1/models", "")
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.Contains(t, w.Body.String(), "identity_unavailable")
	require.Empty(t, g.invocationsRing.page(invocationFilter{}).Items)
}

func TestConfigAndStaticFallback(t *testing.T) {
	t.Setenv("AI_GATEWAY_PUBLIC_URL", "https://gateway.example")
	t.Setenv("PLATFORM_DASHBOARD_URL", "")
	t.Setenv("AI_GATEWAY_STATIC_DIR", "")
	require.Equal(t, "/srv/ai-gateway", defaultStaticDir())
	dir := t.TempDir()
	t.Setenv("AI_GATEWAY_STATIC_DIR", dir)
	require.Equal(t, dir, defaultStaticDir())
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>gateway SPA</html>"), 0600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "assets"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("export {}"), 0600))
	g := &gateway{auth: testAuth{}, providers: &registry{}, projectID: "pj-test", staticDir: dir}
	handler := g.handler()
	for _, path := range []string{"/", "/providers", "/playground", "/invocations", "/connect"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, w.Code)
		require.Contains(t, w.Body.String(), "gateway SPA")
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	}
	for _, path := range []string{"/assets/missing.js", "/api/missing", "/v1/missing", "/api", "/v1"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusNotFound, w.Code)
		require.NotContains(t, w.Body.String(), "gateway SPA")
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	require.Equal(t, "public, max-age=31536000, immutable", w.Header().Get("Cache-Control"))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/providers", nil))
	require.Equal(t, http.StatusMethodNotAllowed, w.Code)
	for _, dashboard := range []string{"", "https://dashboard.example"} {
		t.Setenv("PLATFORM_DASHBOARD_URL", dashboard)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/config", nil))
		require.Equal(t, http.StatusOK, w.Code)
		var config map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &config))
		require.Equal(t, "pj-test", config["projectId"])
		require.Equal(t, "https://gateway.example", config["publicUrl"])
		if dashboard == "" {
			require.NotContains(t, config, "dashboardUrl")
		} else {
			require.Equal(t, dashboard, config["dashboardUrl"])
		}
	}
	var logs bytes.Buffer
	old := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(old)
	g.staticDir = filepath.Join(dir, "missing")
	handler = g.handler()
	for range 2 {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		require.Equal(t, http.StatusNotFound, w.Code)
	}
	require.Equal(t, 1, strings.Count(logs.String(), "SPA is unavailable"))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	require.Equal(t, http.StatusOK, w.Code)
}

func TestInvocationRouteCapturesCallerAndStream(t *testing.T) {
	p := &fakeProvider{name: "test", defaultModel: "good", complete: func(context.Context, provider.Request, bool) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: http.NoBody}, nil
	}}
	g := testGateway(p, aipolicy.Document{Mode: aipolicy.ModeUnrestricted})
	g.auth = uiAuth{caller: identity.Identity{Principal: accesskey.Principal{Subject: "member"}, Name: "Jane"}}
	g.recorder = testRecorder(t, recorderTransport(func(*http.Request) (*http.Response, error) { return recorderResponse(202), nil }))
	w := callGateway(g, http.MethodPost, "/v1/chat/completions", `{"stream":true,"messages":[]}`)
	require.NotEmpty(t, w.Header().Get("X-Request-ID"))
	closeRecorder(t, g.recorder)
	w = callGateway(g, http.MethodGet, "/api/ai-gateway/v1/invocations?caller=me", "")
	var page invocationPage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	require.Len(t, page.Items, 1)
	e := page.Items[0]
	require.Equal(t, invocationCaller{Subject: "member", DisplayName: "Jane"}, e.Caller)
	require.Equal(t, "chat", e.Route)
	require.True(t, e.Stream)
	require.Equal(t, "Delivered", e.Report.State)
	require.Equal(t, aipolicy.OutcomeError, e.Outcome) // Empty SSE is interrupted, not an invented success.
	require.Equal(t, "ai_provider_error", e.Reason)
	g.auth = uiAuth{caller: identity.Identity{Principal: accesskey.Principal{Subject: "other"}}}
	w = callGateway(g, http.MethodGet, "/api/ai-gateway/v1/invocations?caller=me", "")
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	require.Empty(t, page.Items)
}
