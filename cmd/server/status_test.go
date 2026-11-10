package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/appthrust/ai-gateway/cmd/server/internal/provider"
	"github.com/appthrust/appserver-go/aipolicy"
	"github.com/appthrust/appserver-go/apphealth"
	"github.com/appthrust/appserver-go/appserver/link"
	"github.com/stretchr/testify/require"
)

func TestStatusDecisionTable(t *testing.T) {
	now := time.Now()
	openai := &fakeProvider{name: "openai", defaultModel: "adapter-default"}
	capability := &fakeProvider{name: "llm"}
	policy := aipolicy.Document{Mode: aipolicy.ModeEnforced, Providers: []aipolicy.Provider{{Name: "llm", ModelRules: []aipolicy.ModelRule{{Patterns: []string{"gemma"}}}}}}
	for _, tc := range []struct {
		name                          string
		document                      aipolicy.Document
		snapshot                      providerSnapshot
		bound                         bool
		defaultProvider, defaultModel string
		configured                    bool
		state                         string
		route                         *defaultRoute
		steps                         []nextStep
	}{
		{name: "empty unrestricted", document: aipolicy.Document{Mode: aipolicy.ModeUnrestricted}, state: "Failing", steps: []nextStep{{Kind: "BindCredential", Slot: "openai"}, {Kind: "BindCredential", Slot: "anthropic"}, {Kind: "InstallApp", DefinitionID: "inference"}, {Kind: "BindPolicy"}}},
		{name: "credential serving", document: aipolicy.Document{Mode: aipolicy.ModeUnrestricted}, snapshot: providerSnapshot{available: []provider.Provider{openai}, static: []provider.Provider{openai}, observations: map[string]providerObservation{"openai": {observed: now, status: providerHealthStatus(200, nil)}}}, bound: true, configured: true, state: "Serving", route: &defaultRoute{Provider: "openai", Model: "adapter-default", Source: "Credential"}, steps: []nextStep{}},
		{name: "credential override", document: aipolicy.Document{Mode: aipolicy.ModeUnrestricted}, snapshot: providerSnapshot{available: []provider.Provider{openai}, static: []provider.Provider{openai}}, bound: true, configured: true, defaultProvider: "openai", defaultModel: "override", state: "Unknown", route: &defaultRoute{Provider: "openai", Model: "override", Source: "EnvOverride"}, steps: []nextStep{{Kind: "BindPolicy"}}},
		{name: "policy capability", document: policy, snapshot: providerSnapshot{available: []provider.Provider{capability}, bindings: []link.Binding{{Definition: "llm", State: link.StateReady, InternalURL: "http://llm"}}}, configured: true, state: "Serving", route: &defaultRoute{Provider: "llm", Model: "gemma", Source: "Policy"}, steps: []nextStep{}},
		{name: "capability default is omitted upstream", document: aipolicy.Document{Mode: aipolicy.ModeUnrestricted}, snapshot: providerSnapshot{available: []provider.Provider{capability}, bindings: []link.Binding{{Definition: "llm", State: link.StateReady, InternalURL: "http://llm"}}}, configured: true, state: "Serving", route: &defaultRoute{Provider: "llm", Source: "Capability"}, steps: []nextStep{}},
		{name: "pending with credentials", document: aipolicy.Document{Mode: aipolicy.ModePending}, snapshot: providerSnapshot{available: []provider.Provider{openai}, static: []provider.Provider{openai}}, bound: true, configured: true, state: "Failing", steps: []nextStep{{Kind: "ContactAdmin", Reason: "ai_policy_pending"}}},
		{name: "full logging", document: aipolicy.Document{Mode: aipolicy.ModeEnforced, Logging: &aipolicy.Logging{Conversation: "Full"}}, bound: true, configured: true, state: "Failing", steps: []nextStep{{Kind: "ContactAdmin", Reason: "ai_policy_logging_unavailable"}}},
		{name: "missing policy provider", document: policy, state: "Failing", steps: []nextStep{{Kind: "BindCredential", Slot: "openai"}, {Kind: "BindCredential", Slot: "anthropic"}, {Kind: "InstallApp", DefinitionID: "llm"}}},
		{name: "denied credential", document: aipolicy.Document{Mode: aipolicy.ModeUnrestricted}, snapshot: providerSnapshot{available: []provider.Provider{openai}, static: []provider.Provider{openai}, observations: map[string]providerObservation{"openai": {observed: now, status: providerHealthStatus(401, nil)}}}, bound: true, configured: true, state: "Failing", route: &defaultRoute{Provider: "openai", Model: "adapter-default", Source: "Credential"}, steps: []nextStep{{Kind: "FixProvider", Provider: "openai"}, {Kind: "BindPolicy"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.bound && len(tc.snapshot.static) == 0 {
				tc.snapshot.static = []provider.Provider{openai}
			}
			configured := decideConfigured(tc.snapshot)
			if !tc.configured {
				steps := append([]nextStep{}, tc.steps[:2]...)
				steps = append(steps, nextStep{Kind: "BindCredential", Slot: "openai-secondary"}, nextStep{Kind: "BindCredential", Slot: "anthropic-secondary"})
				tc.steps = append(steps, tc.steps[2:]...)
			}
			for i := range tc.steps {
				if tc.steps[i].Kind == "FixProvider" {
					tc.steps[i].Slot = tc.steps[i].Provider
				}
			}
			health := decideHealth(tc.document, tc.snapshot, now)
			route := decideDefaultRoute(tc.document, tc.snapshot, tc.defaultProvider, tc.defaultModel)
			require.Equal(t, tc.configured, configured.Satisfied)
			require.Equal(t, tc.state, health.State)
			require.Equal(t, tc.route, route)
			require.Equal(t, tc.steps, decideNextSteps(tc.document, configured, health, decideProviders(tc.document, tc.snapshot, route, now)))
			// The same request resolution functions used by complete must agree.
			if route != nil {
				record := aipolicy.InvocationRecord{}
				if tc.document.Mode == aipolicy.ModeEnforced {
					decideEnforced(tc.document, &record)
				}
				require.NotNil(t, decideProvider(tc.document.Mode, tc.defaultProvider, tc.defaultModel, tc.snapshot.available, &record))
				require.Equal(t, route.Provider, record.Provider)
				require.Equal(t, route.Model, record.Model)
			}
		})
	}
}

func TestProviderStateDecisions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		code    int
		failure error
		state   string
	}{
		{"ok", 200, nil, "Serving"}, {"denied", 401, nil, "Denied"}, {"forbidden", 403, nil, "Denied"}, {"bad gateway", 502, nil, "Unreachable"}, {"connect", 0, errors.New("private URL"), "Unreachable"}, {"none", 0, nil, "Unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := providerObservation{}
			if tc.code != 0 || tc.failure != nil {
				o = providerObservation{observed: time.Now(), status: providerHealthStatus(tc.code, tc.failure)}
			}
			state, _, message := decideProviderState(aipolicy.Document{Mode: aipolicy.ModeUnrestricted}, "openai", true, true, o, time.Now())
			require.Equal(t, tc.state, state)
			require.NotContains(t, message, "private")
		})
	}
	now := time.Now()
	snapshot := providerSnapshot{available: []provider.Provider{&fakeProvider{name: "llm"}}, bindings: []link.Binding{{Definition: "llm", State: link.StateReady}}}
	document := aipolicy.Document{Mode: aipolicy.ModeEnforced, Providers: []aipolicy.Provider{{Name: "llm"}, {Name: "openai"}, {Name: "vision"}}}
	items := decideProviders(document, snapshot, nil, now)
	require.Equal(t, []string{"llm", "openai", "anthropic", "vision"}, []string{items[0].Name, items[1].Name, items[2].Name, items[3].Name})
	require.Equal(t, []string{"Serving", "NotConfigured", "PolicyExcluded", "NotInstalled"}, []string{items[0].State, items[1].State, items[2].State, items[3].State})
	document.Providers[0].ModelRules = []aipolicy.ModelRule{{Patterns: []string{"g*"}}, {Patterns: []string{"good"}}}
	model := decideModel(document, "llm", "gone", nil)
	require.False(t, model.Allowed)
	require.Equal(t, []string{"good"}, model.DeniedBy.Patterns)
	require.True(t, decideModel(document, "llm", "good", &defaultRoute{Provider: "llm", Model: "good"}).Default)
}

