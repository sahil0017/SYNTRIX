package governor_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/syntrix/runtime/internal/engine"
	"github.com/syntrix/runtime/internal/events"
	"github.com/syntrix/runtime/internal/governor"
	"github.com/syntrix/runtime/internal/intelligence"
	"github.com/syntrix/runtime/internal/store"
	"github.com/syntrix/runtime/internal/tools"
)

func TestGovernorHysteresisTransitions(t *testing.T) {
	gov := governor.New()

	// Single spike should not escalate immediately (requires 2 consecutive samples).
	st := gov.Evaluate(engine.Snapshot{Memory: engine.MemoryMetrics{PressurePct: 92}})
	if st.State != governor.StateNormal {
		t.Fatalf("expected hysteresis to hold NORMAL on 1st spike, got %s", st.State)
	}

	// Second consecutive critical sample escalates to CRITICAL.
	st = gov.Evaluate(engine.Snapshot{Memory: engine.MemoryMetrics{PressurePct: 92}})
	if st.State != governor.StateCritical {
		t.Fatalf("expected CRITICAL on 2nd spike, got %s", st.State)
	}
	if st.AllowInference || st.AllowIndexing {
		t.Fatalf("expected inference and indexing disabled in CRITICAL")
	}

	// Recovery requires 3 consecutive low-pressure samples.
	for i := 0; i < 2; i++ {
		st = gov.Evaluate(engine.Snapshot{Memory: engine.MemoryMetrics{PressurePct: 30}})
		if st.State != governor.StateCritical {
			t.Fatalf("expected recovery hysteresis to hold CRITICAL at sample %d, got %s", i+1, st.State)
		}
	}
	st = gov.Evaluate(engine.Snapshot{Memory: engine.MemoryMetrics{PressurePct: 30}})
	if st.State != governor.StateNormal {
		t.Fatalf("expected NORMAL after 3 consecutive low-pressure samples, got %s", st.State)
	}
	if gov.SamplingInterval(2*time.Second) != 2*time.Second {
		t.Fatalf("expected base sampling interval in NORMAL")
	}
}

func TestPredictiveEngineMultiSignalCorrelation(t *testing.T) {
	pred := intelligence.NewPredictiveEngine()
	for i := 0; i < 5; i++ {
		pred.Evaluate(engine.Snapshot{
			CPU:    engine.CPUMetrics{TotalUsagePct: 20},
			Memory: engine.MemoryMetrics{PressurePct: 45, PressureLevel: "normal"},
		})
	}

	diag := pred.Evaluate(engine.Snapshot{
		CPU: engine.CPUMetrics{TotalUsagePct: 92},
		Memory: engine.MemoryMetrics{
			PressurePct:     82,
			PressureLevel:   "warn",
			CompressedBytes: 1024 * 1024 * 1024,
			SwapUsedBytes:   512 * 1024 * 1024,
		},
		Thermal: engine.ThermalMetrics{
			IsThrottled:      true,
			StateLabel:       "serious",
			CPUSpeedLimitPct: 70,
		},
		TopProcesses: []engine.ProcessInfo{
			{PID: 4242, StartTimeUs: 123456, Name: "clang", CPUPct: 85, MemoryBytes: 900 * 1024 * 1024, VirtualBytes: 2048 * 1024 * 1024, Threads: 8},
		},
	})

	if diag.Confidence < 0.85 {
		t.Fatalf("expected high confidence (>=0.85) on correlated signals, got %.2f", diag.Confidence)
	}
	if len(diag.Evidence) < 3 {
		t.Fatalf("expected >=3 correlated evidence items, got %d (%v)", len(diag.Evidence), diag.Evidence)
	}
	if len(diag.HealthTree) < 4 {
		t.Fatalf("expected populated health tree, got %v", diag.HealthTree)
	}
}

func TestEventBusDedupAndJournal(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "syntrix_test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()

	bus := events.NewBus()
	journaled := 0
	bus.SetJournalHook(func(ev events.Event) {
		journaled++
		if err := st.InsertEventJournal(ev.Seq, ev.EventID, ev.SchemaVersion, int(ev.Priority), string(ev.Type), ev.CorrelationID, ev.Source, ev.Target, ev.Summary, ev.TimestampMs); err != nil {
			t.Errorf("InsertEventJournal: %v", err)
		}
	})

	ev := events.Event{
		Type:     events.TelemetrySnapshotCollected,
		Source:   "test",
		DedupKey: "snap-1",
		Summary:  "first",
	}
	bus.Publish(ev)
	bus.Publish(ev) // duplicate within 5s window should be dropped

	if journaled != 1 {
		t.Fatalf("expected 1 journaled event after dedup, got %d", journaled)
	}
}

func TestToolsCapabilityRevocation(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(tools.Tool{
		Name:       "inspect_proc",
		Risk:       tools.RiskReadOnly,
		Capability: tools.CapProcessMetricsRead,
		Handler: func(args map[string]string) (string, error) {
			return "ok", nil
		},
	})

	if res, _, err := reg.Execute("inspect_proc", nil); err != nil || res != "ok" {
		t.Fatalf("expected granted capability to succeed, got res=%q err=%v", res, err)
	}

	reg.SetCapability(tools.CapProcessMetricsRead, false)
	if _, _, err := reg.Execute("inspect_proc", nil); err == nil {
		t.Fatalf("expected revoked capability to fail")
	}
}
