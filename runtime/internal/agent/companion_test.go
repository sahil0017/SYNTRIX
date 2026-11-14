package agent_test

import (
	"strings"
	"testing"
	"time"

	"github.com/syntrix/runtime/internal/agent"
	"github.com/syntrix/runtime/internal/engine"
	"github.com/syntrix/runtime/internal/events"
	"github.com/syntrix/runtime/internal/intelligence"
	"github.com/syntrix/runtime/internal/policy"
	"github.com/syntrix/runtime/internal/store"
)

func TestEndToEndRuntimePipeline(t *testing.T) {
	col, err := engine.NewCollector()
	if err != nil {
		t.Fatalf("NewCollector: %v", err)
	}
	defer col.Close()

	_, _ = col.Collect(8)
	time.Sleep(150 * time.Millisecond)
	snap, err := col.Collect(8)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if snap.CPU.CoreCount <= 0 || snap.Memory.TotalBytes == 0 {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}

	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()

	if err := st.InsertSnapshot(snap); err != nil {
		t.Fatalf("InsertSnapshot: %v", err)
	}
	recent, err := st.RecentSnapshots(5)
	if err != nil || len(recent) != 1 {
		t.Fatalf("RecentSnapshots: len=%d err=%v", len(recent), err)
	}

	analyzer := intelligence.NewAnalyzer()
	_ = analyzer.Ingest(snap)

	comp := agent.NewCompanion(st, analyzer)
	comp.UpdateSnapshot(snap)

	reply, tool := comp.HandlePrompt("/status")
	if tool != "tool:get_system_status" || !strings.Contains(reply, "System Status") {
		t.Fatalf("unexpected /status reply (%s): %s", tool, reply)
	}

	reply, _ = comp.HandlePrompt("/mode MONITOR")
	if comp.Mode() != agent.ModeMonitor || !strings.Contains(reply, "MONITOR") {
		t.Fatalf("mode switch failed: %s / %s", comp.Mode(), reply)
	}

	rec := store.Recommendation{
		ID:           "test-rec-1",
		CreatedAtMs:  snap.TimestampMs,
		Kind:         "test",
		Severity:     "low",
		Title:        "Test recommendation",
		Detail:       "Test detail",
		TargetPID:    1,
		TargetName:   "launchd",
		Status:       "pending",
		BeforeMetric: "cpu=10%",
	}
	if err := st.UpsertRecommendation(rec); err != nil {
		t.Fatalf("UpsertRecommendation: %v", err)
	}
	msg, err := comp.ApproveRecommendation("test-rec-1")
	if err != nil || !strings.Contains(msg, "approved") {
		t.Fatalf("ApproveRecommendation: %v %s", err, msg)
	}

	logs, err := st.ListAuditLogs(10)
	if err != nil || len(logs) < 3 {
		t.Fatalf("expected >=3 audit logs, got %d (err=%v)", len(logs), err)
	}
}

func TestEventDrivenMemoryOptimizationFlow(t *testing.T) {
	col, err := engine.NewCollector()
	if err != nil {
		t.Fatalf("NewCollector: %v", err)
	}
	defer col.Close()

	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()

	bus := events.NewBus()
	var seen []events.EventType
	bus.SubscribeAll(func(ev events.Event) {
		seen = append(seen, ev.Type)
	})

	pol := policy.NewEngine(bus)
	analyzer := intelligence.NewAnalyzer()
	comp := agent.NewCompanion(st, analyzer)
	orch := agent.NewOrchestrator(bus, pol, col, st, comp)
	comp.SetOrchestrator(orch)

	reply, tool := comp.HandlePrompt("Analyze my memory usage")
	if tool != "event:memory_optimization" || !strings.Contains(reply, "Memory Analysis Complete") {
		t.Fatalf("unexpected memory analysis reply (%s): %s", tool, reply)
	}

	recs, err := st.ListRecommendations(5)
	if err != nil || len(recs) == 0 {
		t.Fatalf("expected pending recommendation in store, got %d (err=%v)", len(recs), err)
	}
	if recs[0].Status != "pending" {
		t.Fatalf("expected pending status, got %s", recs[0].Status)
	}

	outcome, err := comp.ApproveRecommendation(recs[0].ID)
	if err != nil || !strings.Contains(outcome, "Measured Outcome") {
		t.Fatalf("ApproveRecommendation failed: %v (%s)", err, outcome)
	}

	required := []events.EventType{
		events.UserRequestSubmitted,
		events.TelemetrySnapshotRequested,
		events.TelemetrySnapshotCollected,
		events.AnalysisRequested,
		events.RecommendationProposed,
		events.PolicyValidationRequested,
		events.PolicyApprovalRequired,
		events.RecommendationPresented,
		events.ActionApprovalReceived,
		events.PolicyRevalidationRequested,
		events.PolicyAuthorized,
		events.ActionExecutionRequested,
		events.ActionExecuted,
		events.VerificationSnapshotRequested,
		events.VerificationSnapshotCollected,
		events.OutcomeMeasured,
	}
	for _, reqType := range required {
		found := false
		for _, s := range seen {
			if s == reqType {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing expected event %s in sequence %v", reqType, seen)
		}
	}
}