type inspectFixture struct {
	fakeProvider
	probeCalls, modelCalls atomic.Int32
	probeCode              int
	block                  <-chan struct{}
}

func (p *inspectFixture) Probe(ctx context.Context) (int, error) {
	p.probeCalls.Add(1)
	if p.block != nil {
		select {
		case <-p.block:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	return p.probeCode, nil
}

func (p *inspectFixture) Models(ctx context.Context) ([]provider.ModelEntry, error) {
	p.modelCalls.Add(1)
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 30*time.Second {
		return nil, errors.New("missing model deadline")
	}
	return p.models, nil
}

func TestProviderCheckSingleFlightCacheAndRateLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		p := &inspectFixture{fakeProvider: fakeProvider{name: "openai", models: []provider.ModelEntry{provider.NewModelEntry("good", "openai")}}, probeCode: 401, block: release}
		r := &registry{static: []provider.Provider{p}}
		results := make(chan int, 8)
		for range 8 {
			go func() { results <- r.inspectProvider(context.Background(), p, true, true) }()
		}
		synctest.Wait()
		require.EqualValues(t, 1, p.probeCalls.Load())
		require.EqualValues(t, 1, p.modelCalls.Load())
		close(release)
		for range 8 {
			require.Zero(t, <-results)
		}
		require.Equal(t, 30, r.inspectProvider(context.Background(), p, true, true))
		require.Zero(t, r.inspectProvider(context.Background(), p, false, true))
		require.EqualValues(t, 1, p.probeCalls.Load())
		time.Sleep(30 * time.Second)
		require.Zero(t, r.inspectProvider(context.Background(), p, true, true))
		require.EqualValues(t, 2, p.probeCalls.Load())
		require.EqualValues(t, 2, p.modelCalls.Load())
		g := &gateway{auth: testAuth{}, policy: aipolicy.Document{Mode: aipolicy.ModeUnrestricted}, providers: r}
		w := callGateway(g, http.MethodPost, "/api/ai-gateway/v1/providers/openai/check", "")
		require.Equal(t, http.StatusTooManyRequests, w.Code)
		require.JSONEq(t, `{"error":{"code":"ai_check_rate_limited","message":"Provider was checked recently","retryAfterSeconds":30}}`, w.Body.String())
		require.Equal(t, "30", w.Header().Get("Retry-After"))
		w = callGateway(g, http.MethodPost, "/api/ai-gateway/v1/providers/missing/check", "")
		require.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestStatusProvidersNeverSerializeUpstreamSecrets(t *testing.T) {
	const secret = "credential-never-public"
	const bodySecret = "upstream-body-never-public"
	t.Setenv("AI_GATEWAY_OPENAI_API_KEY", secret)
	t.Setenv("AI_GATEWAY_PUBLIC_URL", "https://gateway.example")
	t.Setenv("AI_GATEWAY_RELEASE_VERSION", "0.3.0")
	t.Setenv("AI_GATEWAY_ARTIFACT_DIGEST", "sha256:release")
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"good","credential":"` + secret + `","private":"` + bodySecret + `"}]}`))
	}))
	defer backend.Close()
	resolver := healthResolver(func(context.Context, string, string) ([]link.Binding, error) {
		return []link.Binding{{Definition: "llm", State: link.StateReady, InternalURL: backend.URL}}, nil
	})
	g := &gateway{auth: testAuth{}, providers: &registry{resolver: resolver, client: backend.Client()}, policy: aipolicy.Document{Mode: aipolicy.ModeEnforced, Providers: []aipolicy.Provider{{Name: "llm", Endpoint: backend.URL, Credentials: secret, ModelRules: []aipolicy.ModelRule{{Patterns: []string{"good"}}}}}}}
	for _, path := range []string{"/api/ai-gateway/v1/status", "/api/ai-gateway/v1/providers"} {
		w := callGateway(g, http.MethodGet, path, "")
		require.Equal(t, http.StatusOK, w.Code)
		for _, forbidden := range []string{secret, bodySecret, backend.URL, "credentials", "endpoint" + `\"`} {
			require.NotContains(t, w.Body.String(), forbidden)
		}
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		require.True(t, json.Valid(w.Body.Bytes()))
		if strings.HasSuffix(path, "status") {
			var status GatewayStatus
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &status))
			require.True(t, status.Configured.Satisfied)
			require.Equal(t, "https://gateway.example/v1", status.Endpoints.Public)
			require.Equal(t, "0.3.0", status.Release.Version)
		} else {
			require.Contains(t, w.Body.String(), `"id":"good","allowed":true,"default":true`)
		}
	}
	require.Zero(t, g.invocationsRing.count)
}

