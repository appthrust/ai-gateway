package main

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/appthrust/ai-gateway/cmd/server/internal/provider"
	"github.com/appthrust/appserver-go/aipolicy"
	"github.com/appthrust/appserver-go/apphealth"
	"github.com/appthrust/appserver-go/appserver/contract/chatcompletions"
	"github.com/appthrust/appserver-go/appserver/health"
	"github.com/appthrust/appserver-go/appserver/link"
)

type providerObservation struct {
	status   health.Status
	observed time.Time
	models   []string
}

type providerSnapshot struct {
	bindings        []link.Binding
	available       []provider.Provider
	static          []provider.Provider
	observations    map[string]providerObservation
	discoveryFailed bool
	healthResult    health.Status
	healthObserved  time.Time
	healthStarted   bool
	hasResolver     bool
}

// UI discovery deliberately does not call providers(): observing the operational
// surface is not authenticated inference traffic and must not overwrite health.
func (r *registry) snapshot(ctx context.Context) providerSnapshot {
	s := providerSnapshot{static: r.static, hasResolver: r.resolver != nil}
	if r.resolver != nil {
		var err error
		s.bindings, err = r.resolver.ByCapability(ctx, "", chatcompletions.Capability)
		s.discoveryFailed = err != nil
	}
	if !s.discoveryFailed {
		s.available = r.linkedProviders(s.bindings)
	}
	s.available = append(s.available, r.static...)
	r.healthMu.Lock()
	defer r.healthMu.Unlock()
	s.healthResult, s.healthObserved, s.healthStarted = r.healthResult, r.healthObserved, r.healthStarted
	s.observations = make(map[string]providerObservation, len(r.healthProbes))
	for name, p := range r.healthProbes {
		s.observations[name] = providerObservation{status: p.status, observed: p.observed, models: p.models}
	}
	return s
}

type providerSource struct {
	Link           string `json:"link,omitempty"`
	LinkState      string `json:"linkState,omitempty"`
	CredentialSlot string `json:"credentialSlot,omitempty"`
	Bound          *bool  `json:"bound,omitempty"`
}

type modelDenial struct {
	Provider string   `json:"provider"`
	Patterns []string `json:"patterns"`
}

type modelView struct {
	ID       string       `json:"id"`
	Allowed  bool         `json:"allowed"`
	Default  bool         `json:"default"`
	DeniedBy *modelDenial `json:"deniedBy"`
}

type ProviderView struct {
	Name       string         `json:"name"`
	Kind       string         `json:"kind"`
	Source     providerSource `json:"source"`
	State      string         `json:"state"`
	Reason     string         `json:"reason"`
	Message    string         `json:"message"`
	ObservedAt *time.Time     `json:"observedAt"`
	Embeddings bool           `json:"embeddings"`
	Models     []modelView    `json:"models"`
}

func providerInPolicy(document aipolicy.Document, name string) bool {
	for _, p := range document.Providers {
		if p.Name == name {
			return true
		}
	}
	return false
}

func decideProviderState(document aipolicy.Document, name string, available, credential bool, observation providerObservation, now time.Time) (string, string, string) {
	if document.Mode == aipolicy.ModeEnforced && !providerInPolicy(document, name) {
		return "PolicyExcluded", "ai_model_denied", "Provider is not included in the effective policy"
	}
	if !available {
		if credential {
			return "NotConfigured", "", "Provider credential is not bound"
		}
		return "NotInstalled", "", "No Ready provider link is available"
	}
	if !credential {
		return "Serving", "", ""
	}
	if observation.observed.IsZero() || now.Sub(observation.observed) > providerHealthWindow {
		return "Unknown", apphealth.ReasonNoEvidence, "No recent provider check"
	}
	if observation.status.State == apphealth.StateServing {
		return "Serving", "", ""
	}
	if observation.status.Reason == apphealth.ReasonUpstreamDenied {
		return "Denied", apphealth.ReasonUpstreamDenied, "Provider rejected the configured credential"
	}
	return "Unreachable", apphealth.ReasonUpstreamUnavailable, "AI provider is unavailable"
}

func decideModel(document aipolicy.Document, name, model string, route *defaultRoute) modelView {
	v := modelView{ID: model, Allowed: document.Allows(name, model).Allowed, Default: route != nil && route.Provider == name && route.Model == model}
	if v.Allowed {
		return v
	}
	v.DeniedBy = &modelDenial{Provider: name, Patterns: []string{}}
	for _, p := range document.Providers {
		if p.Name != name {
			continue
		}
		for _, rule := range p.ModelRules {
			one := aipolicy.Document{Mode: aipolicy.ModeEnforced, Providers: []aipolicy.Provider{{Name: name, ModelRules: []aipolicy.ModelRule{rule}}}}
			if !one.Allows(name, model).Allowed {
				v.DeniedBy.Patterns = rule.Patterns
				return v
			}
		}
	}
	return v
}

