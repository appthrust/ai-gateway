package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/appthrust/ai-gateway/cmd/server/internal/provider"
	"github.com/appthrust/appserver-go/aipolicy"
	"github.com/appthrust/appserver-go/apphealth"
	"github.com/appthrust/appserver-go/appserver/health"
	"github.com/appthrust/appserver-go/appserver/identity"
)

type testAuth struct{}

func (testAuth) Authenticate(_ context.Context, header string) (identity.Identity, string, error) {
	if header != "Bearer caller" {
		return identity.Identity{}, "", errors.New("bad bearer")
	}
	return identity.Identity{}, header, nil
}

type fakeProvider struct {
	name         string
	defaultModel string
	models       []provider.ModelEntry
	complete     func(context.Context, provider.Request, bool) (*http.Response, error)
}

func (p *fakeProvider) Name() string         { return p.name }
func (p *fakeProvider) DefaultModel() string { return p.defaultModel }
func (p *fakeProvider) Models(context.Context) ([]provider.ModelEntry, error) {
	return p.models, nil
}
func (p *fakeProvider) Probe(context.Context) (int, error) {
	return http.StatusOK, nil
}
func (p *fakeProvider) ChatCompletions(ctx context.Context, in provider.Request, stream bool) (*http.Response, error) {
	return p.complete(ctx, in, stream)
}

func jsonResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func testGateway(p provider.Provider, document aipolicy.Document) *gateway {
	return &gateway{auth: testAuth{}, policy: document, providers: &registry{static: []provider.Provider{p}, client: provider.NewClient()}}
}

func callGateway(g *gateway, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer caller")
	r.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	g.handler().ServeHTTP(w, r)
	return w
}

func enforced(conversation string) aipolicy.Document {
	return aipolicy.Document{APIVersion: aipolicy.APIVersion, Mode: aipolicy.ModeEnforced,
		Policy:    &aipolicy.PolicyRef{Group: "ee.appthrust.io", Kind: "AgentPolicy", Name: "restricted", UID: "uid-1", Generation: 3},
		Providers: []aipolicy.Provider{{Name: "test", ModelRules: []aipolicy.ModelRule{{Patterns: []string{"good-*"}}, {Patterns: []string{"good-model"}}}}},
		Logging:   &aipolicy.Logging{Conversation: conversation, ToolCalls: "Off"}}
}

func TestPolicyAllowDenyAndDefaults(t *testing.T) {
	for _, tc := range []struct {
		name                          string
		document                      aipolicy.Document
		body                          string
		defaultProvider, defaultModel string
		status                        int
		code, wantModel               string
	}{
		{"unrestricted adapter default", aipolicy.Document{Mode: aipolicy.ModeUnrestricted}, `{"messages":[]}`, "", "", http.StatusOK, "", "adapter-default"},
		{"unrestricted override", aipolicy.Document{Mode: aipolicy.ModeUnrestricted}, `{"messages":[]}`, "test", "override", http.StatusOK, "", "override"},
		{"unrestricted explicit unchanged", aipolicy.Document{Mode: aipolicy.ModeUnrestricted}, `{"model":"anything","messages":[]}`, "", "", http.StatusOK, "", "anything"},
		{"enforced default", enforced("Off"), `{"messages":[]}`, "", "", http.StatusOK, "", "good-model"},
		{"enforced auto", enforced("Off"), `{"model":"auto","messages":[]}`, "", "", http.StatusOK, "", "good-model"},
		{"enforced explicit", enforced("Off"), `{"provider":"test","model":"good-model","messages":[]}`, "", "", http.StatusOK, "", "good-model"},
		{"enforced denied model", enforced("Off"), `{"model":"bad-model","messages":[]}`, "", "", http.StatusForbidden, "ai_model_denied", ""},
		{"enforced denied provider", enforced("Off"), `{"provider":"other","model":"good-model","messages":[]}`, "", "", http.StatusForbidden, "ai_model_denied", ""},
		{"enforced empty rules", aipolicy.Document{Mode: aipolicy.ModeEnforced}, `{"messages":[]}`, "", "", http.StatusForbidden, "ai_model_denied", ""},
		{"pending", aipolicy.Document{Mode: aipolicy.ModePending}, `{"messages":[]}`, "", "", http.StatusForbidden, "ai_policy_pending", ""},
		{"full", enforced("Full"), `{"messages":[]}`, "", "", http.StatusServiceUnavailable, "ai_policy_logging_unavailable", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			p := &fakeProvider{name: "test", defaultModel: "adapter-default", complete: func(_ context.Context, in provider.Request, stream bool) (*http.Response, error) {
				calls++
				var model string
				_ = json.Unmarshal(in.Body["model"], &model)
				if model != tc.wantModel || stream || in.Accept != "application/json" {
					t.Errorf("upstream model=%q stream=%t accept=%q", model, stream, in.Accept)
				}
				if _, found := in.Body["provider"]; found {
					t.Error("gateway routing field leaked upstream")
				}
				return jsonResponse(`{"choices":[{"message":{"content":"answer"}}]}`), nil
			}}
			g := testGateway(p, tc.document)
			g.defaultProvider, g.defaultModel = tc.defaultProvider, tc.defaultModel
			w := callGateway(g, http.MethodPost, "/v1/chat/completions", tc.body)
			if w.Code != tc.status {
				t.Fatalf("status=%d body=%s", w.Code, w.Body)
			}
			if tc.code != "" && !strings.Contains(w.Body.String(), `"code":"`+tc.code+`"`) {
				t.Fatalf("error=%s", w.Body)
			}
			if (tc.status == http.StatusOK) != (calls == 1) {
				t.Fatalf("unexpected provider calls: %d", calls)
			}
		})
	}
}

