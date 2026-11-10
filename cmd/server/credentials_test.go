package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/appthrust/ai-gateway/cmd/server/internal/provider"
	"github.com/appthrust/appserver-go/aipolicy"
	"github.com/appthrust/appserver-go/apphealth"
	"github.com/appthrust/appserver-go/appserver/health"
	"github.com/appthrust/appserver-go/appserver/link"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestCatalogCredentialRegistrations(t *testing.T) {
	data, err := os.ReadFile("../../catalog/entry.yaml")
	require.NoError(t, err)
	var catalog struct {
		Spec struct {
			Components []struct {
				DefaultValues struct {
					CredentialSlots []credentialSlot `yaml:"credentialSlots"`
				} `yaml:"defaultValues"`
			} `yaml:"components"`
		} `yaml:"spec"`
	}
	require.NoError(t, yaml.Unmarshal(data, &catalog))
	require.Equal(t, credentialSlots, catalog.Spec.Components[0].DefaultValues.CredentialSlots)
	names, envs := map[string]bool{}, map[string]bool{}
	for _, slot := range credentialSlots {
		require.False(t, names[slot.Name])
		require.False(t, envs[slot.Env])
		names[slot.Name], envs[slot.Env] = true, true
		found := 0
		for _, registration := range providerRegistrations {
			if registration.kind == slot.Provider {
				found++
			}
		}
		require.Equal(t, 1, found)
	}
}

func TestCapabilityHealthDecisionTable(t *testing.T) {
	denied := providerHealthStatus(http.StatusUnauthorized, nil)
	for _, test := range []struct {
		name      string
		usable    bool
		unchecked bool
		failure   health.Status
		state     string
	}{
		{name: "usable", usable: true, state: "Serving"},
		{name: "usable with rejected alternative", usable: true, failure: denied, state: "Serving"},
		{name: "usable with unreachable alternative", usable: true, failure: providerHealthStatus(503, nil), state: "Serving"},
		{name: "usable with unchecked alternative", usable: true, unchecked: true, state: "Serving"},
		{name: "all rejected", failure: denied, state: "Failing"},
		{name: "known empty", state: "Failing"},
		{name: "no evidence", unchecked: true, state: "Unknown"},
		{name: "rejected with unchecked alternative", unchecked: true, failure: denied, state: "Unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.state, decideCapabilityHealth(test.usable, test.unchecked, test.failure).State)
		})
	}
}

func TestRejectedKeyAndReadyLink(t *testing.T) {
	p := &inspectFixture{fakeProvider: fakeProvider{name: "openai"}, probeCode: http.StatusUnauthorized}
	r := &registry{static: []provider.Provider{registerKey(p, "openai")}, resolver: healthResolver(func(context.Context, string, string) ([]link.Binding, error) {
		return []link.Binding{{Definition: "llm", State: link.StateReady, InternalURL: "http://llm"}}, nil
	})}
	require.Equal(t, apphealth.StateServing, r.healthStatus(context.Background()).State)
	snapshot := r.snapshot(context.Background())
	document := aipolicy.Document{Mode: aipolicy.ModeUnrestricted}
	views := decideProviders(document, snapshot, nil, time.Now())
	require.Equal(t, "openai", views[1].Name)
	require.Equal(t, "Denied", views[1].State)
	require.Equal(t, "rejected", views[1].Keys[0].State)
	steps := decideNextSteps(document, decideConfigured(snapshot), decideHealth(document, snapshot, time.Now()), views)
	require.Contains(t, steps, nextStep{Kind: "FixProvider", Provider: "openai", Slot: "openai"})
	// A withdrawn link must not remain usable in a second routing cache.
	r.resolver = healthResolver(func(context.Context, string, string) ([]link.Binding, error) { return nil, nil })
	require.Equal(t, apphealth.StateFailing, r.healthStatus(context.Background()).State)
}

func TestMultipleKeysSelectionAndIndependentChecks(t *testing.T) {
	primary := &inspectFixture{fakeProvider: fakeProvider{name: "openai"}, probeCode: 401}
	secondary := &inspectFixture{fakeProvider: fakeProvider{name: "openai"}, probeCode: 200}
	r := &registry{static: []provider.Provider{registerKey(primary, "openai"), registerKey(secondary, "openai-secondary")}}
	g := &gateway{auth: testAuth{}, providers: r, policy: aipolicy.Document{Mode: aipolicy.ModeUnrestricted}}
	first := callGateway(g, http.MethodPost, "/api/ai-gateway/v1/providers/openai/keys/openai/check", "")
	require.Equal(t, http.StatusOK, first.Code)
	second := callGateway(g, http.MethodPost, "/api/ai-gateway/v1/providers/openai/keys/openai-secondary/check", "")
	require.Equal(t, http.StatusOK, second.Code)
	var view ProviderView
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &view))
	require.Equal(t, "Serving", view.State)
	require.Equal(t, "rejected", view.Keys[0].State)
	require.Equal(t, "usable", view.Keys[1].State)
	require.NotNil(t, view.Keys[1].ObservedAt)
	require.Equal(t, "openai-secondary", observationID(r.configured("openai")))
	require.Equal(t, apphealth.StateServing, r.healthStatus(context.Background()).State)
	require.Equal(t, http.StatusTooManyRequests, callGateway(g, http.MethodPost, "/api/ai-gateway/v1/providers/openai/keys/openai-secondary/check", "").Code)
	require.Equal(t, http.StatusNotFound, callGateway(g, http.MethodPost, "/api/ai-gateway/v1/providers/openai/keys/not-declared/check", "").Code)
	// The provider that fails later cannot overwrite another key's observation.
	r.observeProvider(r.static[1], providerHealthStatus(503, nil))
	require.Equal(t, apphealth.StateFailing, r.healthStatus(context.Background()).State)
}

func TestCredentialRegistrationConsumesEveryBoundSlot(t *testing.T) {
	t.Setenv("AI_GATEWAY_OPENAI_API_KEY", "test-openai")
	t.Setenv("AI_GATEWAY_ANTHROPIC_API_KEY", "test-anthropic")
	r := registryFromEnv(nil)
	require.Len(t, r.static, 2)
	require.Equal(t, "openai", observationID(r.static[0]))
	require.Equal(t, "anthropic", observationID(r.static[1]))
	_, supportsEmbeddings := r.static[0].(provider.Embedder)
	require.True(t, supportsEmbeddings)
}

func TestPolicyDenialDoesNotChangeUnknownOrFailing(t *testing.T) {
	now := time.Now()
	p := &fakeProvider{name: "openai"}
	for _, test := range []struct {
		name   string
		code   int
		denied time.Time
		state  string
	}{
		{name: "recent denial with usable provider", code: 200, denied: now, state: "Degraded"},
		{name: "expired denial", code: 200, denied: now.Add(-providerHealthWindow - time.Second), state: "Serving"},
		{name: "cleared denial", code: 200, state: "Serving"},
		{name: "failed provider remains failing", code: 401, denied: now, state: "Failing"},
		{name: "unchecked provider remains unknown", denied: now, state: "Unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := providerSnapshot{static: []provider.Provider{p}, policyDeniedAt: test.denied, observations: map[string]providerObservation{}}
			if test.code != 0 {
				snapshot.observations["openai"] = providerObservation{status: providerHealthStatus(test.code, nil), observed: now}
			}
			require.Equal(t, test.state, decideHealth(aipolicy.Document{Mode: aipolicy.ModeUnrestricted}, snapshot, now).State)
		})
	}
}
