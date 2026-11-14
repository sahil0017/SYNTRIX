package agent

import (
	"context"
	"testing"

	"github.com/syntrix/runtime/internal/tools"
)

func TestTaskPlannerApprovalCheckpointAndCancellation(t *testing.T) {
	reg := tools.NewRegistry()
	calls := 0
	reg.Register(tools.Tool{
		Name:       "system.status",
		Risk:       tools.RiskReadOnly,
		Capability: tools.CapSystemMetricsRead,
		Handler: func(args map[string]string) (string, error) {
			calls++
			return "cpu=12%", nil
		},
	})
	reg.Register(tools.Tool{
		Name:       "system.top_processes",
		Risk:       tools.RiskReadOnly,
		Capability: tools.CapProcessMetricsRead,
		Handler: func(args map[string]string) (string, error) {
			calls++
			return "top=clang", nil
		},
	})
	reg.Register(tools.Tool{
		Name:       "optimization.scan",
		Risk:       tools.RiskLow,
		Capability: tools.CapOptimizationRequest,
		Handler: func(args map[string]string) (string, error) {
			calls++
			return "proposed patch", nil
		},
	})

	planner := NewTaskPlanner(reg)
	plan := planner.BuildPlan("Inspect my current project, identify failing tests, and propose a patch", "session-1")

	// Without approval, executes steps 1-2 and pauses at step 3
	paused := planner.ExecutePlan(context.Background(), plan, false)
	if paused.Status != "awaiting_approval" || paused.Checkpt != 2 {
		t.Fatalf("expected awaiting_approval at checkpoint 2, got status=%s checkpt=%d", paused.Status, paused.Checkpt)
	}

	// Resuming with approval should NOT re-run steps 1-2
	done := planner.ExecutePlan(context.Background(), paused, true)
	if done.Status != "completed" || calls != 3 {
		t.Fatalf("expected completed with 3 total tool calls, got status=%s calls=%d", done.Status, calls)
	}

	// Cancellation stops immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelled := planner.ExecutePlan(ctx, planner.BuildPlan("Inspect project", "session-2"), true)
	if cancelled.Status != "cancelled" {
		t.Fatalf("expected cancelled status, got %s", cancelled.Status)
	}
}
