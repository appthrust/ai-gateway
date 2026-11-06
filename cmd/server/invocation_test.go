package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/appthrust/appserver-go/aipolicy"
)

type recorderTransport func(*http.Request) (*http.Response, error)

func (f recorderTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func recorderResponse(status int) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("receiver-secret"))}
}

func testRecorder(t *testing.T, transport http.RoundTripper) *recorder {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	p := newRecorder(ctx, "http://platform.test", "pj-test", transport)
	t.Cleanup(func() {
		cancel()
		closeRecorder(t, p)
	})
	return p
}

func closeRecorder(t *testing.T, p *recorder) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := p.Close(ctx); err != nil {
		t.Fatalf("recorder close: %v", err)
	}
}

func TestRecorderRetriesPreserveRecord(t *testing.T) {
	record := aipolicy.InvocationRecord{
		Provider: "test", Model: "original-model", Outcome: aipolicy.OutcomeAllowed, Status: http.StatusOK,
		Usage: &aipolicy.Usage{PromptTokens: 7, CompletionTokens: 3}, DurationMs: 42,
	}
	wantBody, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	var attempts atomic.Int32
	accepted := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/projects/pj-test/ai-invocations" || r.Header.Get("Authorization") != "Bearer original-caller" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("invocation destination, method, bearer or content type changed")
		}
		if !bytes.Equal(body, wantBody) {
			t.Error("invocation body changed between enqueue and delivery")
		}
		if attempts.Add(1) <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		accepted <- struct{}{}
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(context.Background())
	p := newRecorder(ctx, server.URL, "pj-test", server.Client().Transport)
	t.Cleanup(func() {
		cancel()
		closeRecorder(t, p)
	})
	p.enqueue("Bearer original-caller", record)
	// The queued body is a snapshot, including fields held behind pointers.
	record.Model = "changed-model"
	record.Usage.PromptTokens = 999
	select {
	case <-accepted:
	case <-time.After(3 * time.Second):
		t.Fatal("record was not delivered after the Platform API recovered")
	}
	closeRecorder(t, p)
	if attempts.Load() != 3 || p.DeliveredCount() != 1 || p.queuedBytes.Load() != 0 {
		t.Fatalf("attempts=%d delivered=%d queued bytes=%d", attempts.Load(), p.DeliveredCount(), p.queuedBytes.Load())
	}
	for reason := dropReason(0); reason < dropReasonCount; reason++ {
		if p.DroppedCount(reason) != 0 {
			t.Fatalf("unexpected drop: %s", reason)
		}
	}
}

func TestRecorderDeliveryClassification(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		failure   string
		attempts  int
		delivered uint64
		elapsed   time.Duration
	}{
		{name: "accepted", status: 202, attempts: 1, delivered: 1},
		{name: "request timeout", status: 408, attempts: 2, delivered: 1, elapsed: 100 * time.Millisecond},
		{name: "rate limited", status: 429, attempts: 2, delivered: 1, elapsed: 100 * time.Millisecond},
		{name: "server error", status: 500, attempts: 2, delivered: 1, elapsed: 100 * time.Millisecond},
		{name: "unavailable", status: 503, attempts: 2, delivered: 1, elapsed: 100 * time.Millisecond},
		{name: "transport error", failure: "transport", attempts: 2, delivered: 1, elapsed: 100 * time.Millisecond},
		{name: "attempt timeout", failure: "timeout", attempts: 2, delivered: 1, elapsed: 5*time.Second + 100*time.Millisecond},
		{name: "bad request", status: 400, attempts: 1},
		{name: "unauthorized", status: 401, attempts: 1},
		{name: "forbidden", status: 403, attempts: 1},
		{name: "too large", status: 413, attempts: 1},
		{name: "not accepted", status: 200, attempts: 1},
		{name: "redirect", status: 302, attempts: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var logs bytes.Buffer
				previous := log.Writer()
				log.SetOutput(&logs)
				defer log.SetOutput(previous)
				attempts := 0
				p := testRecorder(t, recorderTransport(func(r *http.Request) (*http.Response, error) {
					attempts++
					deadline, ok := r.Context().Deadline()
					if !ok || time.Until(deadline) > 5*time.Second {
						t.Error("HTTP attempt lacks the five-second deadline")
					}
					if attempts > 1 {
						return recorderResponse(http.StatusAccepted), nil
					}
					switch tc.failure {
					case "transport":
						return nil, errors.New("transport-secret")
					case "timeout":
						<-r.Context().Done()
						return nil, r.Context().Err()
					default:
						response := recorderResponse(tc.status)
						response.Header.Set("Location", "http://another.test/do-not-follow")
						return response, nil
					}
				}))
				started := time.Now()
				p.enqueue("Bearer caller-secret", aipolicy.InvocationRecord{Model: "body-secret"})
				closeRecorder(t, p)
				if attempts != tc.attempts || p.DeliveredCount() != tc.delivered || p.DroppedCount(dropRejected) != 1-tc.delivered {
					t.Fatalf("attempts=%d delivered=%d rejected=%d", attempts, p.DeliveredCount(), p.DroppedCount(dropRejected))
				}
				if elapsed := time.Since(started); elapsed != tc.elapsed {
					t.Fatalf("delivery elapsed=%s want=%s", elapsed, tc.elapsed)
				}
				if tc.delivered == 0 {
					want := "AI invocation reporting dropped: rejected (HTTP " + strconv.Itoa(tc.status) + ")"
					if !strings.Contains(logs.String(), want) || strings.Count(logs.String(), "AI invocation reporting dropped:") != 1 {
						t.Fatalf("rejection log=%q", logs.String())
					}
				} else if logs.Len() != 0 {
					t.Fatalf("successful retry logged a drop: %q", logs.String())
				}
				if strings.Contains(logs.String(), "secret") {
					t.Fatal("reporting leaked a bearer, body or transport error into logs")
				}
			})
		})
	}
}

