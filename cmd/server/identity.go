package main

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/appthrust/appserver-go/appserver/identity"
	"github.com/appthrust/appserver-go/appserver/platformclient"
)

// gatewayIdentity keeps inference authorization independent of workload
// discovery. Knowledge assertions authorize only search-backed inference in
// their own Project; OIDC users still require Project membership.
type gatewayIdentity struct {
	auth     *identity.Authenticator
	platform *platformclient.Client
	project  string
}

func gatewayTrust() identity.Trust {
	trust := identity.FromEnv("AI_GATEWAY")
	if trust.AccessIssuer != "" && trust.AccessAudience == "" {
		trust.AccessAudience = "knowledge"
	}
	return trust
}

func (g gatewayIdentity) Authenticate(ctx context.Context, header string) (identity.Identity, string, error) {
	caller, bearer, err := g.auth.Authenticate(ctx, header)
	if err != nil {
		return identity.Identity{}, "", err
	}
	principal := caller.Principal
	if g.project == "" {
		return identity.Identity{}, "", errors.New("gateway Project is required")
	}
	if principal.Kind == "key" {
		if principal.Project != g.project || !slices.Contains(principal.Scopes, "search") {
			return identity.Identity{}, "", errors.New("same-Project search permission is required")
		}
	} else {
		if g.platform == nil {
			return identity.Identity{}, "", errors.New("Project membership is unavailable")
		}
		access, err := g.platform.Access(ctx, principal.Subject, bearer)
		if err != nil {
			return identity.Identity{}, "", err
		}
		if access.NotMember {
			return identity.Identity{}, "", errors.New("Project membership is required")
		}
	}
	return caller, bearer, nil
}

type callerContextKey struct{}
type streamContextKey struct{}

func (g *gateway) authenticate(w http.ResponseWriter, r *http.Request) (identity.Identity, string, bool) {
	caller, bearer, err := g.auth.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		if errors.Is(err, identity.ErrUnavailable) {
			writeError(w, http.StatusServiceUnavailable, "identity_unavailable", "Caller identity is temporarily unavailable")
		} else {
			writeError(w, http.StatusUnauthorized, "ai_unauthorized", "A valid caller bearer is required")
		}
		return identity.Identity{}, "", false
	}
	return caller, bearer, true
}

// Operational reads use the same membership gate, but never inference policy
// enforcement, invocation reporting, or traffic-health observations.
func (g *gateway) authenticatedRead(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller, _, ok := g.authenticate(w, r)
		if !ok {
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), callerContextKey{}, caller)))
	}
}