func captureRecords(t *testing.T, g *gateway, status int) <-chan aipolicy.InvocationRecord {
	t.Helper()
	records := make(chan aipolicy.InvocationRecord, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/projects/pj-test/ai-invocations" || r.Header.Get("Authorization") != "Bearer caller" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("invocation request: %s %s %v", r.Method, r.URL, r.Header)
		}
		var record aipolicy.InvocationRecord
		if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
			t.Error(err)
		}
		records <- record
		w.WriteHeader(status)
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(context.Background())
	g.recorder = newRecorder(ctx, server.URL, "pj-test", server.Client().Transport)
	t.Cleanup(func() {
		cancel()
		closeRecorder(t, g.recorder)
	})
	return records
}

func digest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestUnauthenticatedRequestsAreNotRecorded(t *testing.T) {
	for _, tc := range []struct {
		method, path string
	}{
		{http.MethodGet, "/v1/models"},
		{http.MethodPost, "/v1/chat/completions"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			var hits atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				hits.Add(1)
				w.WriteHeader(http.StatusForbidden)
			}))
			t.Cleanup(server.Close)
			ctx, cancel := context.WithCancel(context.Background())
			g := testGateway(&fakeProvider{name: "test"}, enforced("Off"))
			g.recorder = newRecorder(ctx, server.URL, "pj-test", server.Client().Transport)
			t.Cleanup(func() {
				cancel()
				closeRecorder(t, g.recorder)
			})

			w := httptest.NewRecorder()
			g.handler().ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"messages":[]}`)))
			if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), `"code":"ai_unauthorized"`) {
				t.Fatalf("response=%d %s", w.Code, w.Body)
			}
			closeRecorder(t, g.recorder)
			if hits.Load() != 0 || g.recorder.DeliveredCount() != 0 || g.recorder.DroppedCount(dropRejected) != 0 {
				t.Fatalf("hits=%d delivered=%d rejected=%d", hits.Load(), g.recorder.DeliveredCount(), g.recorder.DroppedCount(dropRejected))
			}
			if !g.providers.healthObserved.IsZero() {
				t.Fatal("unauthenticated request recorded a health observation")
			}
		})
	}
}

func TestAuthenticatedPolicyDenialIsRecorded(t *testing.T) {
	g := testGateway(&fakeProvider{name: "test"}, enforced("Off"))
	records := captureRecords(t, g, http.StatusAccepted)
	w := callGateway(g, http.MethodPost, "/v1/chat/completions", `{"provider":"test","model":"bad-model","messages":[]}`)
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), `"code":"ai_model_denied"`) {
		t.Fatalf("response=%d %s", w.Code, w.Body)
	}
	closeRecorder(t, g.recorder)
	select {
	case record := <-records:
		if record.Provider != "test" || record.Model != "bad-model" || record.Status != http.StatusForbidden || record.Reason != "ai_model_denied" || record.Outcome != aipolicy.OutcomeDenied {
			t.Fatalf("denial record=%+v", record)
		}
	default:
		t.Fatal("authenticated policy denial was not reported")
	}
	if g.recorder.DeliveredCount() != 1 || g.recorder.DroppedCount(dropRejected) != 0 {
		t.Fatalf("delivered=%d rejected=%d", g.recorder.DeliveredCount(), g.recorder.DroppedCount(dropRejected))
	}
}

func TestInvocationRecordsHashOnlyAndOff(t *testing.T) {
	for _, mode := range []string{"HashOnly", "Off"} {
		t.Run(mode, func(t *testing.T) {
			p := &fakeProvider{name: "test", complete: func(context.Context, provider.Request, bool) (*http.Response, error) {
				return jsonResponse(`{"model":"good-model","choices":[{"message":{"content":"hello world"}}],"usage":{"prompt_tokens":7,"completion_tokens":3}}`), nil
			}}
			g := testGateway(p, enforced(mode))
			records := captureRecords(t, g, http.StatusAccepted)
			w := callGateway(g, http.MethodPost, "/v1/chat/completions", `{"messages":[{"role":"system","content":"system"},{"role":"user","content":[{"type":"text","text":"hello"},{"type":"text","text":" world"}]}]}`)
			if w.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", w.Code, w.Body)
			}
			record := <-records
			if record.Provider != "test" || record.Model != "good-model" || record.Outcome != aipolicy.OutcomeAllowed || record.Status != http.StatusOK || record.Policy == nil || record.Policy.UID != "uid-1" || record.Usage == nil || record.Usage.PromptTokens != 7 || record.Usage.CompletionTokens != 3 || record.DurationMs < 0 {
				t.Fatalf("record=%+v", record)
			}
			if mode == "HashOnly" {
				if record.RequestDigest != digest("systemhello world") || record.CompletionDigest != digest("hello world") {
					t.Fatalf("digests=%+v", record)
				}
			} else if record.RequestDigest != "" || record.CompletionDigest != "" {
				t.Fatalf("Off leaked digests: %+v", record)
			}
		})
	}
}

func TestStreamingPassThroughUsageAndDigests(t *testing.T) {
	stream := ": keepalive\r\n\r\ndata: {\"model\":\"good-model\",\"choices\":[{\"delta\":{\"content\":\"hello \"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"world\"}}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":2}}\n\ndata: [DONE]\n\n"
	p := &fakeProvider{name: "test", complete: func(_ context.Context, in provider.Request, streaming bool) (*http.Response, error) {
		if !streaming || string(in.Body["stream_options"]) != `{"include_usage":true}` {
			t.Errorf("upstream streaming options=%s", in.Body["stream_options"])
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream))}, nil
	}}
	g := testGateway(p, enforced("HashOnly"))
	records := captureRecords(t, g, http.StatusAccepted)
	w := callGateway(g, http.MethodPost, "/v1/chat/completions", `{"messages":[{"content":"question"}],"stream":true,"stream_options":{"include_usage":false}}`)
	if w.Code != http.StatusOK || w.Body.String() != stream || !w.Flushed || w.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("stream status=%d body=%q flushed=%t", w.Code, w.Body, w.Flushed)
	}
	record := <-records
	if record.Outcome != aipolicy.OutcomeAllowed || record.Usage == nil || record.Usage.PromptTokens != 4 || record.Usage.CompletionTokens != 2 || record.RequestDigest != digest("question") || record.CompletionDigest != digest("hello world") {
		t.Fatalf("stream record=%+v", record)
	}
}

func TestErrorsAndReportingFailureDoNotLeakBodies(t *testing.T) {
	for _, tc := range []struct {
		name, body           string
		upstreamStatus, want int
		reason               string
	}{
		{"upstream rejection", `{"model":"good-model"}`, http.StatusTooManyRequests, http.StatusBadGateway, "ai_provider_error"},
		{"denied", `{"model":"bad-model"}`, http.StatusOK, http.StatusForbidden, "ai_model_denied"},
		{"trailing JSON", `{} {}`, http.StatusOK, http.StatusBadRequest, "ai_invalid_request"},
		{"invalid model type", `{"model":42}`, http.StatusOK, http.StatusBadRequest, "ai_invalid_request"},
		{"too large", `{"model":"` + strings.Repeat("x", maxRequestBytes) + `"}`, http.StatusOK, http.StatusRequestEntityTooLarge, "ai_request_too_large"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &fakeProvider{name: "test", complete: func(context.Context, provider.Request, bool) (*http.Response, error) {
				r := jsonResponse(`{"error":"provider-secret"}`)
				r.StatusCode = tc.upstreamStatus
				return r, nil
			}}
			g := testGateway(p, enforced("HashOnly"))
			records := captureRecords(t, g, http.StatusInternalServerError)
			w := callGateway(g, http.MethodPost, "/v1/chat/completions", tc.body)
			if w.Code != tc.want || !strings.Contains(w.Body.String(), tc.reason) || strings.Contains(w.Body.String(), "provider-secret") {
				t.Fatalf("response=%d %s", w.Code, w.Body)
			}
			record := <-records
			if record.Status != tc.want || record.Reason != tc.reason || record.Outcome == aipolicy.OutcomeAllowed {
				t.Fatalf("error record=%+v", record)
			}
		})
	}
}

func TestModelsFilteringAndUnion(t *testing.T) {
	g := testGateway(&fakeProvider{name: "test", models: []provider.ModelEntry{
		provider.NewModelEntry("good-model", "test"), provider.NewModelEntry("other", "test"),
	}}, enforced("Off"))
	g.policy.Providers[0].ModelRules = []aipolicy.ModelRule{{Patterns: []string{"good-*"}}, {Patterns: []string{"good-model", "bad", "good-*", "good-model"}}}
	g.policy.Providers = append(g.policy.Providers, aipolicy.Provider{Name: "second", ModelRules: []aipolicy.ModelRule{{Patterns: []string{"second-model"}}}})
	for _, tc := range []struct {
		mode aipolicy.Mode
		ids  []string
	}{
		{aipolicy.ModeEnforced, []string{"good-model"}},
		{aipolicy.ModeUnrestricted, []string{"good-model", "other"}},
	} {
		g.policy.Mode = tc.mode
		w := callGateway(g, http.MethodGet, "/v1/models", "")
		var list struct {
			Object string `json:"object"`
			Data   []struct {
				ID      string `json:"id"`
				OwnedBy string `json:"owned_by"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil || w.Code != http.StatusOK || list.Object != "list" {
			t.Fatalf("models=%d %s err=%v", w.Code, w.Body, err)
		}
		ids := []string{}
		for _, model := range list.Data {
			ids = append(ids, model.ID)
			if model.OwnedBy == "" {
				t.Error("missing provider ownership")
			}
		}
		if !reflect.DeepEqual(ids, tc.ids) {
			t.Fatalf("models=%v want=%v", ids, tc.ids)
		}
	}
}

