package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/appthrust/appserver-go/aipolicy"
)

const (
	invocationQueueCapacity = 1024
	invocationQueueBytes    = 4 << 20
	invocationRecordTTL     = 2 * time.Minute
)

type dropReason int

const (
	dropQueueFull dropReason = iota
	dropQueueBytes
	dropRejected
	dropExpired
	dropShutdown
	dropEncode
	dropReasonCount
)

func (r dropReason) String() string {
	switch r {
	case dropQueueFull:
		return "queue_full"
	case dropQueueBytes:
		return "queue_bytes"
	case dropRejected:
		return "rejected"
	case dropExpired:
		return "expired"
	case dropShutdown:
		return "shutdown"
	case dropEncode:
		return "encode"
	default:
		return "unknown"
	}
}

type invocationItem struct {
	bearer     string
	body       []byte
	enqueuedAt time.Time
}

func (item invocationItem) size() int64 { return int64(len(item.body)) + int64(len(item.bearer)) }

type recorder struct {
	mu          sync.Mutex
	closed      bool
	queue       chan invocationItem
	done        chan struct{}
	ctx         context.Context
	cancel      context.CancelFunc
	endpoint    string
	client      *http.Client
	queuedBytes atomic.Int64 // Includes the in-flight body and bearer.
	delivered   atomic.Uint64
	dropped     [dropReasonCount]atomic.Uint64
}

func newRecorder(ctx context.Context, origin, projectID string, transport http.RoundTripper) *recorder {
	worker, cancel := context.WithCancel(ctx)
	p := &recorder{
		queue: make(chan invocationItem, invocationQueueCapacity), done: make(chan struct{}),
		ctx: worker, cancel: cancel,
		endpoint: strings.TrimRight(origin, "/") + "/api/v1/projects/" + url.PathEscape(projectID) + "/ai-invocations",
		client:   &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
	go p.run()
	return p
}

// enqueue snapshots the record and caller bearer without waiting for delivery.
// Overflow drops the newest record, preserving the FIFO of accepted records.
func (p *recorder) enqueue(bearer string, record aipolicy.InvocationRecord) {
	if p == nil {
		return
	}
	body, err := json.Marshal(record)
	if err != nil {
		p.drop(dropEncode, 0)
		return
	}
	item := invocationItem{bearer: bearer, body: body, enqueuedAt: time.Now()}
	p.mu.Lock()
	if p.closed || p.ctx.Err() != nil {
		p.mu.Unlock()
		p.drop(dropShutdown, 0)
		return
	}
	if p.queuedBytes.Load()+item.size() > invocationQueueBytes {
		p.mu.Unlock()
		p.drop(dropQueueBytes, 0)
		return
	}
	p.queuedBytes.Add(item.size())
	select {
	case p.queue <- item:
		p.mu.Unlock()
	default:
		p.queuedBytes.Add(-item.size())
		p.mu.Unlock()
		p.drop(dropQueueFull, 0)
	}
}

func (p *recorder) DeliveredCount() uint64 {
	if p == nil {
		return 0
	}
	return p.delivered.Load()
}

func (p *recorder) DroppedCount(reason dropReason) uint64 {
	if p == nil || reason < 0 || reason >= dropReasonCount {
		return 0
	}
	return p.dropped[reason].Load()
}

func (p *recorder) drop(reason dropReason, status int) {
	p.dropped[reason].Add(1)
	// Neither bearers, provider errors nor request/response bodies belong in logs.
	if status != 0 {
		log.Printf("AI invocation reporting dropped: %s (HTTP %d)", reason, status)
	} else {
		log.Printf("AI invocation reporting dropped: %s", reason)
	}
}

func (p *recorder) stopAccepting() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.closed {
		p.closed = true
		close(p.queue)
	}
}

// Close stops admission and drains until ctx expires, then cancels delivery.
// The worker also stops if the construction context is canceled.
func (p *recorder) Close(ctx context.Context) error {
	if p == nil {
		return nil
	}
	p.stopAccepting()
	select {
	case <-p.done:
		return nil
	default:
	}
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		p.cancel()
		return ctx.Err()
	}
}

