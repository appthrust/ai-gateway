package main

import (
	"crypto/sha256"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/appthrust/appserver-go/aipolicy"
	"github.com/appthrust/appserver-go/appserver/identity"
)

const invocationRingCapacity = 500

var processStarted = time.Now().UTC()

type invocationReport struct {
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
	Status int    `json:"status,omitempty"`
}

type invocationCaller struct {
	Subject     string `json:"subject"`
	DisplayName string `json:"displayName"`
}

type invocationPolicy struct {
	Name string `json:"name"`
	UID  string `json:"uid"`
}

type InvocationEntry struct {
	ID               string            `json:"id"`
	Time             time.Time         `json:"time"`
	Route            string            `json:"route"`
	Caller           invocationCaller  `json:"caller"`
	Provider         string            `json:"provider"`
	Model            string            `json:"model"`
	Outcome          aipolicy.Outcome  `json:"outcome"`
	Status           int               `json:"status"`
	Reason           string            `json:"reason"`
	DurationMs       int64             `json:"durationMs"`
	PromptTokens     int64             `json:"promptTokens"`
	CompletionTokens int64             `json:"completionTokens"`
	RequestDigest    string            `json:"requestDigest,omitempty"`
	CompletionDigest string            `json:"completionDigest,omitempty"`
	Policy           *invocationPolicy `json:"policy"`
	Stream           bool              `json:"stream"`
	Report           invocationReport  `json:"report"`
}

type invocationPage struct {
	Since    time.Time         `json:"since"`
	Instance string            `json:"instance"`
	Capacity int               `json:"capacity"`
	Items    []InvocationEntry `json:"items"`
}

type invocationFilter struct {
	outcome, provider, model, subject string
	mine                              bool
	limit                             int
}

type invocationRing struct {
	mu       sync.Mutex
	entries  [invocationRingCapacity]InvocationEntry
	subjects [invocationRingCapacity][sha256.Size]byte
	next     uint64
	count    int
}

// Only bounded metadata is retained; request bodies and caller bearers never enter the ring.
func boundedMetadata(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	for limit > 0 && !utf8.RuneStart(s[limit]) {
		limit--
	}
	return strings.Clone(s[:limit])
}

func (r *invocationRing) begin(caller identity.Identity, route string, started time.Time) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.next++
	id := strconv.FormatUint(r.next, 10)
	name := caller.Name
	if name == "" {
		name = caller.Email
	}
	r.entries[(r.next-1)%invocationRingCapacity] = InvocationEntry{
		ID: id, Time: started.UTC(), Route: route, Outcome: aipolicy.OutcomeAllowed, Status: http.StatusOK,
		Caller: invocationCaller{Subject: boundedMetadata(caller.Principal.Subject, 128), DisplayName: boundedMetadata(name, 128)},
		Report: invocationReport{State: "Pending"},
	}
	r.subjects[(r.next-1)%invocationRingCapacity] = sha256.Sum256([]byte(caller.Principal.Subject))
	r.count = min(r.count+1, invocationRingCapacity)
	return id
}

func (r *invocationRing) entry(id string) *InvocationEntry {
	n, err := strconv.ParseUint(id, 10, 64)
	if err != nil || n == 0 {
		return nil
	}
	e := &r.entries[(n-1)%invocationRingCapacity]
	if e.ID != id {
		return nil
	}
	return e
}

func (r *invocationRing) finish(id string, record aipolicy.InvocationRecord, stream bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.entry(id)
	if e == nil {
		return
	}
	e.Provider, e.Model = boundedMetadata(record.Provider, 64), boundedMetadata(record.Model, 128)
	e.Outcome, e.Status, e.Reason = record.Outcome, record.Status, boundedMetadata(record.Reason, 64)
	e.DurationMs, e.Stream = record.DurationMs, stream
	e.RequestDigest, e.CompletionDigest = record.RequestDigest, record.CompletionDigest
	if record.Usage != nil {
		e.PromptTokens, e.CompletionTokens = record.Usage.PromptTokens, record.Usage.CompletionTokens
	}
	if record.Policy != nil {
		e.Policy = &invocationPolicy{Name: boundedMetadata(record.Policy.Name, 64), UID: boundedMetadata(record.Policy.UID, 64)}
	}
}

func (r *invocationRing) report(id string, result invocationReport) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e := r.entry(id); e != nil {
		e.Report = result
	}
}

func (r *invocationRing) page(filter invocationFilter) invocationPage {
	r.mu.Lock()
	defer r.mu.Unlock()
	subject := sha256.Sum256([]byte(filter.subject))
	limit := filter.limit
	if limit <= 0 {
		limit = 100
	}
	limit = min(limit, invocationRingCapacity)
	page := invocationPage{Since: processStarted, Instance: os.Getenv("HOSTNAME"), Capacity: invocationRingCapacity, Items: make([]InvocationEntry, 0, min(limit, r.count))}
	for i := 0; i < r.count && len(page.Items) < limit; i++ {
		index := (r.next - 1 - uint64(i)) % invocationRingCapacity
		e := r.entries[index]
		if filter.outcome != "" && string(e.Outcome) != filter.outcome || filter.provider != "" && e.Provider != filter.provider || filter.model != "" && e.Model != filter.model || filter.mine && r.subjects[index] != subject {
			continue
		}
		page.Items = append(page.Items, e)
	}
	return page
}

func (g *gateway) invocations(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	limit, _ := strconv.Atoi(query.Get("limit"))
	caller, _ := r.Context().Value(callerContextKey{}).(identity.Identity)
	writeJSON(w, http.StatusOK, g.invocationsRing.page(invocationFilter{
		outcome: query.Get("outcome"), provider: query.Get("provider"), model: query.Get("model"),
		mine: query.Get("caller") == "me", subject: caller.Principal.Subject, limit: limit,
	}))
}