func TestPendingAndFullCoverEveryAIRoute(t *testing.T) {
	for _, document := range []aipolicy.Document{{Mode: aipolicy.ModePending}, enforced("Full")} {
		g := testGateway(&fakeProvider{name: "test"}, document)
		for method, paths := range map[string][]string{http.MethodGet: {"/v1/models"}, http.MethodPost: {"/v1/chat/completions", "/v1/embeddings"}} {
			for _, path := range paths {
				w := callGateway(g, method, path, "invalid JSON")
				if w.Code != http.StatusForbidden && w.Code != http.StatusServiceUnavailable {
					t.Fatalf("mode=%s route=%s status=%d", document.Mode, path, w.Code)
				}
			}
		}
	}
}

func TestAuthenticationAndProbes(t *testing.T) {
	for _, key := range []string{"RELEASE_VERSION", "SOURCE_REVISION", "ARTIFACT_DIGEST", "RELEASE_FILE"} {
		t.Setenv("AI_GATEWAY_"+key, "")
	}
	g := testGateway(&fakeProvider{name: "test"}, aipolicy.Document{Mode: aipolicy.ModeUnrestricted})
	for _, path := range []string{"/healthz", "/readyz", "/releasez", "/v1/models"} {
		w := httptest.NewRecorder()
		g.handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		want := http.StatusOK
		if path == "/v1/models" {
			want = http.StatusUnauthorized
		} else if path == "/releasez" {
			want = http.StatusServiceUnavailable
		}
		if w.Code != want {
			t.Fatalf("route=%s status=%d", path, w.Code)
		}
	}
}

