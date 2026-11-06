package main

import (
	"context"
	"errors"
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
		if access.NotMember || access.TeamID == "" {
			return identity.Identity{}, "", errors.New("Project membership is required")
		}
	}
	return caller, bearer, nil
}