func TestRecorderQueueLimitsAndDrain(t *testing.T) {
	for _, tc := range []struct {
		name    string
		pending int
		prefix  string
		reason  dropReason
	}{
		{"items", invocationQueueCapacity, "model-", dropQueueFull},
		{"bytes", 1, strings.Repeat("x", invocationQueueBytes/3), dropQueueBytes},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				release := make(chan struct{})
				var models, bearers []string
				p := testRecorder(t, recorderTransport(func(r *http.Request) (*http.Response, error) {
					var record aipolicy.InvocationRecord
					if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
						t.Error(err)
					}
					models = append(models, record.Model)
					bearers = append(bearers, r.Header.Get("Authorization"))
					select {
					case <-release:
						return recorderResponse(http.StatusAccepted), nil
					case <-r.Context().Done():
						return nil, r.Context().Err()
					}
				}))
				p.enqueue("Bearer caller-0", aipolicy.InvocationRecord{Model: tc.prefix + "0"})
				synctest.Wait() // Keep the first record in flight while filling the queue.
				for i := 1; i <= tc.pending; i++ {
					suffix := strconv.Itoa(i)
					p.enqueue("Bearer caller-"+suffix, aipolicy.InvocationRecord{Model: tc.prefix + suffix})
				}
				p.enqueue("Bearer newest", aipolicy.InvocationRecord{Model: tc.prefix + "newest"})
				if p.DroppedCount(tc.reason) != 1 || p.DeliveredCount() != 0 {
					t.Fatalf("dropped=%d delivered=%d", p.DroppedCount(tc.reason), p.DeliveredCount())
				}
				if p.queuedBytes.Load() > invocationQueueBytes {
					t.Fatal("queue exceeded its byte cap")
				}
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				closed := make(chan error, 1)
				go func() { closed <- p.Close(ctx) }()
				synctest.Wait()
				select {
				case err := <-closed:
					t.Fatalf("Close returned before pending delivery: %v", err)
				default:
				}
				p.enqueue("Bearer after-close", aipolicy.InvocationRecord{})
				if p.DroppedCount(dropShutdown) != 1 {
					t.Fatal("Close did not stop accepting records before draining")
				}
				close(release)
				if err := <-closed; err != nil {
					t.Fatalf("Close failed to drain pending records: %v", err)
				}
				if len(models) != tc.pending+1 || p.DeliveredCount() != uint64(tc.pending+1) || p.queuedBytes.Load() != 0 {
					t.Fatalf("requests=%d delivered=%d queued bytes=%d", len(models), p.DeliveredCount(), p.queuedBytes.Load())
				}
				for i, model := range models {
					suffix := strconv.Itoa(i)
					if model != tc.prefix+suffix || bearers[i] != "Bearer caller-"+suffix {
						t.Fatalf("FIFO body/bearer mismatch at record %d", i)
					}
				}
			})
		})
	}
}

