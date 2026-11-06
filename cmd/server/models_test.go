package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/appthrust/ai-gateway/cmd/server/internal/provider"
	"github.com/appthrust/appserver-go/aipolicy"
	"github.com/appthrust/appserver-go/appserver/link"
	"github.com/stretchr/testify/require"
)

func TestModelsPartialFailureSkipsUnreadyAndWithdrawnProviders(t *testing.T) {
	for _, mode := range []aipolicy.Mode{aipolicy.ModeUnrestricted, aipolicy.ModeEnforced} {
		t.Run(string(mode), func(t *testing.T) {
			failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "private upstream diagnostic", http.StatusBadGateway)
			}))
			defer failed.Close()
			unready := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("non-Ready provider called") }))
			defer unready.Close()
			bindings := []link.Binding{{Definition: "failed", State: link.StateReady, InternalURL: failed.URL}, {Definition: "pending", State: link.StateInstalling, InternalURL: unready.URL}}
			g := testGateway(&fakeProvider{name: "healthy", models: []provider.ModelEntry{provider.NewModelEntry("good", "healthy")}}, aipolicy.Document{Mode: mode})
			g.providers.resolver = healthResolver(func(_ context.Context, bearer, _ string) ([]link.Binding, error) {
				require.Empty(t, bearer)
				return bindings, nil
			})
			for _, name := range []string{"failed", "pending", "healthy", "removed"} {
				g.policy.Providers = append(g.policy.Providers, aipolicy.Provider{Name: name, ModelRules: []aipolicy.ModelRule{{Patterns: []string{"good"}}}})
			}
			w := callGateway(g, http.MethodGet, "/v1/models", "")
			require.Equal(t, http.StatusOK, w.Code)
			var result struct {
				Data   []provider.ModelEntry `json:"data"`
				Errors []map[string]string   `json:"errors"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
			require.Len(t, result.Data, 1)
			require.Equal(t, "good", result.Data[0].ID)
			require.Equal(t, []map[string]string{{"provider": "failed", "code": "ai_provider_error", "message": "The AI provider could not list models"}}, result.Errors)
			require.NotContains(t, w.Body.String(), "private upstream")
			bindings = nil
			w = callGateway(g, http.MethodGet, "/v1/models", "")
			require.Equal(t, http.StatusOK, w.Code)
			require.NotContains(t, w.Body.String(), "errors")
		})
	}
}
