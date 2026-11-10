package main

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/appthrust/ai-gateway/cmd/server/internal/provider"
	"github.com/appthrust/appserver-go/aipolicy"
	"github.com/appthrust/appserver-go/apphealth"
	"github.com/appthrust/appserver-go/appserver/contract/chatcompletions"
	"github.com/appthrust/appserver-go/appserver/health"
	"github.com/appthrust/appserver-go/appserver/link"
)

type capabilityResolver interface {
	ByCapability(context.Context, string, string) ([]link.Binding, error)
}

// The resolver owns the binding cache. Health retains only observations, never
// caller credentials or a second routing cache.
type registry struct {
	static         []provider.Provider
	resolver       capabilityResolver
	client         *http.Client
	healthMu       sync.Mutex
	healthProbes   map[string]*providerHealth
	policyDeniedAt time.Time
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

func (r *registry) providers(ctx context.Context) ([]provider.Provider, error) {
	var result []provider.Provider
	if r.resolver != nil {
		bindings, err := r.resolver.ByCapability(ctx, "", chatcompletions.Capability)
		if err != nil {
			if len(r.static) == 0 {
				return nil, err
			}
		} else {
			result = r.linkedProviders(bindings)
		}
	}
	return append(result, r.selectedStatic()...), nil
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
	for _, p := range r.selectedStatic() {
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

func (r *registry) observeProvider(p provider.Provider, status health.Status) {
	r.healthMu.Lock()
	defer r.healthMu.Unlock()
	if r.healthProbes == nil {
		r.healthProbes = make(map[string]*providerHealth)
	}
	id := observationID(p)
	probe := r.healthProbes[id]
	if probe == nil {
		probe = &providerHealth{}
		r.healthProbes[id] = probe
	}
	probe.status, probe.observed = status, time.Now()
}

func (r *registry) observePolicyDenial(denied bool) {
	r.healthMu.Lock()
	defer r.healthMu.Unlock()
	r.policyDeniedAt = time.Time{}
	if denied {
		r.policyDeniedAt = time.Now()
	}
}

func (r *registry) healthStatus(ctx context.Context) health.Status {
	return r.healthForPolicy(ctx, aipolicy.Document{Mode: aipolicy.ModeUnrestricted})
}

func (r *registry) healthForPolicy(ctx context.Context, document aipolicy.Document) health.Status {
	if r == nil {
		return health.Status{State: apphealth.StateUnknown, Reason: apphealth.ReasonNoEvidence}
	}
	r.healthMu.Lock()
	if r.healthProbes == nil {
		r.healthProbes = make(map[string]*providerHealth, len(r.static))
	}
	var pending []chan struct{}
	for _, p := range r.static {
		id := observationID(p)
		probe := r.healthProbes[id]
		if probe == nil {
			probe = &providerHealth{}
			r.healthProbes[id] = probe
		}
		if probe.done == nil && (probe.observed.IsZero() || time.Since(probe.observed) > providerHealthWindow) {
			probe.done = make(chan struct{})
			go r.probeProvider(p, probe)
		}
		if probe.done != nil {
			pending = append(pending, probe.done)
		}
	}
	r.healthMu.Unlock()
	// Discovery has workload authority only. Drop all caller context values.
	discoveryCtx, cancel := context.WithTimeout(context.Background(), health.Budget/2)
	defer cancel()
	snapshot := r.snapshot(discoveryCtx)
	for _, done := range pending {
		select {
		case <-done:
		case <-ctx.Done():
		}
	}
	r.refreshObservations(&snapshot)
	v := decideHealth(document, snapshot, time.Now())
	return health.Status{State: v.State, Reason: v.Reason, Message: v.Message}
}

func (r *registry) probeProvider(p provider.Provider, probe *providerHealth) {
	// Finish before the application-health handler's budget. Parallel probes
	// keep one unavailable provider from hiding a reachable alternative.
	ctx, cancel := context.WithTimeout(context.Background(), health.Budget/2)
	defer cancel()
	started := time.Now()
	code, err := p.Probe(ctx)
	r.healthMu.Lock()
	defer r.healthMu.Unlock()
	if !probe.observed.After(started) {
		probe.status = providerHealthStatus(code, err)
		probe.observed = time.Now()
	}
	close(probe.done)
	probe.done = nil
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
