package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestModelEntryRoundTripPreservesUnknownFields(t *testing.T) {
	const raw = `{ "id": "local-model", "object": "model", "created": 0, "owned_by": "upstream", "context_window": 16384, "appthrust.io/maxPromptTokens": 3000, "vendor": {"values": [1e+06, 9007199254740993, null, true], "label": "future"} }`
	input := []byte(raw)
	var entry ModelEntry
	if err := json.Unmarshal(input, &entry); err != nil {
		t.Fatal(err)
	}
	if entry.ID != "local-model" {
		t.Fatalf("model id=%q", entry.ID)
	}
	// The decoder may reuse its input; the retained object must own its bytes.
	input[0] = '['
	encoded, err := entry.MarshalJSON()
	if err != nil || string(encoded) != raw {
		t.Fatalf("raw model=%s err=%v", encoded, err)
	}
	encoded, err = json.Marshal(entry)
	const compact = `{"id":"local-model","object":"model","created":0,"owned_by":"upstream","context_window":16384,"appthrust.io/maxPromptTokens":3000,"vendor":{"values":[1e+06,9007199254740993,null,true],"label":"future"}}`
	if err != nil || string(encoded) != compact {
		t.Fatalf("model wire=%s err=%v", encoded, err)
	}
}

func TestNewModelEntryUsesStandardFields(t *testing.T) {
	entry := NewModelEntry("model-a", "provider-a")
	encoded, err := json.Marshal(entry)
	if err != nil || entry.ID != "model-a" || string(encoded) != `{"created":0,"id":"model-a","object":"model","owned_by":"provider-a"}` {
		t.Fatalf("model=%+v wire=%s err=%v", entry, encoded, err)
	}
}

func TestProviderHeaderTimeoutsByPurpose(t *testing.T) {
	const discoveryTimeout = 100 * time.Millisecond
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * discoveryTimeout)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/models" {
			_, _ = io.WriteString(w, `{"data":[{"id":"local-model"}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()
	client := newClient(10*discoveryTimeout, discoveryTimeout)
	defer client.CloseIdleConnections()
	local := NewCapability("local", server.URL, client)
	openai := NewOpenAI("key", client).(*openAI)
	openai.base = server.URL + "/v1"
	in := Request{Body: requestBody(t, `{"model":"local-model","messages":[],"input":"hello"}`)}

	for _, tt := range []struct {
		name string
		call func(context.Context, Request) (*http.Response, error)
	}{
		{"chat", func(ctx context.Context, in Request) (*http.Response, error) {
			return local.ChatCompletions(ctx, in, false)
		}},
		{"streaming chat", func(ctx context.Context, in Request) (*http.Response, error) {
			return local.ChatCompletions(ctx, in, true)
		}},
		{"embeddings", openai.Embeddings},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response, err := tt.call(context.Background(), in)
			if err != nil {
				t.Fatalf("inference rejected headers after twice the discovery deadline: %v", err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil || response.StatusCode != http.StatusOK || string(body) != `{"ok":true}` {
				t.Fatalf("status=%d body=%s err=%v", response.StatusCode, body, err)
			}
		})
	}

	t.Run("models", func(t *testing.T) {
		models, err := local.Models(context.Background())
		var timeout net.Error
		if !errors.As(err, &timeout) || !timeout.Timeout() {
			t.Fatalf("expected model discovery header timeout, got models=%v err=%v", models, err)
		}
	})
}

func TestProviderHeaderTimeoutDoesNotLimitResponseBody(t *testing.T) {
	const headerTimeout = 100 * time.Millisecond
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		time.Sleep(2 * headerTimeout)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()
	client := newClient(headerTimeout, headerTimeout)
	defer client.CloseIdleConnections()

	for _, tt := range []struct {
		name   string
		method string
		path   string
	}{
		{"chat", http.MethodPost, "/v1/chat/completions"},
		{"embeddings", http.MethodPost, "/v1/embeddings"},
		{"models", http.MethodGet, "/v1/models"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response, err := request(context.Background(), client, tt.method, server.URL+tt.path, nil, "", nil)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil || string(body) != `{"ok":true}` {
				t.Fatalf("body exceeded the header deadline: body=%s err=%v", body, err)
			}
		})
	}
}

func TestProviderProbeUsesLightweightModelsRequest(t *testing.T) {
	for _, adapter := range []struct {
		name string
		new  func(string, *http.Client) Provider
	}{
		{"openai", func(base string, client *http.Client) Provider {
			p := NewOpenAI("provider-key", client).(*openAI)
			p.base = base + "/v1"
			return p
		}},
		{"anthropic", func(base string, client *http.Client) Provider {
			p := NewAnthropic("provider-key", client).(*anthropic)
			p.base = base + "/v1"
			return p
		}},
		{"capability", func(base string, client *http.Client) Provider {
			return NewCapability("local", base, client)
		}},
	} {
		t.Run(adapter.name, func(t *testing.T) {
			for _, code := range []int{http.StatusOK, http.StatusUnauthorized, http.StatusForbidden, http.StatusServiceUnavailable} {
				calls := 0
				backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					calls++
					if req.Method != http.MethodGet || req.URL.Path != "/v1/models" || req.Header.Get("Accept") != "application/json" {
						t.Error("probe must use the models GET endpoint")
					}
					switch adapter.name {
					case "openai":
						if req.Header.Get("Authorization") != "Bearer provider-key" || req.Header.Get("X-Api-Key") != "" {
							t.Error("invalid OpenAI credential boundary")
						}
					case "anthropic":
						if req.Header.Get("X-Api-Key") != "provider-key" || req.Header.Get("Anthropic-Version") != "2023-06-01" ||
							req.Header.Get("Authorization") != "" || req.URL.RawQuery != "limit=1" {
							t.Error("invalid Anthropic probe credentials or pagination")
						}
					case "capability":
						if req.Header.Get("Authorization") != "" || req.Header.Get("X-Api-Key") != "" {
							t.Error("internal probe forwarded credentials")
						}
					}
					w.WriteHeader(code)
					w.(http.Flusher).Flush()
					// A cheap liveness probe must close after headers, even
					// when the server never supplies a response body.
					<-req.Context().Done()
				}))
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				got, err := adapter.new(backend.URL, backend.Client()).Probe(ctx)
				cancel()
				backend.Close()
				if err != nil || got != code || calls != 1 {
					t.Fatalf("probe status=%d want=%d calls=%d err=%v", got, code, calls, err)
				}
			}
		})
	}
}
