package intelligence

import (
	"context"
	"testing"
	"time"

	"github.com/syntrix/runtime/internal/engine"
)

func TestPythonPredictorSync(t *testing.T) {
	pred := NewPythonPredictor("")
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	snap := engine.Snapshot{
		TimestampMs: time.Now().UnixMilli(),
		Memory:      engine.MemoryMetrics{PressurePct: 62.5},
		Disk:        engine.DiskMetrics{UsedPct: 48.0},
		TopProcesses: []engine.ProcessInfo{
			{PID: 1234, Name: "RunawayTask", CPUPct: 92.0, MemoryBytes: 2500 * 1024 * 1024, Threads: 16},
		},
	}

	fc, err := pred.EvaluateSync(ctx, snap)
	if err != nil {
		t.Fatalf("PythonPredictor.EvaluateSync failed: %v", err)
	}
	if fc.ModelBackend == "" {
		t.Fatalf("expected non-empty model_backend")
	}
	if len(fc.AbnormalApps) == 0 || fc.AbnormalApps[0].Name != "RunawayTask" {
		t.Fatalf("expected RunawayTask flagged as abnormal app, got %+v", fc.AbnormalApps)
	}
}