func TestInternalEndpointRequiresOwnServiceEvidence(t *testing.T) {
	for _, name := range []string{"AI_GATEWAY_URL", "K_SERVICE", "POD_NAMESPACE", "NAMESPACE"} {
		t.Setenv(name, "")
	}
	require.Nil(t, internalEndpoint())
	t.Setenv("K_SERVICE", "ai-gateway-web")
	require.Nil(t, internalEndpoint())
	t.Setenv("POD_NAMESPACE", "pj-test")
	require.Equal(t, "http://ai-gateway-web.pj-test.svc/v1", *internalEndpoint())
	t.Setenv("AI_GATEWAY_URL", "http://secret@internal")
	require.Nil(t, internalEndpoint())
	t.Setenv("AI_GATEWAY_URL", "http://own-service")
	require.Equal(t, "http://own-service/v1", *internalEndpoint())
}

func TestStatusUsesSameObservedHealthAsProbe(t *testing.T) {
	p := &fakeProvider{name: "openai"}
	r := &registry{static: []provider.Provider{p}}
	require.Equal(t, apphealth.StateServing, r.healthStatus(context.Background()).State)
	s := r.snapshot(context.Background())
	require.Equal(t, r.healthStatus(context.Background()).State, decideHealth(aipolicy.Document{Mode: aipolicy.ModeUnrestricted}, s, time.Now()).State)
}
