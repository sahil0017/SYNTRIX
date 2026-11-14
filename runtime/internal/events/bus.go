package events

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type EventType string

const (
	UserRequestSubmitted          EventType = "user.request.submitted"
	TelemetrySnapshotRequested    EventType = "telemetry.snapshot.requested"
	TelemetrySnapshotCollected    EventType = "telemetry.snapshot.collected"
	AnalysisRequested             EventType = "analysis.requested"
	RecommendationProposed        EventType = "recommendation.proposed"
	PolicyValidationRequested     EventType = "policy.validation.requested"
	PolicyApprovalRequired        EventType = "policy.approval.required"
	RecommendationPresented       EventType = "recommendation.presented"
	ActionApprovalReceived        EventType = "action.approval.received"
	PolicyRevalidationRequested   EventType = "policy.revalidation.requested"
	PolicyAuthorized              EventType = "policy.authorized"
	PolicyDenied                  EventType = "policy.denied"
	ActionExecutionRequested      EventType = "action.execution.requested"
	ActionExecuted                EventType = "action.executed"
	VerificationSnapshotRequested EventType = "verification.snapshot.requested"
	VerificationSnapshotCollected EventType = "verification.snapshot.collected"
	OutcomeMeasured               EventType = "outcome.measured"
)

type Priority int

const (
	PriorityHigh   Priority = 1
	PriorityMedium Priority = 2
	PriorityLow    Priority = 3
)

type Event struct {
	EventID       string    `json:"event_id"`
	Seq           uint64    `json:"seq"`
	SchemaVersion int       `json:"schema_version"`
	Priority      Priority  `json:"priority"`
	DedupKey      string    `json:"dedup_key,omitempty"`
	Type          EventType `json:"type"`
	CorrelationID string    `json:"correlation_id"`
	Source        string    `json:"source"`
	Target        string    `json:"target"`
	TimestampMs   int64     `json:"timestamp_ms"`
	Summary       string    `json:"summary"`
	Payload       any       `json:"-"`
}

type Handler func(Event)
type JournalFunc func(Event)

type Bus struct {
	mu          sync.RWMutex
	subscribers map[EventType][]Handler
	anyHandlers []Handler
	journal     JournalFunc
	seq         atomic.Uint64
	dedupMu     sync.Mutex
	recentDedup map[string]int64
}

func NewBus() *Bus {
	return &Bus{
		subscribers: make(map[EventType][]Handler),
		recentDedup: make(map[string]int64),
	}
}

func (b *Bus) SetJournalHook(fn JournalFunc) {
	b.mu.Lock()
	b.journal = fn
	b.mu.Unlock()
}

func (b *Bus) Subscribe(eventType EventType, h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers[eventType] = append(b.subscribers[eventType], h)
}

func (b *Bus) SubscribeAll(h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.anyHandlers = append(b.anyHandlers, h)
}

func (b *Bus) Publish(ev Event) {
	nowMs := time.Now().UnixMilli()
	if ev.TimestampMs == 0 {
		ev.TimestampMs = nowMs
	}
	if ev.SchemaVersion == 0 {
		ev.SchemaVersion = 2
	}
	if ev.Priority == 0 {
		ev.Priority = PriorityMedium
	}

	if ev.DedupKey != "" {
		b.dedupMu.Lock()
		prevTs, seen := b.recentDedup[ev.DedupKey]
		if seen && (nowMs-prevTs) < 5000 {
			b.dedupMu.Unlock()
			return
		}
		if len(b.recentDedup) > 512 {
			b.recentDedup = make(map[string]int64)
		}
		b.recentDedup[ev.DedupKey] = nowMs
		b.dedupMu.Unlock()
	}

	seq := b.seq.Add(1)
	ev.Seq = seq
	if ev.EventID == "" {
		ev.EventID = fmt.Sprintf("ev-%d-%d", nowMs, seq)
	}

	b.mu.RLock()
	specific := append([]Handler(nil), b.subscribers[ev.Type]...)
	all := append([]Handler(nil), b.anyHandlers...)
	journalFn := b.journal
	b.mu.RUnlock()

	if journalFn != nil && ev.Priority <= PriorityMedium {
		journalFn(ev)
	}
	for _, h := range all {
		h(ev)
	}
	for _, h := range specific {
		h(ev)
	}
}