func TestMissingProviderAndUnsupportedEmbeddings(t *testing.T) {
	g := &gateway{auth: testAuth{}, policy: aipolicy.Document{Mode: aipolicy.ModeUnrestricted}, providers: &registry{}}
	w := callGateway(g, http.MethodPost, "/v1/chat/completions", `{"messages":[]}`)
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "ai_provider_unavailable") {
		t.Fatalf("missing provider=%d %s", w.Code, w.Body)
	}
	g.providers.static = []provider.Provider{&fakeProvider{name: "test", defaultModel: "m"}}
	w = callGateway(g, http.MethodPost, "/v1/embeddings", `{"input":"hello"}`)
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "ai_unsupported") {
		t.Fatalf("unsupported embeddings=%d %s", w.Code, w.Body)
	}
}

type readAfterEvent struct {
	event string
	reads int
}

func (r *readAfterEvent) Read(p []byte) (int, error) {
	r.reads++
	if r.event == "" {
		return 0, errors.New("stream read after terminal event")
	}
	n := copy(p, r.event)
	r.event = r.event[n:]
	return n, nil
}

func TestTerminalStreamEventStopsReadingAndIncompleteStreamIsRecorded(t *testing.T) {
	for _, tc := range []struct {
		name, event string
		outcome     aipolicy.Outcome
	}{
		{"done", "data: [DONE]\n\n", aipolicy.OutcomeAllowed},
		{"incomplete", "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n", aipolicy.OutcomeError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &readAfterEvent{event: tc.event}
			p := &fakeProvider{name: "test", complete: func(context.Context, provider.Request, bool) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(reader)}, nil
			}}
			g := testGateway(p, enforced("Off"))
			records := captureRecords(t, g, http.StatusAccepted)
			w := callGateway(g, http.MethodPost, "/v1/chat/completions", `{"stream":true,"messages":[]}`)
			record := <-records
			if w.Code != http.StatusOK || record.Status != http.StatusOK || record.Outcome != tc.outcome {
				t.Fatalf("stream status=%d record=%+v", w.Code, record)
			}
			if tc.outcome == aipolicy.OutcomeAllowed && reader.reads != 1 {
				t.Fatalf("terminal event caused extra reads: %d", reader.reads)
			}
			if tc.outcome == aipolicy.OutcomeError && strings.Contains(w.Body.String(), "[DONE]") {
				t.Fatal("incomplete stream must not manufacture success")
			}
		})
	}
}

