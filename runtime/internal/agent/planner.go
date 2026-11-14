package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/syntrix/runtime/internal/tools"
)

type PlanStep struct {
	ID              string            `json:"id"`
	Title           string            `json:"title"`
	ToolName        string            `json:"tool_name"`
	Args            map[string]string `json:"args,omitempty"`
	RequiresApprove bool              `json:"requires_approval"`
	Completed       bool              `json:"completed"`
	Output          string            `json:"output,omitempty"`
}

type TaskPlan struct {
	Goal     string     `json:"goal"`
	Risk     string     `json:"risk"`
	Scope    string     `json:"scope"`
	Steps    []PlanStep `json:"steps"`
	Status   string     `json:"status"`
	Checkpt  int        `json:"checkpoint"`
}

type TaskPlanner struct {
	registry *tools.Registry
}

func NewTaskPlanner(reg *tools.Registry) *TaskPlanner {
	return &TaskPlanner{registry: reg}
}

func ClassifyIntentAndRisk(prompt string) (intent string, risk string) {
	lower := strings.ToLower(prompt)
	switch {
	case strings.Contains(lower, "patch") || strings.Contains(lower, "fix") || strings.Contains(lower, "delete") || strings.Contains(lower, "clean"):
		return "modify_or_remediate", "approval_required"
	case strings.Contains(lower, "test") || strings.Contains(lower, "inspect") || strings.Contains(lower, "scan"):
		return "inspect_and_diagnose", "read_only"
	default:
		return "assist", "read_only"
	}
}

func (p *TaskPlanner) BuildPlan(goal, scope string) TaskPlan {
	_, risk := ClassifyIntentAndRisk(goal)
	steps := []PlanStep{
		{ID: "step-1", Title: "Build project & telemetry context", ToolName: "system.status", RequiresApprove: false},
		{ID: "step-2", Title: "Inspect processes & workspace diagnostics", ToolName: "system.top_processes", RequiresApprove: false},
	}
	if risk == "approval_required" {
		steps = append(steps, PlanStep{
			ID:              "step-3",
			Title:           "Propose bounded remediation (awaiting human approval)",
			ToolName:        "optimization.scan",
			RequiresApprove: true,
		})
	}
	return TaskPlan{
		Goal:   goal,
		Risk:   risk,
		Scope:  scope,
		Steps:  steps,
		Status: "planned",
	}
}

func (p *TaskPlanner) ExecutePlan(ctx context.Context, plan TaskPlan, approved bool) TaskPlan {
	for i := plan.Checkpt; i < len(plan.Steps); i++ {
		if ctx.Err() != nil {
			plan.Status = "cancelled"
			return plan
		}
		step := &plan.Steps[i]
		if step.Completed {
			plan.Checkpt = i + 1
			continue
		}
		if step.RequiresApprove && !approved {
			plan.Status = "awaiting_approval"
			return plan
		}
		if p.registry == nil {
			step.Output = "tool registry unavailable"
			plan.Status = "failed"
			return plan
		}
		out, _, err := p.registry.ExecuteInScope(plan.Scope, step.ToolName, step.Args)
		if err != nil {
			step.Output = fmt.Sprintf("error: %v", err)
			plan.Status = "failed"
			return plan
		}
		step.Output = out
		step.Completed = true
		plan.Checkpt = i + 1
	}
	plan.Status = "completed"
	return plan
}
