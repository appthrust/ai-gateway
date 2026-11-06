package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

const openAIDefaultModel = "gpt-5-mini"

type compatible struct {
	name         string
	base         string
	defaultModel string
	headers      http.Header
	client       *http.Client
}

// NewOpenAI retains the provider key here, never in a caller request.
func NewOpenAI(key string, client *http.Client) Provider {
	return &openAI{compatible: compatible{name: "openai", base: "https://api.openai.com/v1", defaultModel: openAIDefaultModel,
		headers: http.Header{"Authorization": {"Bearer " + key}}, client: client}}
}

// NewCapability targets an internal, unauthenticated Service.
// Discovery uses the Gateway workload credential; this adapter forwards neither
// it nor the caller bearer. Embeddings are not part of chat-completions.openai/v1,
// so this adapter does not implement Embedder.
func NewCapability(name, internalURL string, client *http.Client) Provider {
	return &compatible{name: name, base: strings.TrimRight(internalURL, "/") + "/v1", client: client}
}

func (p *compatible) Name() string         { return p.name }
func (p *compatible) DefaultModel() string { return p.defaultModel }

func (p *compatible) Probe(ctx context.Context) (int, error) {
	return probe(ctx, p.client, p.base+"/models", p.headers)
}

func (p *compatible) ChatCompletions(ctx context.Context, in Request, _ bool) (*http.Response, error) {
	return request(ctx, p.client, http.MethodPost, p.base+"/chat/completions", in.Body, in.Accept, p.headers)
}

func (p *compatible) Models(ctx context.Context) ([]ModelEntry, error) {
	response, err := request(ctx, p.client, http.MethodGet, p.base+"/models", nil, "application/json", p.headers)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("provider model listing failed")
	}
	var body struct {
		Data []ModelEntry `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return nil, err
	}
	return body.Data, nil
}

type openAI struct{ compatible }

func (p *openAI) Embeddings(ctx context.Context, in Request) (*http.Response, error) {
	return request(ctx, p.client, http.MethodPost, p.base+"/embeddings", in.Body, in.Accept, p.headers)
}