func TestInvocationReportingDoesNotBlockHandler(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := &fakeProvider{name: "test", complete: func(context.Context, provider.Request, bool) (*http.Response, error) {
			return jsonResponse(`{"choices":[{"message":{"content":"answer"}}]}`), nil
		}}
		g := testGateway(p, enforced("Off"))
		release := make(chan struct{})
		g.recorder = testRecorder(t, recorderTransport(func(r *http.Request) (*http.Response, error) {
			select {
			case <-release:
				return recorderResponse(http.StatusAccepted), nil
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
		}))
		started := time.Now()
		w := callGateway(g, http.MethodPost, "/v1/chat/completions", `{"messages":[]}`)
		if w.Code != http.StatusOK || w.Body.String() != `{"choices":[{"message":{"content":"answer"}}]}` {
			t.Fatalf("response=%d %s", w.Code, w.Body)
		}
		if time.Since(started) != 0 {
			t.Fatal("handler waited for the reporting HTTP attempt")
		}
		synctest.Wait()
		if g.recorder.DeliveredCount() != 0 {
			t.Fatal("reporting receiver unexpectedly completed before release")
		}
		close(release)
		closeRecorder(t, g.recorder)
		if g.recorder.DeliveredCount() != 1 {
			t.Fatal("record was not delivered after the handler returned")
		}
	})
}

