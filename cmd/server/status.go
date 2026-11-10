package main

import (
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/appthrust/appserver-go/aipolicy"
	"github.com/appthrust/appserver-go/apphealth"
	"github.com/appthrust/appserver-go/appserver/health"
	"github.com/appthrust/appserver-go/appserver/link"
	"github.com/appthrust/appserver-go/appserver/releaseidentity"
)

type policyView struct {
	Name       string `json:"name"`
	UID        string `json:"uid"`
	Generation int64  `json:"generation"`
}

type requirementCandidate struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Satisfied bool   `json:"satisfied"`
}

type configuredView struct {
	Satisfied  bool                   `json:"satisfied"`
	Candidates []requirementCandidate `json:"candidates"`
}

type healthView struct {
	State      string     `json:"state"`
	Reason     string     `json:"reason"`
	Message    string     `json:"message"`
	ObservedAt *time.Time `json:"observedAt"`
}

type defaultRoute struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Source   string `json:"source"`
}

type nextStep struct {
	Kind         string `json:"kind"`
	Slot         string `json:"slot,omitempty"`
	DefinitionID string `json:"definitionId,omitempty"`
	Provider     string `json:"provider,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

type endpointView struct {
	Public   string  `json:"public"`
	Internal *string `json:"internal"`
}

type releaseView struct {
	Version        string `json:"version"`
	ArtifactDigest string `json:"artifactDigest"`
}

type GatewayStatus struct {
	Mode         aipolicy.Mode     `json:"mode"`
	Policy       *policyView       `json:"policy"`
	Configured   configuredView    `json:"configured"`
	Health       healthView        `json:"health"`
	DefaultRoute *defaultRoute     `json:"defaultRoute"`
	Endpoints    endpointView      `json:"endpoints"`
	Release      releaseView       `json:"release"`
	Logging      *aipolicy.Logging `json:"logging"`
	NextSteps    []nextStep        `json:"nextSteps"`
}

func decideConfigured(snapshot providerSnapshot) configuredView {
	v := configuredView{Candidates: []requirementCandidate{}}
	for _, slot := range credentialSlots {
		bound := false
		for _, p := range snapshot.static {
			bound = bound || observationID(p) == slot.Name
		}
		v.Candidates = append(v.Candidates, requirementCandidate{Kind: "CredentialSlot", Name: slot.Name, Satisfied: bound})
		v.Satisfied = v.Satisfied || bound
	}
	for _, b := range snapshot.bindings {
		if b.State == link.StateReady && b.InternalURL != "" {
			v.Candidates = append(v.Candidates, requirementCandidate{Kind: "Link", Name: b.Definition, Satisfied: true})
			v.Satisfied = true
		}
	}
	return v
}

func decidePolicyHealth(document aipolicy.Document) (health.Status, bool) {
	if document.Mode == aipolicy.ModePending {
		return health.Status{State: apphealth.StateFailing, Reason: apphealth.ReasonUpstreamDenied, Message: "The Project AI policy is not ready"}, true
	}
	if document.Mode == aipolicy.ModeEnforced && document.Logging != nil && document.Logging.Conversation == "Full" {
		return health.Status{State: apphealth.StateFailing, Reason: apphealth.ReasonDependencyUnavailable, Message: "Required conversation logging is unavailable"}, true
	}
	return health.Status{}, false
}

// Failed alternatives remain per-provider findings, not capability degradation.
func decideCapabilityHealth(usable, unchecked bool, failure health.Status) health.Status {
	if usable {
		return health.Status{State: apphealth.StateServing}
	}
	if unchecked {
		return health.Status{State: apphealth.StateUnknown, Reason: apphealth.ReasonNoEvidence, Message: "Provider availability has not been established"}
	}
	if failure.State == apphealth.StateFailing {
		return failure
	}
	return health.Status{State: apphealth.StateFailing, Reason: apphealth.ReasonDependencyUnavailable, Message: "No AI provider is Ready"}
}

func decideHealth(document aipolicy.Document, snapshot providerSnapshot, now time.Time) healthView {
	status, blocked := decidePolicyHealth(document)
	var observed time.Time
	if !blocked {
		usable, unchecked := false, snapshot.discoveryFailed
		failure := health.Status{}
		add := func(name string, o providerObservation, ready bool) {
			if document.Mode == aipolicy.ModeEnforced && !providerInPolicy(document, name) {
				return
			}
			if o.observed.After(observed) {
				observed = o.observed
			}
			if o.observed.IsZero() {
				usable = usable || ready
				unchecked = unchecked || !ready
			} else if o.status.State == apphealth.StateServing {
				usable = true
			} else if o.status.State == apphealth.StateFailing {
				if failure.State == "" || o.status.Reason == apphealth.ReasonUpstreamDenied {
					failure = o.status
				}
			} else {
				unchecked = true
			}
		}
		for _, p := range snapshot.static {
			add(p.Name(), snapshot.observations[observationID(p)], false)
		}
		for _, binding := range snapshot.bindings {
			if binding.State != link.StateReady || binding.InternalURL == "" {
				continue
			}
			o := snapshot.observations[binding.Definition]
			// A refreshed Ready link supersedes old inference failures.
			if !o.observed.IsZero() && now.Sub(o.observed) > providerHealthWindow {
				o = providerObservation{}
			}
			add(binding.Definition, o, true)
		}
		status = decideCapabilityHealth(usable, unchecked, failure)
		if status.State == apphealth.StateUnknown && snapshot.discoveryFailed {
			status.Reason, status.Message = apphealth.ReasonDependencyUnavailable, "Provider discovery is unavailable"
		}
		if status.State == apphealth.StateServing && !snapshot.policyDeniedAt.IsZero() && now.Sub(snapshot.policyDeniedAt) <= providerHealthWindow {
			status = health.Status{State: apphealth.StateDegraded, Reason: apphealth.ReasonUpstreamDenied}
			observed = snapshot.policyDeniedAt
		}
	}
	v := healthView{State: status.State, Reason: status.Reason, Message: status.Message}
	if !observed.IsZero() {
		at := observed.UTC()
		v.ObservedAt = &at
	}
	return v
}

func decideDefaultRoute(document aipolicy.Document, snapshot providerSnapshot, defaultProvider, defaultModel string) *defaultRoute {
	if _, blocked := decidePolicyHealth(document); blocked {
		return nil
	}
	record := aipolicy.InvocationRecord{}
	if document.Mode == aipolicy.ModeEnforced {
		decideEnforced(document, &record)
		if record.Model == "" || !document.Allows(record.Provider, record.Model).Allowed {
			return nil
		}
	}
	selected := decideProvider(document.Mode, defaultProvider, defaultModel, snapshot.available, &record)
	if selected == nil {
		return nil
	}
	source := "Capability"
	for _, p := range snapshot.static {
		if p.Name() == selected.Name() {
			source = "Credential"
			break
		}
	}
	if defaultProvider != "" || defaultModel != "" {
		source = "EnvOverride"
	}
	if document.Mode == aipolicy.ModeEnforced {
		source = "Policy"
	}
	return &defaultRoute{Provider: record.Provider, Model: record.Model, Source: source}
}

func decideNextSteps(document aipolicy.Document, configured configuredView, health healthView, providers []ProviderView) []nextStep {
	steps := []nextStep{}
	if document.Mode == aipolicy.ModePending {
		return append(steps, nextStep{Kind: "ContactAdmin", Reason: "ai_policy_pending"})
	}
	if document.Mode == aipolicy.ModeEnforced && document.Logging != nil && document.Logging.Conversation == "Full" {
		return append(steps, nextStep{Kind: "ContactAdmin", Reason: "ai_policy_logging_unavailable"})
	}
	if !configured.Satisfied {
		for _, c := range configured.Candidates {
			if c.Kind == "CredentialSlot" && !c.Satisfied {
				steps = append(steps, nextStep{Kind: "BindCredential", Slot: c.Name})
			}
		}
	}
	install := false
	for _, p := range providers {
		keyFailure := false
		for _, key := range p.Keys {
			if key.Bound && (key.State == "rejected" || key.State == "unreachable") {
				steps = append(steps, nextStep{Kind: "FixProvider", Provider: p.Name, Slot: key.Slot})
				keyFailure = true
			}
		}
		switch p.State {
		case "Denied", "Unreachable":
			if !keyFailure {
				steps = append(steps, nextStep{Kind: "FixProvider", Provider: p.Name})
			}
		case "NotInstalled":
			if !configured.Satisfied {
				steps = append(steps, nextStep{Kind: "InstallApp", DefinitionID: p.Name})
				install = true
			}
		}
	}
	if !configured.Satisfied && !install {
		steps = append(steps, nextStep{Kind: "InstallApp", DefinitionID: "inference"})
	}
	if document.Mode == aipolicy.ModeUnrestricted && health.State != apphealth.StateServing {
		steps = append(steps, nextStep{Kind: "BindPolicy"})
	}
	return steps
}

// Only the gateway's own binding or injected Knative Service identity is used.
// A provider's upstream URL is never an endpoint of this gateway.
func internalEndpoint() *string {
	base := os.Getenv("AI_GATEWAY_URL")
	if base == "" {
		service, namespace := os.Getenv("K_SERVICE"), os.Getenv("POD_NAMESPACE")
		if namespace == "" {
			namespace = os.Getenv("NAMESPACE")
		}
		if service == "" || namespace == "" {
			return nil
		}
		base = "http://" + service + "." + namespace + ".svc"
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Scheme != "http" && u.Scheme != "https" {
		return nil
	}
	endpoint := strings.TrimRight(base, "/") + "/v1"
	return &endpoint
}

func (g *gateway) status(w http.ResponseWriter, r *http.Request) {
	snapshot := g.providers.snapshot(r.Context())
	now := time.Now()
	v := GatewayStatus{Mode: g.policy.Mode, Logging: g.policy.Logging,
		Configured:   decideConfigured(snapshot),
		Health:       decideHealth(g.policy, snapshot, now),
		DefaultRoute: decideDefaultRoute(g.policy, snapshot, g.defaultProvider, g.defaultModel),
		Endpoints:    endpointView{Internal: internalEndpoint()},
	}
	if g.policy.Mode != aipolicy.ModeUnrestricted && g.policy.Policy != nil {
		p := g.policy.Policy
		v.Policy = &policyView{Name: p.Name, UID: p.UID, Generation: p.Generation}
	}
	if public := os.Getenv("AI_GATEWAY_PUBLIC_URL"); public != "" {
		v.Endpoints.Public = strings.TrimRight(public, "/") + "/v1"
	}
	digest, version := releaseidentity.HealthIdentity("AI_GATEWAY")()
	v.Release = releaseView{Version: version, ArtifactDigest: digest}
	v.NextSteps = decideNextSteps(g.policy, v.Configured, v.Health, decideProviders(g.policy, snapshot, v.DefaultRoute, now))
	writeJSON(w, http.StatusOK, v)
}
