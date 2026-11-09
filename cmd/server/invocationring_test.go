package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/appthrust/appserver-go/accesskey"
	"github.com/appthrust/appserver-go/aipolicy"
	"github.com/appthrust/appserver-go/appserver/identity"
	"github.com/stretchr/testify/require"
)

func TestInvocationRingBoundFiltersAndPatchBack(t *testing.T) {
	t.Setenv("HOSTNAME", "instance-test")
	var ring invocationRing
	started := time.Now()
	for i := range 510 {
		caller := identity.Identity{Principal: accesskey.Principal{Subject: fmt.Sprint(i % 2)}, Email: "member@example.test"}
		id := ring.begin(caller, "chat", started.Add(time.Duration(i)*time.Millisecond))
		record := aipolicy.InvocationRecord{Provider: fmt.Sprint(i % 3), Model: fmt.Sprint(i % 5), Outcome: aipolicy.OutcomeAllowed, Status: 200, DurationMs: 12, Usage: &aipolicy.Usage{PromptTokens: 7, CompletionTokens: 8}, Policy: &aipolicy.PolicyRef{Name: "policy", UID: "uid"}, RequestDigest: strings.Repeat("a", 64), CompletionDigest: strings.Repeat("b", 64)}
		if i%2 == 0 {
			record.Outcome = aipolicy.OutcomeDenied
			record.Status = 403
			record.Reason = "ai_model_denied"
		}
		ring.finish(id, record, true)
	}
	page := ring.page(invocationFilter{limit: 1000})
	require.Len(t, page.Items, 500)
	require.Equal(t, "510", page.Items[0].ID)
	require.Equal(t, "11", page.Items[499].ID)
	require.Equal(t, "instance-test", page.Instance)
	require.Equal(t, processStarted, page.Since)
	require.Equal(t, 500, page.Capacity)
	require.Len(t, ring.page(invocationFilter{}).Items, 100)
	ring.report("1", invocationReport{State: "Delivered"}) // Evicted reports must not patch a reused slot.
	require.Equal(t, "Pending", ring.page(invocationFilter{limit: 500}).Items[9].Report.State)
	ring.report("510", invocationReport{State: "Dropped", Reason: "rejected", Status: 403})
	e := ring.page(invocationFilter{limit: 1}).Items[0]
	require.Equal(t, invocationReport{State: "Dropped", Reason: "rejected", Status: 403}, e.Report)
	require.Equal(t, "member@example.test", e.Caller.DisplayName)
	require.EqualValues(t, 7, e.PromptTokens)
	require.EqualValues(t, 8, e.CompletionTokens)
	require.True(t, e.Stream)
	require.Equal(t, &invocationPolicy{Name: "policy", UID: "uid"}, e.Policy)
	for _, filter := range []invocationFilter{{outcome: "Denied"}, {provider: "2"}, {model: "3"}, {mine: true, subject: "1"}, {outcome: "Denied", provider: "2", model: "3", mine: true, subject: "0", limit: 5}} {
		filtered := ring.page(filter)
		require.NotEmpty(t, filtered.Items)
		for _, e := range filtered.Items {
			if filter.outcome != "" {
				require.Equal(t, filter.outcome, string(e.Outcome))
			}
			if filter.provider != "" {
				require.Equal(t, filter.provider, e.Provider)
			}
			if filter.model != "" {
				require.Equal(t, filter.model, e.Model)
			}
			if filter.mine {
				require.Equal(t, filter.subject, e.Caller.Subject)
			}
		}
		if filter.limit != 0 {
			require.LessOrEqual(t, len(filtered.Items), filter.limit)
		}
	}
}

func TestRecorderReportsPerRecordDelivery(t *testing.T) {
	for _, status := range []int{http.StatusAccepted, http.StatusForbidden} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var ring invocationRing
			id := ring.begin(identity.Identity{}, "models", time.Now())
			record := aipolicy.InvocationRecord{Outcome: aipolicy.OutcomeAllowed, Status: 200}
			ring.finish(id, record, false)
			p := testRecorder(t, recorderTransport(func(*http.Request) (*http.Response, error) { return recorderResponse(status), nil }))
			p.enqueue("Bearer caller", record, invocationDelivery{id: id, report: ring.report})
			closeRecorder(t, p)
			result := ring.page(invocationFilter{}).Items[0].Report
			if status == http.StatusAccepted {
				require.Equal(t, invocationReport{State: "Delivered"}, result)
			} else {
				require.Equal(t, invocationReport{State: "Dropped", Reason: "rejected", Status: status}, result)
			}
		})
	}
}

func TestRecorderReportsOverflowAndShutdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		p := testRecorder(t, recorderTransport(func(*http.Request) (*http.Response, error) { <-release; return recorderResponse(202), nil }))
		p.enqueue("Bearer caller", aipolicy.InvocationRecord{})
		synctest.Wait()
		for range invocationQueueCapacity {
			p.enqueue("Bearer caller", aipolicy.InvocationRecord{})
		}
		var ring invocationRing
		id := ring.begin(identity.Identity{}, "chat", time.Now())
		p.enqueue("Bearer caller", aipolicy.InvocationRecord{}, invocationDelivery{id: id, report: ring.report})
		require.Equal(t, invocationReport{State: "Dropped", Reason: "queue_full"}, ring.page(invocationFilter{}).Items[0].Report)
		close(release)
		closeRecorder(t, p)
		id = ring.begin(identity.Identity{}, "chat", time.Now())
		p.enqueue("Bearer caller", aipolicy.InvocationRecord{}, invocationDelivery{id: id, report: ring.report})
		require.Equal(t, invocationReport{State: "Dropped", Reason: "shutdown"}, ring.page(invocationFilter{}).Items[0].Report)
	})
}

func TestRecorderReportsExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var ring invocationRing
		id := ring.begin(identity.Identity{}, "chat", time.Now())
		p := testRecorder(t, recorderTransport(func(*http.Request) (*http.Response, error) { return recorderResponse(503), nil }))
		p.enqueue("Bearer caller", aipolicy.InvocationRecord{}, invocationDelivery{id: id, report: ring.report})
		ctx, cancel := context.WithTimeout(context.Background(), invocationRecordTTL+time.Second)
		defer cancel()
		require.NoError(t, p.Close(ctx))
		require.Equal(t, invocationReport{State: "Dropped", Reason: "expired"}, ring.page(invocationFilter{}).Items[0].Report)
	})
}

func TestInvocationCallerFilterUsesFullSubject(t *testing.T) {
	var ring invocationRing
	prefix := strings.Repeat("x", 128)
	for _, suffix := range []string{"first", "second"} {
		ring.begin(identity.Identity{Principal: accesskey.Principal{Subject: prefix + suffix}}, "models", time.Now())
	}
	page := ring.page(invocationFilter{mine: true, subject: prefix + "first"})
	require.Len(t, page.Items, 1)
	require.Equal(t, "1", page.Items[0].ID)
}