func TestApplicationHealthPolicyDenialAndProviderAbsence(t *testing.T) {
	g := testGateway(&fakeProvider{name: "test"}, enforced("Off"))
	requireStatus := func(w *httptest.ResponseRecorder, want int) {
		t.Helper()
		if w.Code != want {
			t.Fatalf("status=%d body=%s", w.Code, w.Body)
		}
	}
	requireStatus(callGateway(g, http.MethodPost, "/v1/chat/completions", `{"model":"denied","messages":[]}`), http.StatusForbidden)
	if status := g.checkHealth(context.Background()); status.State != apphealth.StateDegraded || status.Reason != apphealth.ReasonUpstreamDenied {
		t.Fatalf("policy denial=%+v", status)
	}
	g.providers = &registry{}
	if status := g.checkHealth(context.Background()); status.State != apphealth.StateFailing || status.Reason != apphealth.ReasonDependencyUnavailable {
		t.Fatalf("missing provider=%+v", status)
	}
	h := g.handler()
	for _, header := range []string{"", apphealth.PublicRouteHeader, "Authorization"} {
		r := httptest.NewRequest(http.MethodGet, health.Path, nil)
		if header != "" {
			r.Header.Set(header, "public")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		want := http.StatusOK
		if header != "" {
			want = http.StatusNotFound
		}
		requireStatus(w, want)
	}
}

func TestApplicationHealthActiveProbeAndTrafficOverride(t *testing.T) {
	var modelCalls atomic.Int32
	var chatStatus atomic.Int32
	chatStatus.Store(http.StatusOK)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "" {
			t.Error("caller credential reached the internal provider")
		}
		switch req.URL.Path {
		case "/v1/models":
			modelCalls.Add(1)
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, "private provider diagnostic")
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(int(chatStatus.Load()))
			_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"hello"}}]}`)
		default:
			t.Error("unexpected provider route")
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer backend.Close()
	client := provider.NewClient()
	defer client.CloseIdleConnections()
	g := testGateway(provider.NewCapability("test", backend.URL, client), aipolicy.Document{Mode: aipolicy.ModeUnrestricted})
	handler := g.handler()
	requireProbes := func() {
		t.Helper()
		for _, path := range []string{"/healthz", "/readyz"} {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
			if w.Code != http.StatusOK || w.Body.String() != "{\"status\":\"ok\"}\n" {
				t.Fatalf("%s status=%d body=%s", path, w.Code, w.Body)
			}
		}
	}
	requireProbes()
	if modelCalls.Load() != 0 {
		t.Fatal("liveness/readiness probed an upstream provider")
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, health.Path, nil))
	var report apphealth.Report
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil || len(report.Capabilities) != 1 {
		t.Fatalf("health report=%s err=%v", w.Body, err)
	}
	capability := report.Capabilities[0]
	if capability.State != apphealth.StateFailing || capability.Reason != apphealth.ReasonUpstreamDenied ||
		capability.Message != "Provider rejected the configured credential" {
		t.Fatalf("active health=%+v", capability)
	}
	requireProbes()
	for _, code := range []int{http.StatusOK, http.StatusServiceUnavailable, http.StatusOK} {
		chatStatus.Store(int32(code))
		response := callGateway(g, http.MethodPost, "/v1/chat/completions", `{"messages":[{"role":"user","content":"hello"}]}`)
		wantHTTP, wantState := http.StatusOK, apphealth.StateServing
		if code != http.StatusOK {
			wantHTTP, wantState = http.StatusBadGateway, apphealth.StateFailing
		}
		if response.Code != wantHTTP {
			t.Fatalf("chat status=%d body=%s", response.Code, response.Body)
		}
		if got := g.checkHealth(context.Background()); got.State != wantState || modelCalls.Load() != 1 {
			t.Fatalf("traffic health=%+v model calls=%d", got, modelCalls.Load())
		}
	}
}
