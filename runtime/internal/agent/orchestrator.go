package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/syntrix/runtime/internal/engine"
	"github.com/syntrix/runtime/internal/events"
	"github.com/syntrix/runtime/internal/intelligence"
	"github.com/syntrix/runtime/internal/policy"
	"github.com/syntrix/runtime/internal/store"
)

type SnapshotProvider interface {
	Collect(maxProcs uint32) (engine.Snapshot, error)
}

type Orchestrator struct {
	bus       *events.Bus
	policy    *policy.Engine
	collector SnapshotProvider
	store     *store.Store
	companion *Companion
	optCtrl   *intelligence.OptimizationController
}

func NewOrchestrator(
	bus *events.Bus,
	pol *policy.Engine,
	collector SnapshotProvider,
	st *store.Store,
	companion *Companion,
) *Orchestrator {
	return &Orchestrator{
		bus:       bus,
		policy:    pol,
		collector: collector,
		store:     st,
		companion: companion,
		optCtrl:   intelligence.NewOptimizationController(),
	}
}

// RunMemoryOptimizationFlow executes the event-driven sequence:
// User -> SwiftUI -> GoRuntime -> CEngine -> OptimizationAgent -> PolicyEngine -> SwiftUI.
func (o *Orchestrator) RunMemoryOptimizationFlow(prompt string) (string, store.Recommendation) {
	corrID := fmt.Sprintf("mem-%d", time.Now().UnixMilli())
	mode := string(o.companion.Mode())

	o.publish(events.UserRequestSubmitted, corrID, "SwiftUI", "GoRuntime",
		fmt.Sprintf("Submitted request: %s", prompt))

	o.publish(events.TelemetrySnapshotRequested, corrID, "GoRuntime", "CSystemEngine",
		"Request memory & process snapshot")

	snap, ok := o.collectFreshSnapshot()
	if !ok {
		return "Unable to collect memory snapshot from C17 engine.", store.Recommendation{}
	}
	o.companion.UpdateSnapshot(snap)

	o.publish(events.TelemetrySnapshotCollected, corrID, "CSystemEngine", "GoRuntime",
		fmt.Sprintf("Memory %.1f%% (%.2f/%.1f GB), pressure %.0f%%, %d procs",
			snap.Memory.UsedPct,
			float64(snap.Memory.UsedBytes)/(1024*1024*1024),
			float64(snap.Memory.TotalBytes)/(1024*1024*1024),
			snap.Memory.PressurePct,
			snap.ProcessCountTotal))

	o.publish(events.AnalysisRequested, corrID, "GoRuntime", "OptimizationAgent",
		"Analyze system memory state & identify bottlenecks")

	rec := o.analyzeMemoryBottleneck(snap)

	o.publish(events.RecommendationProposed, corrID, "OptimizationAgent", "GoRuntime",
		fmt.Sprintf("Proposed: %s", rec.Title))

	dec := o.policy.ValidateAction(corrID, rec, mode)
	if !dec.Allowed {
		return dec.Reason, rec
	}

	if o.store != nil {
		_ = o.store.UpsertRecommendation(rec)
		_ = o.store.AppendAuditLog("OptimizationAgent", "propose_optimization", rec.TargetName, "pending_approval", rec.BeforeMetric)
	}

	o.publish(events.RecommendationPresented, corrID, "GoRuntime", "SwiftUI",
		fmt.Sprintf("Awaiting user approval for '%s'", rec.ID))

	reply := fmt.Sprintf(
		"Memory Analysis Complete [%s]:\n• Used: %.2f / %.2f GB (%.1f%%) | Compressed: %.0f MB | Swap: %.0f MB | Pressure: %.0f%% [%s]\n• Bottleneck: %s\n• Recommendation (%s): %s\n→ Review & click Approve in the OPTIMIZATION panel to execute and verify.",
		corrID,
		float64(snap.Memory.UsedBytes)/(1024*1024*1024),
		float64(snap.Memory.TotalBytes)/(1024*1024*1024),
		snap.Memory.UsedPct,
		float64(snap.Memory.CompressedBytes)/(1024*1024),
		float64(snap.Memory.SwapUsedBytes)/(1024*1024),
		snap.Memory.PressurePct,
		snap.Memory.PressureLevel,
		rec.Title,
		rec.ID,
		rec.Detail,
	)
	return reply, rec
}

func (o *Orchestrator) PreviewRecommendation(recID string) string {
	var rec store.Recommendation
	if o.store != nil {
		if r, err := o.store.GetRecommendation(recID); err == nil {
			rec = r
		}
	}
	snap, _ := o.companion.LatestSnapshot()
	res := o.optCtrl.Execute(context.Background(), rec, true, snap, nil)
	return fmt.Sprintf("Dry-Run Preview [%s | Risk: %s | Reversible: %v]\n• %s\n• Expected: %s",
		recID, res.Spec.Risk, res.Spec.Reversible, res.Summary, res.Spec.ExpectedEffect)
}

func (o *Orchestrator) ActivateKillSwitch() int {
	if o.optCtrl == nil {
		return 0
	}
	n := o.optCtrl.ActivateKillSwitch()
	if o.store != nil {
		_ = o.store.AppendAuditLog("user", "kill_switch", "optimizations", "activated", fmt.Sprintf("cancelled %d active action(s)", n))
	}
	return n
}

