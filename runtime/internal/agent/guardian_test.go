package agent

import (
	"path/filepath"
	"testing"

	"github.com/syntrix/runtime/internal/cognitive"
	"github.com/syntrix/runtime/internal/engine"
	"github.com/syntrix/runtime/internal/events"
	"github.com/syntrix/runtime/internal/intelligence"
	"github.com/syntrix/runtime/internal/policy"
	"github.com/syntrix/runtime/internal/store"
)

func TestGuardianAgentAbsenceAndSafety(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "guardian.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	bus := events.NewBus()
	comp := NewCompanion(st, nil)
	orch := NewOrchestrator(bus, policy.NewEngine(bus), nil, st, comp)
	comp.SetOrchestrator(orch)

	guard := NewGuardianAgent(orch, st)
	guard.Configure(true, 180)
	guard.SetIdleProvider(func() int { return 0 }) // user is present

	snap := engine.Snapshot{
		TimestampMs: 1000,
		CPU:         engine.CPUMetrics{TotalUsagePct: 88},
		Memory:      engine.MemoryMetrics{PressurePct: 82, UsedPct: 85, UsedBytes: 6 << 30, TotalBytes: 8 << 30},
	}
	comp.UpdateSnapshot(snap)

	recRev := store.Recommendation{
		ID:          "rec-mem-1",
		CreatedAtMs: 1000,
		Kind:        "memory_pressure",
		Severity:    "high",
		Title:       "Memory optimization",
		Detail:      "runtime trim",
		Status:      "pending",
	}
	recIrr := store.Recommendation{
		ID:          "rec-cache-1",
		CreatedAtMs: 1000,
		Kind:        "storage_pressure",
		Severity:    "medium",
		Title:       "Stale caches",
		Detail:      "cleanup",
		Status:      "pending",
	}
	_ = st.UpsertRecommendation(recRev)
	_ = st.UpsertRecommendation(recIrr)

	// 1. ModeOff -> should NOT act
	if _, _, acted := guard.Evaluate(ModeOff, snap, []store.Recommendation{recIrr, recRev}); acted {
		t.Fatalf("expected guardian not to act in ModeOff")
	}

	// 2. User active (idle=5s) in ASSIST mode -> should STILL monitor & act safely, skipping irreversible cache delete below 50% disk
	id, _, acted := guard.Evaluate(ModeAssist, snap, []store.Recommendation{recIrr, recRev})
	if !acted || id != "rec-mem-1" {
		t.Fatalf("expected guardian to fulfill rec-mem-1 even while user is active, got acted=%v id=%s", acted, id)
	}
	stStatus := guard.Status()
	if stStatus.ActionsTaken != 1 || stStatus.SkippedUnsafe < 1 {
		t.Fatalf("unexpected guardian status: %+v", stStatus)
	}

	// 3. Kill switch active -> must pause immediately
	orch.ActivateKillSwitch()
	if _, _, acted := guard.Evaluate(ModeAutopilot, snap, []store.Recommendation{recRev}); acted {
		t.Fatalf("expected guardian to pause when kill switch is active")
	}
}

func TestGuardianProactiveForecastAndRLFeedback(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "guardian_rl.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	bus := events.NewBus()
	comp := NewCompanion(st, nil)
	orch := NewOrchestrator(bus, policy.NewEngine(bus), nil, st, comp)
	comp.SetOrchestrator(orch)

	nce := cognitive.NewNeuralCognitiveEngine()
	guard := NewGuardianAgent(orch, st)
	guard.SetCognitiveEngine(nce)
	guard.SetIdleProvider(func() int { return 300 })

	// Current metrics are BELOW threshold (mem=62%, cpu=45%), but Transformer+RL predicts mem=82%
	snap := engine.Snapshot{
		TimestampMs: 2000,
		CPU:         engine.CPUMetrics{TotalUsagePct: 45},
		Memory:      engine.MemoryMetrics{PressurePct: 62, UsedPct: 65, UsedBytes: 5 << 30, TotalBytes: 8 << 30},
	}
	comp.UpdateSnapshot(snap)

	fc := &intelligence.PythonForecast{
		MemoryForecast: intelligence.MemoryForecast{Predicted5mPct: 78.0, Predicted15mPct: 83.0},
		TransformerRL: &intelligence.TransformerRLInfo{
			PredCPUPct:    50.0,
			PredMemPct:    82.0,
			OptimalAction: "trim_daemon_heap",
			QValue:        1.45,
		},
	}

	id, _, acted := guard.EvaluateWithForecast(ModeAssist, snap, nil, fc)
	if !acted || id == "" {
		t.Fatalf("expected proactive RL action before threshold breach, got acted=%v id=%s", acted, id)
	}
	if guard.Status().ProactivePrevented != 1 {
		t.Fatalf("expected ProactivePrevented=1, got %+v", guard.Status())
	}

	// Next snapshot stays safe -> online RL reward should be recorded (> 0)
	snapAfter := snap
	snapAfter.TimestampMs = 4000
	snapAfter.Memory.PressurePct = 59.0
	guard.EvaluateWithForecast(ModeAssist, snapAfter, nil, nil)
	if guard.Status().RLAvgReward <= 0 {
		t.Fatalf("expected positive online RL reward after preventing breach, got %+v", guard.Status())
	}
}
