package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestOpenAIDefaultBaseURL(t *testing.T) {
	p := NewOpenAI("provider-key", NewClient()).(*openAI)
	if p.base != "https://api.openai.com/v1" {
		t.Fatalf("default base=%q", p.base)
	}
}

func TestOpenAIChatEmbeddingsModelsAndCredentialBoundary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer provider-key" || r.Header.Get("X-Api-Key") != "" {
			t.Errorf("credential headers=%v", r.Header)
		}
		switch r.URL.Path {
		case "/v1/models":
			fmt.Fprint(w, `{"object":"list","data":[{"id":"model-a","object":"model","owned_by":"upstream","extra":{"tokens":3000}},{"id":"model-b","object":"model"}]}`)
		case "/v1/chat/completions", "/v1/embeddings":
			if r.Method != http.MethodPost || r.Header.Get("Accept") != "application/json" {
				t.Errorf("request=%s headers=%v", r.Method, r.Header)
			}
			var body map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if string(body["model"]) != `"explicit-model"` {
				t.Errorf("model must be forwarded as-is: %s", body["model"])
			}
			fmt.Fprint(w, `{"usage":{"prompt_tokens":3,"completion_tokens":1}}`)
		default:
			t.Errorf("unexpected route %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	p := NewOpenAI("provider-key", NewClient()).(*openAI)
	p.base = server.URL + "/v1"
	models, err := p.Models(context.Background())
	if err != nil || len(models) != 2 || models[0].ID != "model-a" || models[1].ID != "model-b" {
		t.Fatalf("models=%v err=%v", models, err)
	}
	encoded, err := json.Marshal(models)
	if err != nil || string(encoded) != `[{"id":"model-a","object":"model","owned_by":"upstream","extra":{"tokens":3000}},{"id":"model-b","object":"model"}]` {
		t.Fatalf("models wire=%s err=%v", encoded, err)
	}
	in := Request{Body: requestBody(t, `{"model":"explicit-model","input":"hello"}`), Accept: "application/json"}
	for _, call := range []func(context.Context, Request) (*http.Response, error){
		func(ctx context.Context, in Request) (*http.Response, error) {
			return p.ChatCompletions(ctx, in, false)
		},
		p.Embeddings,
	} {
		response, err := call(context.Background(), in)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil || !strings.Contains(string(body), `"prompt_tokens":3`) {
			t.Fatalf("response=%s err=%v", body, err)
		}
	}
}

func TestRedirectNeverForwardsCredentials(t *testing.T) {
	var called atomic.Bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { called.Store(true) }))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	p := NewOpenAI("private-key", NewClient()).(*openAI)
	p.base = source.URL
	response, err := p.ChatCompletions(context.Background(), Request{Body: requestBody(t, `{}`)}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusTemporaryRedirect || called.Load() {
		t.Fatalf("redirect status=%d followed=%t", response.StatusCode, called.Load())
	}
}

func TestStreamingClientHasOnlyConnectHeaderAndIdleLimits(t *testing.T) {
	client := NewClient()
	defer client.CloseIdleConnections()
	if client.Timeout != 0 || IdleTimeout != 300*time.Second {
		t.Fatalf("stream client timeout=%v idle timeout=%v", client.Timeout, IdleTimeout)
	}
	transport := client.Transport.(*purposeTransport)
	for _, tt := range []struct {
		name          string
		transport     *http.Transport
		headerTimeout time.Duration
	}{
		{"inference", transport.inference, IdleTimeout},
		{"discovery", transport.discovery, 30 * time.Second},
	} {
		if tt.transport.ResponseHeaderTimeout != tt.headerTimeout || tt.transport.TLSHandshakeTimeout != 10*time.Second || tt.transport.DialContext == nil {
			t.Errorf("%s transport has incorrect connect/header deadlines: %+v", tt.name, tt.transport)
		}
	}
}