func (o *Orchestrator) ApproveAndVerify(recID string) (string, error) {
	corrID := fmt.Sprintf("exec-%d", time.Now().UnixMilli())
	mode := string(o.companion.Mode())

	o.publish(events.ActionApprovalReceived, corrID, "SwiftUI", "GoRuntime",
		fmt.Sprintf("User approved recommendation '%s'", recID))

	if !o.policy.RevalidateAuthorization(corrID, recID, mode) {
		return "", fmt.Errorf("policy denied execution in mode %s", mode)
	}

	beforeSnap, _ := o.companion.LatestSnapshot()

	o.publish(events.ActionExecutionRequested, corrID, "GoRuntime", "CSystemEngine",
		fmt.Sprintf("Execute supported optimization operation for '%s'", recID))

	// Execute typed, precondition-checked optimization for the specific recommendation.
	var rec store.Recommendation
	if o.store != nil {
		if r, err := o.store.GetRecommendation(recID); err == nil {
			rec = r
		}
	}
	var afterSnap engine.Snapshot
	res := o.optCtrl.Execute(context.Background(), rec, false, beforeSnap, func() (engine.Snapshot, bool) {
		o.publish(events.VerificationSnapshotRequested, corrID, "GoRuntime", "CSystemEngine", "Collect post-action verification snapshot")
		s, ok := o.collectFreshSnapshot()
		if ok {
			afterSnap = s
			o.companion.UpdateSnapshot(s)
		}
		return s, ok
	})
	if res.Cancelled {
		return "", fmt.Errorf("%s", res.Summary)
	}
	actionSummary := res.Summary
	if afterSnap.TimestampMs == 0 {
		afterSnap = beforeSnap
	}

	o.publish(events.ActionExecuted, corrID, "CSystemEngine", "GoRuntime", actionSummary)
	o.publish(events.VerificationSnapshotCollected, corrID, "CSystemEngine", "GoRuntime", res.VerificationReport)

	beforeMB := float64(beforeSnap.Memory.UsedBytes) / (1024 * 1024)
	afterMB := float64(afterSnap.Memory.UsedBytes) / (1024 * 1024)
	deltaMB := afterMB - beforeMB
	measured := fmt.Sprintf("mem_used=%.0fMB->%.0fMB (delta=%+.0fMB) pressure=%.0f%%->%.0f%%",
		beforeMB, afterMB, deltaMB, beforeSnap.Memory.PressurePct, afterSnap.Memory.PressurePct)

	if o.store != nil {
		if err := o.store.UpdateRecommendationStatus(recID, "approved", measured); err != nil {
			return "", err
		}
		_ = o.store.AppendAuditLog("ControlledExecutor", "execute_and_verify", recID, "verified", measured)
	}

	o.publish(events.OutcomeMeasured, corrID, "GoRuntime", "SwiftUI",
		fmt.Sprintf("Verified outcome for '%s': %s", recID, measured))

	return fmt.Sprintf("Optimization '%s' executed & verified.\n• Action: %s\n• Measured Outcome: %s", recID, actionSummary, measured), nil
}

func (o *Orchestrator) collectFreshSnapshot() (engine.Snapshot, bool) {
	if o.collector != nil {
		if snap, err := o.collector.Collect(12); err == nil {
			return snap, true
		}
	}
	return o.companion.LatestSnapshot()
}

func (o *Orchestrator) analyzeMemoryBottleneck(snap engine.Snapshot) store.Recommendation {
	var topProc engine.ProcessInfo
	for _, p := range snap.TopProcesses {
		if p.MemoryBytes > topProc.MemoryBytes {
			topProc = p
		}
	}

	severity := "low"
	if snap.Memory.PressurePct >= 80 || snap.Memory.UsedPct >= 85 {
		severity = "high"
	} else if snap.Memory.PressurePct >= 60 || snap.Memory.UsedPct >= 70 {
		severity = "medium"
	}

	title := fmt.Sprintf("Memory optimization: %s (%.0f MB)", topProc.Name, float64(topProc.MemoryBytes)/(1024*1024))
	if topProc.Name == "" {
		title = fmt.Sprintf("Memory optimization (%.1f%% used)", snap.Memory.UsedPct)
	}

	detail := fmt.Sprintf(
		"Top consumer is %s (PID %d, %.0f MB). Approve to run safe memory/cache verification and measure before-and-after impact.",
		topProc.Name, topProc.PID, float64(topProc.MemoryBytes)/(1024*1024),
	)

	return store.Recommendation{
		ID:           fmt.Sprintf("mem-opt-%d", snap.TimestampMs%100000),
		CreatedAtMs:  snap.TimestampMs,
		Kind:         "memory_optimization",
		Severity:     severity,
		Title:        title,
		Detail:       detail,
		TargetPID:    topProc.PID,
		TargetName:   strings.TrimSpace(topProc.Name),
		Status:       "pending",
		BeforeMetric: fmt.Sprintf("used=%.0fMB pressure=%.0f%%", float64(snap.Memory.UsedBytes)/(1024*1024), snap.Memory.PressurePct),
	}
}

func (o *Orchestrator) publish(t events.EventType, corrID, src, dst, summary string) {
	if o.bus == nil {
		return
	}
	o.bus.Publish(events.Event{
		Type:          t,
		CorrelationID: corrID,
		Source:        src,
		Target:        dst,
		Summary:       summary,
	})
}
