// Package provider owns provider credentials, wire translation, and transport.
package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/appthrust/appserver-go/appserver/contract/chatcompletions"
)

// Request contains OpenAI fields after gateway-only routing fields are removed.
// Accept is the only caller header that may cross the provider boundary.
type Request struct {
	Body   map[string]json.RawMessage
	Accept string
}

type Provider interface {
	Name() string
	DefaultModel() string
	ChatCompletions(context.Context, Request, bool) (*http.Response, error)
	Models(context.Context) ([]ModelEntry, error)
	// Probe checks credential acceptance and reachability without reading model
	// data. It returns only the HTTP status; callers must not expose raw errors.
	Probe(context.Context) (int, error)
}

// ModelEntry retains the provider's entire JSON object. ID is decoded once for
// policy filtering and deduplication; unknown fields are never reinterpreted.
type ModelEntry struct {
	ID  string
	raw json.RawMessage
}

func (m *ModelEntry) UnmarshalJSON(data []byte) error {
	var model struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &model); err != nil {
		return err
	}
	m.ID = model.ID
	return m.raw.UnmarshalJSON(data)
}

func (m ModelEntry) MarshalJSON() ([]byte, error) {
	return m.raw.MarshalJSON()
}

// NewModelEntry synthesizes the standard fields when no OpenAI model object is
// available, using the capability contract's model shape.
func NewModelEntry(id, owner string) ModelEntry {
	raw, _ := json.Marshal(chatcompletions.Model{ID: id, Object: "model", OwnedBy: owner})
	return ModelEntry{ID: id, raw: raw}
}

// Embedder is optional: not every chat provider implements embeddings.
type Embedder interface {
	Embeddings(context.Context, Request) (*http.Response, error)
}

const IdleTimeout = 300 * time.Second

// NewClient allows 300 seconds for inference headers and 30 seconds for model
// discovery, with 10-second connect/TLS deadlines and no whole-response deadline.
// The response body is cancelled after 300 seconds without upstream data.
func NewClient() *http.Client {
	return newClient(IdleTimeout, 30*time.Second)
}

func newClient(inferenceHeaderTimeout, discoveryHeaderTimeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ResponseHeaderTimeout = inferenceHeaderTimeout
	discovery := transport.Clone()
	discovery.ResponseHeaderTimeout = discoveryHeaderTimeout
	return &http.Client{Transport: &purposeTransport{inference: transport, discovery: discovery}, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

type purposeTransport struct {
	inference *http.Transport
	discovery *http.Transport
}

func (t *purposeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// GETs list models; inference POSTs may spend minutes evaluating a prompt.
	if req.Method == http.MethodGet {
		return t.discovery.RoundTrip(req)
	}
	return t.inference.RoundTrip(req)
}

func (t *purposeTransport) CloseIdleConnections() {
	t.inference.CloseIdleConnections()
	t.discovery.CloseIdleConnections()
}

func probe(ctx context.Context, client *http.Client, endpoint string, headers http.Header) (int, error) {
	response, err := request(ctx, client, http.MethodGet, endpoint, nil, "application/json", headers)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	return response.StatusCode, nil
}

func request(ctx context.Context, client *http.Client, method, endpoint string, body any, accept string, headers http.Header) (*http.Response, error) {
	var input io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		input = bytes.NewReader(encoded)
	}
	ctx, cancel := context.WithCancel(ctx)
	req, err := http.NewRequestWithContext(ctx, method, endpoint, input)
	if err != nil {
		cancel()
		return nil, err
	}
	req.Header = headers.Clone()
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	response, err := client.Do(req)
	if err != nil {
		cancel()
		return nil, err
	}
	response.Body = &idleBody{ReadCloser: response.Body, cancel: cancel, timer: time.AfterFunc(IdleTimeout, cancel)}
	return response, nil
}

type idleBody struct {
	io.ReadCloser
	cancel context.CancelFunc
	timer  *time.Timer
	mu     sync.Mutex
	closed bool
}

func (b *idleBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.mu.Lock()
	if n > 0 && !b.closed {
		b.timer.Reset(IdleTimeout)
	}
	b.mu.Unlock()
	return n, err
}

func (b *idleBody) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	b.timer.Stop()
	b.mu.Unlock()
	b.cancel()
	return b.ReadCloser.Close()
}