func TestRecorderDeadlineIncludesQueueTime(t *testing.T) {
	for _, queued := range []bool{false, true} {
		t.Run("queued="+strconv.FormatBool(queued), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				started := time.Now()
				var attempts []time.Duration
				p := testRecorder(t, recorderTransport(func(r *http.Request) (*http.Response, error) {
					var record aipolicy.InvocationRecord
					if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
						t.Error(err)
					}
					switch record.Model {
					case "retry":
						attempts = append(attempts, time.Since(started))
						return recorderResponse(http.StatusServiceUnavailable), nil
					case "queued":
						t.Error("expired queued record reached the receiver")
					case "fresh":
						if time.Since(started) != invocationRecordTTL {
							t.Error("expired record blocked a later record beyond its deadline")
						}
					}
					return recorderResponse(http.StatusAccepted), nil
				}))
				p.enqueue("Bearer caller", aipolicy.InvocationRecord{Model: "retry"})
				if queued {
					p.enqueue("Bearer caller", aipolicy.InvocationRecord{Model: "queued"})
				}
				synctest.Wait()
				// synctest advances virtual time, not wall-clock time.
				time.Sleep(invocationRecordTTL - time.Second)
				p.enqueue("Bearer later-caller", aipolicy.InvocationRecord{Model: "fresh"})
				closeRecorder(t, p)
				wantExpired := uint64(1)
				if queued {
					wantExpired++
				}
				if p.DroppedCount(dropExpired) != wantExpired || p.DeliveredCount() != 1 || p.queuedBytes.Load() != 0 {
					t.Fatalf("expired=%d delivered=%d queued bytes=%d", p.DroppedCount(dropExpired), p.DeliveredCount(), p.queuedBytes.Load())
				}
				if len(attempts) < 8 || attempts[0] != 0 {
					t.Fatalf("retry attempts=%v", attempts)
				}
				backoff := 100 * time.Millisecond
				for i := 1; i < len(attempts); i++ {
					if attempts[i]-attempts[i-1] != backoff || attempts[i] >= invocationRecordTTL {
						t.Fatalf("unexpected retry schedule: %v", attempts)
					}
					backoff = min(backoff*2, 5*time.Second)
				}
			})
		})
	}
}

func TestRecorderShutdownCancellation(t *testing.T) {
	for _, parentCanceled := range []bool{false, true} {
		t.Run("parent="+strconv.FormatBool(parentCanceled), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				parent, cancelParent := context.WithCancel(context.Background())
				defer cancelParent()
				attempts := 0
				p := newRecorder(parent, "http://platform.test", "pj-test", recorderTransport(func(r *http.Request) (*http.Response, error) {
					attempts++
					<-r.Context().Done()
					return nil, r.Context().Err()
				}))
				p.enqueue("Bearer caller", aipolicy.InvocationRecord{})
				synctest.Wait()
				p.enqueue("Bearer queued", aipolicy.InvocationRecord{})
				ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
				defer cancel()
				if parentCanceled {
					cancelParent()
				}
				err := p.Close(ctx)
				if parentCanceled && err != nil || !parentCanceled && !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("close error=%v", err)
				}
				synctest.Wait()
				select {
				case <-p.done:
				default:
					t.Fatal("delivery worker did not stop after cancellation")
				}
				if attempts != 1 || p.DeliveredCount() != 0 || p.DroppedCount(dropShutdown) != 2 || p.queuedBytes.Load() != 0 {
					t.Fatalf("attempts=%d delivered=%d shutdown=%d queued bytes=%d", attempts, p.DeliveredCount(), p.DroppedCount(dropShutdown), p.queuedBytes.Load())
				}
				p.enqueue("Bearer after-close", aipolicy.InvocationRecord{})
				if p.DroppedCount(dropShutdown) != 3 {
					t.Fatal("closed recorder accepted a record")
				}
				closeRecorder(t, p) // Repeated Close is harmless.
			})
		})
	}
}

func TestNilRecorder(t *testing.T) {
	var p *recorder
	p.enqueue("Bearer caller", aipolicy.InvocationRecord{})
	closeRecorder(t, p)
	if p.DeliveredCount() != 0 || p.DroppedCount(dropQueueFull) != 0 {
		t.Fatal("nil recorder has nonzero counters")
	}
}
