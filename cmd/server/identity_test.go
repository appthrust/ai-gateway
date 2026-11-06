package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/appthrust/ai-gateway/cmd/server/internal/provider"
	"github.com/appthrust/appserver-go/accesskey"
	"github.com/appthrust/appserver-go/aigatewaydiscovery"
	"github.com/appthrust/appserver-go/aipolicy"
	"github.com/appthrust/appserver-go/appserver/identity"
	"github.com/appthrust/appserver-go/appserver/link"
	"github.com/appthrust/appserver-go/appserver/platformclient"
	"github.com/stretchr/testify/require"
)

func TestColdKnowledgeAssertionModelsAndCompletionUseWorkloadDiscovery(t *testing.T) {
	for _, tc := range []struct {
		name, audience, project string
		scopes                  []string
		status                  int
	}{
		{"knowledge search", "knowledge", "pj-test", []string{"search"}, http.StatusOK},
		{"wrong audience", "other", "pj-test", []string{"search"}, http.StatusUnauthorized},
		{"other Project", "knowledge", "pj-other", []string{"search"}, http.StatusUnauthorized},
		{"insufficient scope", "knowledge", "pj-test", []string{"documents:read"}, http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key, err := accesskey.GenerateES256Key()
			require.NoError(t, err)
			signer := accesskey.NewES256Signer("test", key)
			upstreamCalls, discoveryCalls := 0, 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upstreamCalls++
				require.Empty(t, r.Header.Get("Authorization"))
				if r.URL.Path == "/v1/models" {
					fmt.Fprint(w, `{"object":"list","data":[{"id":"local","appthrust.io/maxPromptTokens":3000}]}`)
					return
				}
				require.Equal(t, "/v1/chat/completions", r.URL.Path)
				fmt.Fprint(w, `{"model":"local","choices":[{"message":{"content":"answer"}}]}`)
			}))
			defer upstream.Close()
			platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/access/jwks.json":
					require.NoError(t, json.NewEncoder(w).Encode(signer.JWKS()))
				case aigatewaydiscovery.Path:
					discoveryCalls++
					require.Equal(t, "Bearer gateway-workload-key", r.Header.Get("Authorization"))
					require.Equal(t, "pj-test", r.URL.Query().Get("projectId"))
					fmt.Fprintf(w, `{"links":[{"definitionId":"llm","state":"Ready","capabilities":["chat-completions.openai/v1"],"internalUrl":%q}]}`, upstream.URL)
				default:
					t.Errorf("caller assertion reached ordinary Platform API: %s", r.URL)
					w.WriteHeader(http.StatusUnauthorized)
				}
			}))
			defer platform.Close()
			issuer := accesskey.IssuerURL(platform.URL)
			t.Setenv("PLATFORM_ACCESS_ISSUER", issuer)
			t.Setenv("AI_GATEWAY_ACCESS_AUDIENCE", "")
			t.Setenv("PLATFORM_OIDC_ISSUER", "")
			auth, err := identity.New(gatewayTrust())
			require.NoError(t, err)
			resolver := link.NewResolver(platformclient.NewGatewayDiscovery(platform.URL, "pj-test", "gateway-workload-key"), "ai-gateway", nil, nil)
			g := &gateway{auth: gatewayIdentity{auth: auth, project: "pj-test"}, policy: aipolicy.Document{Mode: aipolicy.ModeUnrestricted}, providers: &registry{resolver: resolver, client: provider.NewClient()}}
			token, err := signer.Sign(accesskey.Claims{Issuer: issuer, Audience: tc.audience, Project: tc.project, Subject: accesskey.Subject{Kind: accesskey.SubjectComponent, ID: "app"}, Scopes: tc.scopes, KeyName: "app-knowledge", JTI: "ask", IssuedAt: time.Now(), ExpiresAt: time.Now().Add(time.Minute)})
			require.NoError(t, err)
			// Knowledge forwards this same signed assertion to models first (prompt
			// budget), then completion. Neither cold discovery needs an OIDC warmup.
			for _, path := range []string{"/v1/models", "/v1/chat/completions"} {
				method, body := http.MethodGet, ""
				if path != "/v1/models" {
					method, body = http.MethodPost, `{"model":"local","messages":[{"role":"user","content":"question"}]}`
				}
				r := httptest.NewRequest(method, path, strings.NewReader(body))
				r.Header.Set("Authorization", "Bearer "+token)
				w := httptest.NewRecorder()
				g.handler().ServeHTTP(w, r)
				require.Equal(t, tc.status, w.Code, w.Body.String())
				if tc.status == http.StatusOK && path == "/v1/models" {
					require.Contains(t, w.Body.String(), `"appthrust.io/maxPromptTokens":3000`)
				}
			}
			if tc.status == http.StatusOK {
				require.Equal(t, 1, discoveryCalls)
				require.Equal(t, 2, upstreamCalls)
			} else {
				require.Zero(t, discoveryCalls)
				require.Zero(t, upstreamCalls)
			}
		})
	}
}

func TestGatewayUserMembershipRemainsIndependentOfDiscovery(t *testing.T) {
	for _, member := range []bool{false, true} {
		t.Run(fmt.Sprint(member), func(t *testing.T) {
			bearer := "Bearer " + base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`)) + "." +
				base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"user","email":"user@example.test"}`)) + "."
			platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, bearer, r.Header.Get("Authorization"))
				if !member {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				if r.URL.Path == "/api/v1/projects/pj-test" {
					fmt.Fprint(w, `{"teamSlug":"team"}`)
				} else {
					require.Equal(t, "/api/v1/teams", r.URL.Path)
					fmt.Fprint(w, `{"items":[{"id":"team-id","slug":"team","role":"Viewer"}]}`)
				}
			}))
			defer platform.Close()
			auth, err := identity.New(identity.Trust{})
			require.NoError(t, err)
			g := testGateway(&fakeProvider{name: "configured"}, aipolicy.Document{Mode: aipolicy.ModeUnrestricted})
			g.auth = gatewayIdentity{auth: auth, project: "pj-test", platform: platformclient.New(platform.URL, "pj-test")}
			r := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			r.Header.Set("Authorization", bearer)
			w := httptest.NewRecorder()
			g.handler().ServeHTTP(w, r)
			if member {
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			} else {
				require.Equal(t, http.StatusUnauthorized, w.Code)
			}
		})
	}
}
