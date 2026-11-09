package main

import (
	"context"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/appthrust/ai-gateway/cmd/server/internal/provider"
	"github.com/appthrust/appserver-go/apphealth"
	"github.com/appthrust/appserver-go/appserver/contract/chatcompletions"
	"github.com/appthrust/appserver-go/appserver/health"
	"github.com/appthrust/appserver-go/appserver/link"
)

type capabilityResolver interface {
	ByCapability(context.Context, string, string) ([]link.Binding, error)
}

// One registry/resolver is constructed per Project. link.Resolver caches the
// Project's read-model bindings for 60 seconds; credentials are never cached in
// bindings. No second cache can extend the lifetime of a withdrawn binding.
type registry struct {
	static         []provider.Provider
	resolver       capabilityResolver
	client         *http.Client
	healthMu       sync.Mutex
	healthObserved time.Time
	healthResult   health.Status
	healthStarted  bool
	healthProbes   map[string]*providerHealth
}

const providerHealthWindow = 5 * time.Minute

type providerHealth struct {
	done           chan struct{}
	observed       time.Time
	status         health.Status
	modelsDone     chan struct{}
	modelsObserved time.Time
	models         []string
}

func registryFromEnv(resolver capabilityResolver) *registry {
	client := provider.NewClient()
	r := &registry{resolver: resolver, client: client}
	if key := os.Getenv("AI_GATEWAY_OPENAI_API_KEY"); key != "" {
		r.static = append(r.static, provider.NewOpenAI(key, client))
	}
	if key := os.Getenv("AI_GATEWAY_ANTHROPIC_API_KEY"); key != "" {
		r.static = append(r.static, provider.NewAnthropic(key, client))
	}
	return r
}

func (r *registry) providers(ctx context.Context) ([]provider.Provider, error) {
	var result []provider.Provider
	if r.resolver != nil {
		// Discovery uses the Gateway workload credential, never caller authority.
		bindings, err := r.resolver.ByCapability(ctx, "", chatcompletions.Capability)
		if err != nil {
			if len(r.static) == 0 {
				r.observeHealth(health.Status{State: apphealth.StateUnknown, Reason: apphealth.ReasonDependencyUnavailable, Message: "Provider discovery is unavailable"})
			}
			return nil, err
		}
		result = r.linkedProviders(bindings)
	}
	// Discovery is evidence for Project links, not for static credentials.
	if len(r.static) == 0 {
		if len(result) == 0 {
			r.observeHealth(health.Status{State: apphealth.StateFailing, Reason: apphealth.ReasonDependencyUnavailable, Message: "No AI provider is Ready"})
		} else {
			r.observeHealth(health.Status{State: apphealth.StateServing})
		}
	}
	return append(result, r.static...), nil
}

func (r *registry) linkedProviders(bindings []link.Binding) []provider.Provider {
	var result []provider.Provider
	for _, binding := range bindings {
		if binding.State == link.StateReady && binding.InternalURL != "" && r.configured(binding.Definition) == nil {
			result = append(result, provider.NewCapability(binding.Definition, binding.InternalURL, r.client))
		}
	}
	return result
}

func (r *registry) configured(name string) provider.Provider {
	for _, p := range r.static {
		if p.Name() == name {
			return p
		}
	}
	return nil
}

func (r *registry) lookup(ctx context.Context, name string) (provider.Provider, error) {
	if p := r.configured(name); p != nil {
		return p, nil
	}
	providers, err := r.providers(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range providers {
		if p.Name() == name {
			return p, nil
		}
	}
	return nil, nil
}

func (r *registry) observeHealth(status health.Status) {
	r.healthMu.Lock()
	r.healthResult, r.healthObserved = status, time.Now()
	r.healthStarted = true
	r.healthMu.Unlock()
}

func (r *registry) healthStatus(ctx context.Context) health.Status {
	if r == nil {
		return health.Status{State: apphealth.StateUnknown, Reason: apphealth.ReasonNoEvidence}
	}
	r.healthMu.Lock()
	if !r.healthObserved.IsZero() && time.Since(r.healthObserved) <= providerHealthWindow {
		status := r.healthResult
		r.healthMu.Unlock()
		return status
	}
	if len(r.static) == 0 {
		status := decideResolutionHealth(r.resolver != nil, r.healthStarted)
		r.healthStarted = true
		r.healthMu.Unlock()
		return status
	}
	if r.healthProbes == nil {
		r.healthProbes = make(map[string]*providerHealth, len(r.static))
	}
	probes := make([]*providerHealth, 0, len(r.static))
	for _, p := range r.static {
		probe := r.healthProbes[p.Name()]
		if probe == nil || !probe.observed.IsZero() && time.Since(probe.observed) > providerHealthWindow {
			probe = &providerHealth{done: make(chan struct{})}
			r.healthProbes[p.Name()] = probe
			go r.probeProvider(p, probe)
		}
		probes = append(probes, probe)
	}
	r.healthMu.Unlock()

	result := providerHealthStatus(0, nil)
	for _, probe := range probes {
		select {
		case <-probe.done:
			result = decideProbeHealth(result, probe.status)
		case <-ctx.Done():
			// The shared probe has its own bounded lifetime; a cancelled health
			// reader must not cancel it for other readers or poison its cache.
		}
	}
	r.healthMu.Lock()
	defer r.healthMu.Unlock()
	// Traffic observed while checks were in flight is stronger evidence.
	if !r.healthObserved.IsZero() && time.Since(r.healthObserved) <= providerHealthWindow {
		return r.healthResult
	}
	return result
}

func (r *registry) probeProvider(p provider.Provider, probe *providerHealth) {
	// Finish before the application-health handler's budget. Parallel probes
	// keep one unavailable provider from hiding a reachable alternative.
	ctx, cancel := context.WithTimeout(context.Background(), health.Budget/2)
	defer cancel()
	code, err := p.Probe(ctx)
	r.healthMu.Lock()
	defer r.healthMu.Unlock()
	probe.status = providerHealthStatus(code, err)
	probe.observed = time.Now()
	close(probe.done)
}

func providerHealthStatus(code int, err error) health.Status {
	if err == nil {
		if code == http.StatusOK {
			return health.Status{State: apphealth.StateServing}
		}
		if code == http.StatusUnauthorized || code == http.StatusForbidden {
			return health.Status{State: apphealth.StateFailing, Reason: apphealth.ReasonUpstreamDenied, Message: "Provider rejected the configured credential"}
		}
	}
	return health.Status{State: apphealth.StateFailing, Reason: apphealth.ReasonUpstreamUnavailable, Message: "AI provider is unavailable"}
}
