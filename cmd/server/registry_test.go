package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/appthrust/ai-gateway/cmd/server/internal/provider"
	"github.com/appthrust/appserver-go/aigatewaydiscovery"
	"github.com/appthrust/appserver-go/aipolicy"
	"github.com/appthrust/appserver-go/apphealth"
	"github.com/appthrust/appserver-go/appserver/contract/chatcompletions"
	"github.com/appthrust/appserver-go/appserver/health"
	"github.com/appthrust/appserver-go/appserver/link"
	"github.com/appthrust/appserver-go/appserver/platformclient"
)

func TestCapabilityDiscoveryIsProjectCachedAndNeverForwardsBearer(t *testing.T) {
	var upstreamCalls, platformCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "" || r.Header.Get("Accept") != "application/json" {
			t.Errorf("internal provider request=%s headers=%v", r.URL, r.Header)
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if _, ok := body["model"]; ok {
			t.Errorf("capability default must omit model: %s", body["model"])
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"model":"local-default","choices":[{"message":{"content":"hello"}}]}`)
	}))
	defer upstream.Close()
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		platformCalls.Add(1)
		if r.URL.Path != aigatewaydiscovery.Path || r.URL.Query().Get("projectId") != "pj-test" || r.Header.Get("Authorization") != "Bearer workload-key" {
			t.Errorf("discovery request=%s headers=%v", r.URL, r.Header)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"links": []any{
			map[string]any{"definitionId": "not-ready", "state": "Preparing", "capabilities": []string{chatcompletions.Capability}},
			map[string]any{"definitionId": "llm", "state": "Ready", "capabilities": []string{chatcompletions.Capability}, "internalUrl": upstream.URL},
			map[string]any{"definitionId": "not-chat", "state": "Ready", "capabilities": []string{"other"}, "internalUrl": upstream.URL},
		}})
	}))
	defer platform.Close()
	resolver := link.NewResolver(platformclient.NewGatewayDiscovery(platform.URL, "pj-test", "workload-key"), "ai-gateway", nil, nil)
	g := &gateway{auth: testAuth{}, policy: aipolicy.Document{Mode: aipolicy.ModeUnrestricted}, providers: &registry{
		resolver: resolver, client: provider.NewClient(), static: []provider.Provider{&fakeProvider{name: "cloud", defaultModel: "cloud-model"}},
	}}
	records := captureRecords(t, g, http.StatusAccepted)
	for range 2 {
		w := callGateway(g, "POST", "/v1/chat/completions", `{"messages":[{"content":"hi"}]}`)
		if w.Code != 200 {
			t.Fatalf("response=%d %s", w.Code, w.Body)
		}
		record := <-records
		if record.Provider != "llm" || record.Model != "local-default" || record.RequestDigest != "" || record.CompletionDigest != "" {
			t.Fatalf("default record=%+v", record)
		}
	}
	if platformCalls.Load() != 1 || upstreamCalls.Load() != 2 {
		t.Fatalf("calls platform=%d provider=%d", platformCalls.Load(), upstreamCalls.Load())
	}
}

func TestCapabilityModelMetadataFilteringAndDedupe(t *testing.T) {
	const entry = `{"id":"good-model","object":"model","created":0,"owned_by":"appthrust","context_window":16384,"appthrust.io/maxPromptTokens":3000,"vendor":{"limits":[1e+06,9007199254740993,null]}}`
	const duplicate = `{"id":"good-model","object":"model","owned_by":"duplicate","appthrust.io/maxPromptTokens":9999}`
	const blocked = `{"id":"blocked-model","object":"model","owned_by":"appthrust"}`
	const wildcardOnly = `{"id":"good-unlisted","object":"model","owned_by":"appthrust"}`
	const foreignMissing = `{"id":"missing-model","object":"model","owned_by":"forbidden","appthrust.io/maxPromptTokens":9999}`
	const foreignOnly = `{"id":"provider-only","object":"model","owned_by":"forbidden"}`
	var upstreamCalls, forbiddenCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "" || r.Header.Get("Accept") != "application/json" {
			t.Errorf("model request=%s %s headers=%v", r.Method, r.URL, r.Header)
		}
		fmt.Fprintf(w, `{"object":"list","data":[%s,%s,%s,%s]}`, entry, duplicate, blocked, wildcardOnly)
	}))
	defer upstream.Close()
	forbidden := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		forbiddenCalls.Add(1)
		fmt.Fprintf(w, `{"object":"list","data":[%s,%s,%s]}`, duplicate, foreignMissing, foreignOnly)
	}))
	defer forbidden.Close()
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer workload-key" {
			t.Errorf("discovery authorization=%q", r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"links": []any{
			map[string]any{"definitionId": "edge-chat", "state": "Ready", "capabilities": []string{chatcompletions.Capability}, "internalUrl": upstream.URL},
		}})
	}))
	defer platform.Close()
	for _, tc := range []struct {
		mode           aipolicy.Mode
		want           []string
		forbiddenCalls int32
	}{
		{aipolicy.ModeUnrestricted, []string{entry, blocked, wildcardOnly, foreignMissing, foreignOnly}, 1},
		{aipolicy.ModeEnforced, []string{`{"created":0,"id":"missing-model","object":"model","owned_by":"edge-chat"}`, entry}, 0},
	} {
		t.Run(string(tc.mode), func(t *testing.T) {
			upstreamCalls.Store(0)
			forbiddenCalls.Store(0)
			document := aipolicy.Document{Mode: tc.mode, Providers: []aipolicy.Provider{{
				Name: "edge-chat", ModelRules: []aipolicy.ModelRule{
					{Patterns: []string{"good-*", "missing-model"}},
					{Patterns: []string{"good-model", "missing-model", "blocked-model", "good-model"}},
				},
			}}}
			resolver := link.NewResolver(platformclient.NewGatewayDiscovery(platform.URL, "pj-test", "workload-key"), "ai-gateway", nil, nil)
			g := &gateway{auth: testAuth{}, policy: document, providers: &registry{
				resolver: resolver, client: provider.NewClient(),
				static: []provider.Provider{provider.NewCapability("forbidden", forbidden.URL, provider.NewClient())},
			}}
			w := callGateway(g, http.MethodGet, "/v1/models", "")
			var list struct {
				Object string            `json:"object"`
				Data   []json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil || w.Code != http.StatusOK || list.Object != "list" || len(list.Data) != len(tc.want) {
				t.Fatalf("models=%d %s err=%v", w.Code, w.Body, err)
			}
			for i, want := range tc.want {
				if string(list.Data[i]) != want {
					t.Errorf("entry[%d]=%s want=%s", i, list.Data[i], want)
				}
			}
			if upstreamCalls.Load() != 1 || forbiddenCalls.Load() != tc.forbiddenCalls {
				t.Fatalf("model calls allowed=%d forbidden=%d", upstreamCalls.Load(), forbiddenCalls.Load())
			}
		})
	}
}

func TestCredentialRegistryOrderAndDefaults(t *testing.T) {
	t.Setenv("AI_GATEWAY_OPENAI_API_KEY", "openai-key")
	t.Setenv("AI_GATEWAY_ANTHROPIC_API_KEY", "anthropic-key")
	registry := registryFromEnv(nil)
	providers, err := registry.providers(context.Background())
	if err != nil || len(providers) != 2 || providers[0].Name() != "openai" || providers[0].DefaultModel() != "gpt-5-mini" || providers[1].Name() != "anthropic" || providers[1].DefaultModel() != "claude-sonnet-4-5" {
		t.Fatalf("registry=%v err=%v", providers, err)
	}
	t.Setenv("AI_GATEWAY_OPENAI_API_KEY", "")
	registry = registryFromEnv(nil)
	if len(registry.static) != 1 || registry.static[0].Name() != "anthropic" {
		t.Fatal("missing keys must not register unusable providers")
	}
}

func TestPolicyEnvironmentFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name, mode, document                string
		unsetMode, unsetDocument, wantError bool
		want                                aipolicy.Mode
	}{
		{"both unset", "", "", true, true, false, aipolicy.ModeUnrestricted},
		{"explicit empty mode", "", "", false, true, true, ""},
		{"missing mode", "", `{"apiVersion":"aipolicy.appthrust.io/v1","mode":"Unrestricted"}`, true, false, true, ""},
		{"missing document", "Unrestricted", "", false, true, true, ""},
		{"invalid document", "Unrestricted", "{", false, false, true, ""},
		{"mismatch", "Enforced", `{"apiVersion":"aipolicy.appthrust.io/v1","mode":"Unrestricted"}`, false, false, true, ""},
		{"valid", "Enforced", `{"apiVersion":"aipolicy.appthrust.io/v1","mode":"Enforced","providers":[]}`, false, false, false, aipolicy.ModeEnforced},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(aipolicy.EnvMode, tc.mode)
			t.Setenv(aipolicy.EnvDocument, tc.document)
			if tc.unsetMode {
				if err := os.Unsetenv(aipolicy.EnvMode); err != nil {
					t.Fatal(err)
				}
			}
			if tc.unsetDocument {
				if err := os.Unsetenv(aipolicy.EnvDocument); err != nil {
					t.Fatal(err)
				}
			}
			document, err := policyFromEnv()
			if (err != nil) != tc.wantError || !tc.wantError && document.Mode != tc.want {
				t.Fatalf("document=%+v err=%v", document, err)
			}
		})
	}
}

func TestRegistryActiveHealthClassificationAndCache(t *testing.T) {
	for _, tc := range []struct {
		name    string
		codes   []int
		state   string
		reason  string
		message string
	}{
		{"models OK", []int{200}, apphealth.StateServing, "", ""},
		{"unauthorized", []int{401}, apphealth.StateFailing, apphealth.ReasonUpstreamDenied, "Provider rejected the configured credential"},
		{"forbidden", []int{403}, apphealth.StateFailing, apphealth.ReasonUpstreamDenied, "Provider rejected the configured credential"},
		{"unavailable", []int{503}, apphealth.StateFailing, apphealth.ReasonUpstreamUnavailable, "AI provider is unavailable"},
		{"all denied", []int{401, 403}, apphealth.StateFailing, apphealth.ReasonUpstreamDenied, "Provider rejected the configured credential"},
		{"all unavailable", []int{500, 503}, apphealth.StateFailing, apphealth.ReasonUpstreamUnavailable, "AI provider is unavailable"},
		{"mixed failures", []int{503, 401}, apphealth.StateFailing, apphealth.ReasonUpstreamDenied, "Provider rejected the configured credential"},
		{"mixed failures reversed", []int{403, 500}, apphealth.StateFailing, apphealth.ReasonUpstreamDenied, "Provider rejected the configured credential"},
		{"one serving", []int{401, 200}, apphealth.StateServing, "", ""},
		{"one serving reversed", []int{200, 401}, apphealth.StateServing, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			r := &registry{}
			for i, code := range tc.codes {
				backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					calls.Add(1)
					if req.Method != http.MethodGet || req.URL.Path != "/v1/models" {
						t.Errorf("unexpected probe: %s %s", req.Method, req.URL.Path)
					}
					w.WriteHeader(code)
					fmt.Fprint(w, "private provider diagnostics that must not appear in health")
				}))
				t.Cleanup(backend.Close)
				r.static = append(r.static, provider.NewCapability(fmt.Sprintf("provider-%d", i), backend.URL, backend.Client()))
			}
			// Merely listing configured adapters must not mask a failed check.
			if _, err := r.providers(context.Background()); err != nil {
				t.Fatal(err)
			}
			want := health.Status{State: tc.state, Reason: tc.reason, Message: tc.message}
			for range 2 {
				if got := r.healthStatus(context.Background()); got != want {
					t.Fatalf("health=%+v want=%+v", got, want)
				}
			}
			if got := calls.Load(); got != int32(len(tc.codes)) {
				t.Fatalf("probe calls=%d want=%d", got, len(tc.codes))
			}
		})
	}
}

func TestRegistryActiveHealthConnectionRefused(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("closed backend received a request")
	}))
	backend.Close()
	client := provider.NewClient()
	defer client.CloseIdleConnections()
	r := &registry{static: []provider.Provider{provider.NewCapability("test", backend.URL, client)}}
	want := health.Status{State: apphealth.StateFailing, Reason: apphealth.ReasonUpstreamUnavailable, Message: "AI provider is unavailable"}
	if got := r.healthStatus(context.Background()); got != want {
		t.Fatalf("health=%+v want=%+v", got, want)
	}
}

func TestRegistryActiveHealthConcurrentReaders(t *testing.T) {
	var calls atomic.Int32
	entered := make(chan struct{}, 32)
	release := make(chan struct{})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		entered <- struct{}{}
		select {
		case <-release:
			w.WriteHeader(http.StatusOK)
		case <-req.Context().Done():
		}
	}))
	defer backend.Close()
	r := &registry{static: []provider.Provider{provider.NewCapability("test", backend.URL, backend.Client())}}
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan health.Status, 1)
	go func() { first <- r.healthStatus(ctx) }()
	<-entered
	cancel()
	<-first
	// A cancelled reader leaves the shared probe alive for other readers.
	const readers = 16
	results := make(chan health.Status, readers)
	var started sync.WaitGroup
	started.Add(readers)
	for range readers {
		go func() {
			started.Done()
			results <- r.healthStatus(context.Background())
		}()
	}
	started.Wait()
	close(release)
	for range readers {
		if got := <-results; got.State != apphealth.StateServing {
			t.Errorf("concurrent health=%+v", got)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("concurrent probe calls=%d", got)
	}
}

func TestRegistryActiveHealthExpiryAndTrafficOverride(t *testing.T) {
	var calls atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer backend.Close()
	r := &registry{static: []provider.Provider{provider.NewCapability("test", backend.URL, backend.Client())}}
	if got := r.healthStatus(context.Background()); got.Reason != apphealth.ReasonUpstreamDenied {
		t.Fatalf("initial health=%+v", got)
	}
	for _, status := range []health.Status{
		{State: apphealth.StateServing},
		{State: apphealth.StateFailing, Reason: apphealth.ReasonUpstreamUnavailable},
	} {
		r.observeHealth(status)
		if got := r.healthStatus(context.Background()); got != status || calls.Load() != 1 {
			t.Fatalf("traffic override=%+v calls=%d", got, calls.Load())
		}
	}
	r.healthMu.Lock()
	r.healthObserved = time.Now().Add(-providerHealthWindow - time.Second)
	r.healthMu.Unlock()
	if got := r.healthStatus(context.Background()); got.Reason != apphealth.ReasonUpstreamDenied || calls.Load() != 1 {
		t.Fatalf("cached health=%+v calls=%d", got, calls.Load())
	}
	r.healthMu.Lock()
	r.healthProbes["test"].observed = time.Now().Add(-providerHealthWindow - time.Second)
	r.healthMu.Unlock()
	if got := r.healthStatus(context.Background()); got.Reason != apphealth.ReasonUpstreamDenied || calls.Load() != 2 {
		t.Fatalf("refreshed health=%+v calls=%d", got, calls.Load())
	}
}

func TestRegistryTrafficWinsOverInflightProbe(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		close(entered)
		select {
		case <-release:
			w.WriteHeader(http.StatusUnauthorized)
		case <-req.Context().Done():
		}
	}))
	defer backend.Close()
	r := &registry{static: []provider.Provider{provider.NewCapability("test", backend.URL, backend.Client())}}
	result := make(chan health.Status, 1)
	go func() { result <- r.healthStatus(context.Background()) }()
	<-entered
	r.observeHealth(health.Status{State: apphealth.StateServing})
	close(release)
	if got := <-result; got.State != apphealth.StateServing {
		t.Fatalf("probe overwrote traffic: %+v", got)
	}
}

type healthResolver func(context.Context, string, string) ([]link.Binding, error)

func (f healthResolver) ByCapability(ctx context.Context, bearer, capability string) ([]link.Binding, error) {
	return f(ctx, bearer, capability)
}

func TestRegistryResolverHealthEvidence(t *testing.T) {
	calls := 0
	ready := false
	r := &registry{resolver: healthResolver(func(_ context.Context, bearer, capability string) ([]link.Binding, error) {
		calls++
		if bearer != "" || capability != chatcompletions.Capability {
			t.Error("discovery must not receive caller authority")
		}
		if ready {
			return []link.Binding{{Definition: "inference", State: link.StateReady, InternalURL: "http://inference"}}, nil
		}
		return nil, nil
	})}
	ctx := context.Background()
	if got := r.healthStatus(ctx); got.State != apphealth.StateUnknown || got.Reason != apphealth.ReasonNoEvidence {
		t.Fatalf("first observation=%+v", got)
	}
	if got := r.healthStatus(ctx); got.State != apphealth.StateFailing || got.Reason != apphealth.ReasonDependencyUnavailable {
		t.Fatalf("second observation=%+v", got)
	}
	if calls != 0 {
		t.Fatalf("health triggered unauthenticated discovery: calls=%d", calls)
	}
	if _, err := r.providers(ctx); err != nil {
		t.Fatal(err)
	}
	if got := r.healthStatus(ctx); got.State != apphealth.StateFailing || got.Message != "No AI provider is Ready" {
		t.Fatalf("empty discovery=%+v", got)
	}
	ready = true
	if _, err := r.providers(ctx); err != nil {
		t.Fatal(err)
	}
	if got := r.healthStatus(ctx); got.State != apphealth.StateServing {
		t.Fatalf("ready discovery=%+v", got)
	}
	r.healthMu.Lock()
	r.healthObserved = time.Now().Add(-providerHealthWindow - time.Second)
	r.healthMu.Unlock()
	if got := r.healthStatus(ctx); got.Reason == apphealth.ReasonNoEvidence || got.State != apphealth.StateFailing {
		t.Fatalf("expired discovery=%+v", got)
	}
}