func decideProviders(document aipolicy.Document, snapshot providerSnapshot, route *defaultRoute, now time.Time) []ProviderView {
	items := make([]ProviderView, 0, len(snapshot.available)+2)
	seen := make(map[string]bool)
	add := func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		var p provider.Provider
		for _, candidate := range snapshot.available {
			if candidate.Name() == name {
				p = candidate
				break
			}
		}
		credential := name == "openai" || name == "anthropic"
		v := ProviderView{Name: name, Kind: "InProjectApp", Models: []modelView{}}
		if credential {
			bound := p != nil
			v.Source = providerSource{CredentialSlot: name, Bound: &bound}
			v.Kind = "OpenAI"
			if name == "anthropic" {
				v.Kind = "Anthropic"
			}
		} else {
			v.Source = providerSource{Link: name, LinkState: string(link.StateAbsent)}
			for _, b := range snapshot.bindings {
				if b.Definition == name {
					v.Source.LinkState = string(b.State)
					break
				}
			}
		}
		observation := snapshot.observations[name]
		v.State, v.Reason, v.Message = decideProviderState(document, name, p != nil, credential, observation, now)
		if !observation.observed.IsZero() {
			at := observation.observed.UTC()
			v.ObservedAt = &at
		}
		_, v.Embeddings = p.(provider.Embedder)
		if p != nil {
			for _, model := range observation.models {
				v.Models = append(v.Models, decideModel(document, name, model, route))
			}
		}
		items = append(items, v)
	}
	for _, p := range snapshot.available {
		if p.Name() != "openai" && p.Name() != "anthropic" {
			add(p.Name())
		}
	}
	add("openai")
	add("anthropic")
	for _, p := range document.Providers {
		add(p.Name)
	}
	return items
}

// A forced check shares an in-flight probe; completed probes have a 30-second
// admission window. Ordinary reads retain the existing five-minute cache.
func (r *registry) inspectProvider(ctx context.Context, p provider.Provider, force bool, static bool) int {
	r.healthMu.Lock()
	if r.healthProbes == nil {
		r.healthProbes = make(map[string]*providerHealth)
	}
	probe := r.healthProbes[p.Name()]
	if force && probe != nil && !probe.observed.IsZero() && time.Since(probe.observed) < 30*time.Second {
		retry := int((30*time.Second - time.Since(probe.observed) + time.Second - 1) / time.Second)
		r.healthMu.Unlock()
		return retry
	}
	if probe == nil || static && !probe.observed.IsZero() && (force || time.Since(probe.observed) > providerHealthWindow) {
		probe = &providerHealth{}
		r.healthProbes[p.Name()] = probe
	}
	if static && probe.done == nil {
		probe.done = make(chan struct{})
		go r.probeProvider(p, probe)
	}
	if probe.modelsDone == nil || !probe.modelsObserved.IsZero() && time.Since(probe.modelsObserved) > providerHealthWindow {
		probe.modelsDone = make(chan struct{})
		go r.listProviderModels(p, probe, probe.modelsDone)
	}
	done, modelsDone := probe.done, probe.modelsDone
	r.healthMu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return 0
		}
	}
	select {
	case <-modelsDone:
	case <-ctx.Done():
	}
	return 0
}

func (r *registry) listProviderModels(p provider.Provider, probe *providerHealth, done chan struct{}) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	entries, err := p.Models(ctx)
	var models []string
	if err == nil {
		models = make([]string, 0, len(entries))
		for _, entry := range entries {
			if entry.ID != "" {
				models = append(models, entry.ID)
			}
		}
	}
	r.healthMu.Lock()
	probe.models, probe.modelsObserved = models, time.Now()
	close(done)
	r.healthMu.Unlock()
}

func (g *gateway) providerViews(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	snapshot := g.providers.snapshot(ctx)
	var pending []chan struct{}
	for _, p := range snapshot.available {
		done := make(chan struct{})
		pending = append(pending, done)
		go func() {
			defer close(done)
			g.providers.inspectProvider(ctx, p, false, g.providers.configured(p.Name()) != nil)
		}()
	}
	for _, done := range pending {
		<-done
	}
	// Retain this request's binding snapshot; refreshing only cached observations
	// avoids inventing a second resolver lifetime or probing withdrawn links.
	g.providers.refreshObservations(&snapshot)
	writeJSON(w, http.StatusOK, map[string]any{"items": decideProviders(g.policy, snapshot, decideDefaultRoute(g.policy, snapshot, g.defaultProvider, g.defaultModel), time.Now())})
}

func (r *registry) refreshObservations(snapshot *providerSnapshot) {
	r.healthMu.Lock()
	defer r.healthMu.Unlock()
	for name, p := range r.healthProbes {
		snapshot.observations[name] = providerObservation{status: p.status, observed: p.observed, models: p.models}
	}
}

func (g *gateway) checkProvider(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	snapshot := g.providers.snapshot(ctx)
	name := r.PathValue("name")
	for _, p := range snapshot.available {
		if p.Name() != name {
			continue
		}
		if g.providers.configured(name) != nil {
			if retry := g.providers.inspectProvider(ctx, p, true, true); retry > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(retry))
				writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": map[string]any{"code": "ai_check_rate_limited", "message": "Provider was checked recently", "retryAfterSeconds": retry}})
				return
			}
		}
		break
	}
	g.providers.refreshObservations(&snapshot)
	for _, item := range decideProviders(g.policy, snapshot, decideDefaultRoute(g.policy, snapshot, g.defaultProvider, g.defaultModel), time.Now()) {
		if item.Name == name {
			writeJSON(w, http.StatusOK, item)
			return
		}
	}
	writeError(w, http.StatusNotFound, "ai_provider_unknown", "The AI provider is unknown")
}