func (p *recorder) run() {
	defer close(p.done)
	defer p.cancel()
	defer p.client.CloseIdleConnections()
	for {
		select {
		case <-p.ctx.Done():
			p.stopAccepting()
			for item := range p.queue {
				p.queuedBytes.Add(-item.size())
				p.drop(dropShutdown, 0)
			}
			return
		case item, ok := <-p.queue:
			if !ok {
				return
			}
			p.deliver(item)
			p.queuedBytes.Add(-item.size())
		}
	}
}

func (p *recorder) deliver(item invocationItem) {
	ctx, cancel := context.WithDeadline(p.ctx, item.enqueuedAt.Add(invocationRecordTTL))
	defer cancel()
	backoff := 100 * time.Millisecond
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	for {
		if ctx.Err() != nil {
			reason := dropExpired
			if p.ctx.Err() != nil {
				reason = dropShutdown
			}
			p.drop(reason, 0)
			return
		}
		status, err := p.post(ctx, item)
		if err == nil {
			if status == http.StatusAccepted {
				p.delivered.Add(1)
				return
			}
			if status != http.StatusRequestTimeout && status != http.StatusTooManyRequests && (status < 500 || status >= 600) {
				p.drop(dropRejected, status)
				return
			}
		}
		if timer == nil {
			timer = time.NewTimer(backoff)
		} else {
			timer.Reset(backoff)
		}
		select {
		case <-ctx.Done():
		case <-timer.C:
		}
		backoff = min(backoff*2, 5*time.Second)
	}
}

// post performs one attempt. Only 202 acknowledges delivery; redirects are
// never followed, and response bodies are discarded without logging them.
func (p *recorder) post(ctx context.Context, item invocationItem) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(item.body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", item.bearer)
	req.Header.Set("Content-Type", "application/json")
	response, err := p.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	return response.StatusCode, nil
}

// Canonical text is UTF-8, concatenated with no separators: messages in request
// order, text parts in part order; completions in received choice/delta order.
// Hash incrementally so streaming never retains the complete conversation.
func writeContent(h hash.Hash, raw json.RawMessage) {
	if h == nil || len(raw) == 0 || string(raw) == "null" {
		return
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		_, _ = io.WriteString(h, text)
		return
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		for _, part := range parts {
			if part.Type == "text" {
				_, _ = io.WriteString(h, part.Text)
			}
		}
	}
}

func requestDigest(body map[string]json.RawMessage) string {
	h := sha256.New()
	var messages []struct {
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(body["messages"], &messages) == nil {
		for _, message := range messages {
			writeContent(h, message.Content)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

type responseObserver struct {
	record     *aipolicy.InvocationRecord
	completion hash.Hash
	done       bool
}

func newObserver(record *aipolicy.InvocationRecord) *responseObserver {
	observer := &responseObserver{record: record}
	if record.Logging != nil && record.Logging.Conversation == "HashOnly" {
		observer.completion = sha256.New()
	}
	return observer
}

func (o *responseObserver) observe(data []byte) error {
	var body struct {
		Model string          `json:"model"`
		Error json.RawMessage `json:"error"`
		Usage *struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
		} `json:"usage"`
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
			Delta struct {
				Content json.RawMessage `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return errors.New("invalid provider JSON")
	}
	if len(body.Error) != 0 && string(body.Error) != "null" {
		return errors.New("provider returned an error")
	}
	// The response supplies the actual default model when the request omitted it.
	if o.record.Model == "" && body.Model != "" {
		o.record.Model = body.Model
	}
	if body.Usage != nil {
		o.record.Usage = &aipolicy.Usage{PromptTokens: body.Usage.PromptTokens, CompletionTokens: body.Usage.CompletionTokens}
	}
	for _, choice := range body.Choices {
		writeContent(o.completion, choice.Message.Content)
		writeContent(o.completion, choice.Delta.Content)
	}
	return nil
}

func (o *responseObserver) finish() {
	if o.completion != nil {
		o.record.CompletionDigest = hex.EncodeToString(o.completion.Sum(nil))
	}
}
