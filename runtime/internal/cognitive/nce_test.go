package cognitive

import (
	"testing"

	"github.com/syntrix/runtime/internal/engine"
	"github.com/syntrix/runtime/internal/governor"
)

func TestNeuralCognitiveEngineFusionMemoryAndGovernor(t *testing.T) {
	nce := NewNeuralCognitiveEngine()
	snap := engine.Snapshot{
		TimestampMs: 1_700_000_000_000,
		CPU:         engine.CPUMetrics{TotalUsagePct: 82.0},
		Memory: engine.MemoryMetrics{
			PressurePct:   84.0,
			SwapUsedBytes: 1024 * 1024 * 1024,
			UsedBytes:     6 << 30,
			TotalBytes:    8 << 30,
		},
		TopProcesses: []engine.ProcessInfo{
			{PID: 4242, Name: "BrowserHelper", CPUPct: 45.0, MemoryBytes: 1800 * 1024 * 1024},
		},
	}

	stNormal := nce.Evaluate(snap, governor.Status{State: governor.StateNormal, AllowInference: true}, "Safari")
	if stNormal.SchedulingTier != TierNormal || !stNormal.AllowNeuralML {
		t.Fatalf("expected NORMAL tier with ML enabled, got %+v", stNormal)
	}
	if stNormal.Observation.ContentionScore < 0.65 || stNormal.GraphEdgeCount < 2 || stNormal.EpisodicLen != 1 {
		t.Fatalf("unexpected NCE observation/graph/episodic state: %+v", stNormal)
	}

	nce.RecordOutcome("trim_daemon_heap", true, 84.0, 78.0)
	nce.RecordOutcome("nice_background_cpu", false, 84.0, 84.0)

	stCrit := nce.Evaluate(snap, governor.Status{State: governor.StateCritical, AllowInference: false}, "Safari")
	if stCrit.SchedulingTier != TierCritical || stCrit.AllowNeuralML {
		t.Fatalf("expected CRITICAL tier to suspend neural ML, got %+v", stCrit)
	}
	if len(stCrit.TopActions) != 2 || stCrit.TopActions[0].Action != "trim_daemon_heap" {
		t.Fatalf("expected trim_daemon_heap ranked first, got %+v", stCrit.TopActions)
	}
}
