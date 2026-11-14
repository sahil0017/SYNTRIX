package workflow

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/syntrix/runtime/internal/engine"
	"github.com/syntrix/runtime/internal/events"
	"github.com/syntrix/runtime/internal/store"
	"github.com/syntrix/runtime/internal/tools"
)

type Step struct {
	Name            string            `json:"name"`
	Tool            string            `json:"tool"`
	Args            map[string]string `json:"args,omitempty"`
	Condition       string            `json:"condition"` // "always", "if_cpu_gt:75", "if_mem_gt:70", "if_disk_gt:80"
	RequireApproval bool              `json:"require_approval"`
	MaxRetries      int               `json:"max_retries"`
}

type Definition struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Steps []Step `json:"steps"`
}

type Engine struct {
	bus      *events.Bus
	registry *tools.Registry
	store    *store.Store
}

func NewEngine(bus *events.Bus, reg *tools.Registry, st *store.Store) *Engine {
	return &Engine{
		bus:      bus,
		registry: reg,
		store:    st,
	}
}

func (e *Engine) Execute(def Definition, snap engine.Snapshot, startStep int, approvedCheckpoint bool) store.WorkflowRun {
	runID := def.ID
	if runID == "" {
		runID = fmt.Sprintf("wf-%d", time.Now().UnixMilli())
	}

	run := store.WorkflowRun{
		ID:          runID,
		Name:        def.Name,
		Status:      "running",
		CurrentStep: startStep,
		TotalSteps:  len(def.Steps),
		UpdatedAtMs: time.Now().UnixMilli(),
	}
	e.persist(run)

	var out strings.Builder
	for i := startStep; i < len(def.Steps); i++ {
		step := def.Steps[i]
		run.CurrentStep = i
		run.UpdatedAtMs = time.Now().UnixMilli()

		if !evalCondition(step.Condition, snap) {
			out.WriteString(fmt.Sprintf("Step %d [%s]: SKIPPED (condition %q not met)\n", i+1, step.Name, step.Condition))
			e.emit(events.ActionExecuted, step.Name, "skipped condition "+step.Condition)
			continue
		}

		if step.RequireApproval && !approvedCheckpoint {
			run.Status = "awaiting_approval"
			out.WriteString(fmt.Sprintf("Step %d [%s]: PAUSED awaiting human approval.\n", i+1, step.Name))
			run.Output = strings.TrimSpace(out.String())
			e.persist(run)
			e.emit(events.PolicyApprovalRequired, step.Name, "workflow checkpoint awaiting approval")
			return run
		}
		approvedCheckpoint = false // consume single checkpoint approval

		retries := step.MaxRetries
		if retries < 0 {
			retries = 0
		}

		var res string
		var execErr error
		for attempt := 0; attempt <= retries; attempt++ {
			res, _, execErr = e.registry.Execute(step.Tool, step.Args)
			if execErr == nil {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}

		if execErr != nil {
			run.Status = "failed"
			out.WriteString(fmt.Sprintf("Step %d [%s]: FAILED after %d retries (%v)\n", i+1, step.Name, retries, execErr))
			run.Output = strings.TrimSpace(out.String())
			e.persist(run)
			e.emit(events.ActionExecuted, step.Name, "failed: "+execErr.Error())
			return run
		}

		out.WriteString(fmt.Sprintf("Step %d [%s]: OK\n%s\n\n", i+1, step.Name, res))
		e.emit(events.ActionExecuted, step.Name, "step completed")
	}

	run.Status = "completed"
	run.CurrentStep = len(def.Steps)
	run.Output = strings.TrimSpace(out.String())
	run.UpdatedAtMs = time.Now().UnixMilli()
	e.persist(run)
	if e.store != nil {
		_ = e.store.AppendAuditLog("WorkflowEngine", "execute_workflow", def.Name, "completed", fmt.Sprintf("steps=%d", len(def.Steps)))
	}
	return run
}

func evalCondition(cond string, snap engine.Snapshot) bool {
	c := strings.TrimSpace(strings.ToLower(cond))
	if c == "" || c == "always" {
		return true
	}
	parts := strings.SplitN(c, ":", 2)
	if len(parts) != 2 {
		return true
	}
	thresh, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return true
	}
	switch parts[0] {
	case "if_cpu_gt":
		return snap.CPU.TotalUsagePct > thresh
	case "if_mem_gt":
		return snap.Memory.UsedPct > thresh
	case "if_disk_gt":
		return snap.Disk.UsedPct > thresh
	default:
		return true
	}
}

func (e *Engine) persist(run store.WorkflowRun) {
	if e.store != nil {
		_ = e.store.UpsertWorkflowRun(run)
	}
}

func (e *Engine) emit(t events.EventType, target, summary string) {
	if e.bus != nil {
		e.bus.Publish(events.Event{
			Type:    t,
			Source:  "WorkflowEngine",
			Target:  target,
			Summary: summary,
		})
	}
}
