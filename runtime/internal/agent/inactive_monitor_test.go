package agent

import (
	"testing"
	"time"

	"github.com/syntrix/runtime/internal/engine"
)

func TestInactiveAppMonitorDetectsIdleHeavyProcess(t *testing.T) {
	now := time.Unix(1700000000, 0)
	mon := NewInactiveAppMonitor()
	mon.nowFn = func() time.Time { return now }
	mon.MinObservedSamples = 2
	mon.InactivityThreshold = 120 * time.Second

	snap := engine.Snapshot{
		Memory: engine.MemoryMetrics{PressurePct: 55},
		TopProcesses: []engine.ProcessInfo{
			{PID: 501, Name: "Google Chrome", CPUPct: 12.0, MemoryBytes: 900 * 1024 * 1024},
			{PID: 502, Name: "Google Chrome Helper", CPUPct: 1.5, MemoryBytes: 750 * 1024 * 1024},
			{PID: 601, Name: "Slack", CPUPct: 0.4, MemoryBytes: 620 * 1024 * 1024},
			{PID: 602, Name: "ffmpeg", CPUPct: 45.0, MemoryBytes: 500 * 1024 * 1024},
			{PID: 1, Name: "launchd", CPUPct: 0.1, MemoryBytes: 400 * 1024 * 1024},
		},
	}

	activeSince := now.Add(-5 * time.Minute)
	lastActive := map[string]time.Time{
		"google chrome": now,
		"slack":         now.Add(-4 * time.Minute),
	}

	// First observation: below MinObservedSamples.
	rec, notif := mon.Evaluate(snap, "Google Chrome", activeSince, lastActive)
	if rec != nil || notif != nil {
		t.Fatalf("expected nil on first sample before MinObservedSamples, got rec=%v notif=%v", rec, notif)
	}

	// Second observation: should detect Slack (not Chrome Helper, not active ffmpeg, not launchd).
	now = now.Add(2 * time.Second)
	rec, notif = mon.Evaluate(snap, "Google Chrome", activeSince, lastActive)
	if rec == nil || notif == nil {
		t.Fatal("expected inactive app recommendation and notification")
	}
	if rec.Kind != "inactive_app_memory" || rec.TargetName != "Slack" || rec.TargetPID != 601 {
		t.Fatalf("unexpected recommendation: %+v", rec)
	}
	if notif.RecID != rec.ID || notif.TargetName != "Slack" {
		t.Fatalf("unexpected notification: %+v", notif)
	}

	// Immediate subsequent call: should be suppressed by cooldown.
	now = now.Add(10 * time.Second)
	rec2, notif2 := mon.Evaluate(snap, "Google Chrome", activeSince, lastActive)
	if rec2 != nil || notif2 != nil {
		t.Fatalf("expected cooldown to suppress duplicate notification, got rec=%v notif=%v", rec2, notif2)
	}
}
