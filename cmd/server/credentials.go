package main

import (
	"net/http"
	"os"

	"github.com/appthrust/ai-gateway/cmd/server/internal/provider"
)

type providerRegistration struct {
	name string
	kind string
	new  func(string, *http.Client) provider.Provider
}

var providerRegistrations = []providerRegistration{
	{name: "openai", kind: "OpenAI", new: provider.NewOpenAI},
	{name: "anthropic", kind: "Anthropic", new: provider.NewAnthropic},
}

type credentialSlot struct {
	Name        string `yaml:"name"`
	DisplayName string `yaml:"displayName"`
	Provider    string `yaml:"provider"`
	Env         string `yaml:"env"`
}

// Catalog credentialSlots are the published projection of this registration table.
// TestCatalogCredentialRegistrations prevents the consumer and catalog drifting.
// The platform treats ApplicationStackDefinition components as immutable, so the
// slot list cannot change in an app release; see docs/provider-keys.md.
var credentialSlots = []credentialSlot{
	{Name: "openai", DisplayName: "OpenAI API key", Provider: "OpenAI", Env: "AI_GATEWAY_OPENAI_API_KEY"},
	{Name: "anthropic", DisplayName: "Anthropic API key", Provider: "Anthropic", Env: "AI_GATEWAY_ANTHROPIC_API_KEY"},
}

type keyedProvider struct {
	provider.Provider
	slot string
}

func (p *keyedProvider) credentialSlot() string { return p.slot }

type keyedEmbedder struct {
	*keyedProvider
	provider.Embedder
}

func registerKey(p provider.Provider, slot string) provider.Provider {
	key := &keyedProvider{Provider: p, slot: slot}
	if embedder, ok := p.(provider.Embedder); ok {
		return &keyedEmbedder{keyedProvider: key, Embedder: embedder}
	}
	return key
}

func observationID(p provider.Provider) string {
	if key, ok := p.(interface{ credentialSlot() string }); ok {
		return key.credentialSlot()
	}
	return p.Name()
}

func registryFromEnv(resolver capabilityResolver) *registry {
	client := provider.NewClient()
	r := &registry{resolver: resolver, client: client}
	for _, registration := range providerRegistrations {
		for _, slot := range credentialSlots {
			if slot.Provider != registration.kind {
				continue
			}
			if value := os.Getenv(slot.Env); value != "" {
				r.static = append(r.static, registerKey(registration.new(value, client), slot.Name))
			}
		}
	}
	return r
}

func (r *registry) selectedStatic() []provider.Provider {
	r.healthMu.Lock()
	defer r.healthMu.Unlock()
	return selectStatic(r.static, r.observationsLocked())
}

func selectStatic(keys []provider.Provider, observations map[string]providerObservation) []provider.Provider {
	selected := make([]provider.Provider, 0, len(keys))
	for _, key := range keys {
		index := -1
		for i, p := range selected {
			if p.Name() == key.Name() {
				index = i
				break
			}
		}
		if index < 0 {
			selected = append(selected, key)
		} else if keyRank(observations[observationID(key)]) < keyRank(observations[observationID(selected[index])]) {
			selected[index] = key
		}
	}
	return selected
}

func keyRank(o providerObservation) int {
	if o.observed.IsZero() {
		return 1
	}
	if o.status.State == "Serving" {
		return 0
	}
	return 2
}
