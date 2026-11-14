package workflow_test

import (
	"testing"

	"github.com/syntrix/runtime/internal/engine"
	"github.com/syntrix/runtime/internal/events"
	"github.com/syntrix/runtime/internal/intelligence"
	"github.com/syntrix/runtime/internal/store"
	"github.com/syntrix/runtime/internal/tools"
	"github.com/syntrix/runtime/internal/workflow"
)

func TestConditionalWorkflowApprovalAndStorageForecast(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()

	bus := events.NewBus()
	reg := tools.NewRegistry()
	reg.Register(tools.Tool{
		Name: "ok.tool",
		Risk: tools.RiskReadOnly,
		Handler: func(args map[string]string) (string, error) {
			return "ok-result", nil
		},
	})

	wfEng := workflow.NewEngine(bus, reg, st)
	def := workflow.Definition{
		ID:   "wf-test-1",
		Name: "Conditional Checkpoint Flow",
		Steps: []workflow.Step{
			{Name: "Always Step", Tool: "ok.tool", Condition: "always"},
			{Name: "Skipped Step", Tool: "ok.tool", Condition: "if_cpu_gt:95"},
			{Name: "Gated Step", Tool: "ok.tool", Condition: "if_mem_gt:50", RequireApproval: true, MaxRetries: 1},
		},
	}

	snap := engine.Snapshot{
		TimestampMs: 1000,
		CPU:         engine.CPUMetrics{TotalUsagePct: 20},
		Memory:      engine.MemoryMetrics{UsedPct: 75},
		Disk:        engine.DiskMetrics{UsedPct: 60},
	}

	// First run pauses at Gated Step
	run1 := wfEng.Execute(def, snap, 0, false)
	if run1.Status != "awaiting_approval" || run1.CurrentStep != 2 {
		t.Fatalf("expected awaiting_approval at step 2, got status=%s step=%d", run1.Status, run1.CurrentStep)
	}

	// Resume with approval completes
	run2 := wfEng.Execute(def, snap, run1.CurrentStep, true)
	if run2.Status != "completed" {
		t.Fatalf("expected completed, got %s (%s)", run2.Status, run2.Output)
	}

	runs, err := st.ListWorkflowRuns(5)
	if err != nil || len(runs) == 0 || runs[0].Status != "completed" {
		t.Fatalf("unexpected persisted workflow runs: %+v err=%v", runs, err)
	}

	// Storage forecast check
	for i := 0; i < 5; i++ {
		snap.TimestampMs += 86400000
		snap.Disk.UsedPct += 1.0
		_ = st.InsertSnapshot(snap)
	}
	fc := intelligence.ForecastStorageGrowth(st, 60.0)
	if fc.GrowthPctPerDay <= 0.5 || fc.DaysUntil90Pct <= 0 {
		t.Fatalf("unexpected storage forecast: %+v", fc)
	}
}
