package agent

import (
	"fmt"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/syntrix/runtime/internal/cognitive"
	"github.com/syntrix/runtime/internal/engine"
	"github.com/syntrix/runtime/internal/intelligence"
	"github.com/syntrix/runtime/internal/store"
)

type GuardianStatus struct {
	Enabled            bool    `json:"enabled"`
	State              string  `json:"state"`
	IdleSeconds        int     `json:"idle_seconds"`
	IdleThresholdSec   int     `json:"idle_threshold_sec"`
	ActionsTaken       int     `json:"actions_taken"`
	ProactivePrevented int     `json:"proactive_prevented"`
	SkippedUnsafe      int     `json:"skipped_unsafe"`
	RLAvgReward        float64 `json:"rl_avg_reward"`
	DevBoostActive     bool    `json:"dev_boost_active"`
	FocusContext       string  `json:"focus_context,omitempty"`
	LastAction         string  `json:"last_action"`
	LastCheckMs        int64   `json:"last_check_ms"`
}

// GuardianAgent monitors the Mac when the user is away (or in AUTOPILOT)
// and safely executes reversible optimizations within rate/cooldown bounds.
type GuardianAgent struct {
	mu                 sync.Mutex
	enabled            bool
	state              string
	idleThresholdSec   int
	actionsTaken       int
	proactivePrevented int
	skippedUnsafe      int
	rlAvgReward        float64
	rlRewardCount      int
	rlActionScores     map[string]float64
	devBoostActive     bool
	focusContext       string
	pendingVerifySnap  *engine.Snapshot
	pendingVerifyAct   string
	pendingPredMem     float64
	lastAction         string
	lastCheckMs        int64
	lastIdleSampleAt   time.Time
	cachedIdleSec      int
	lastKindActionAt   map[string]time.Time
	hourlyActions      []time.Time
	orch               *Orchestrator
	store              *store.Store
	nce                *cognitive.NeuralCognitiveEngine
	idleProvider       func() int
}

func NewGuardianAgent(orch *Orchestrator, st *store.Store) *GuardianAgent {
	return &GuardianAgent{
		enabled:          true,
		state:            "WATCHING",
		idleThresholdSec: 180, // 3 minutes of user absence by default
		lastAction:       "Monitoring system health",
		rlActionScores:   make(map[string]float64),
		lastKindActionAt: make(map[string]time.Time),
		orch:             orch,
		store:            st,
		idleProvider:     readMacOSIdleSeconds,
	}
}

func (g *GuardianAgent) SetCognitiveEngine(nce *cognitive.NeuralCognitiveEngine) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nce = nce
}

func (g *GuardianAgent) SetIdleProvider(fn func() int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if fn != nil {
		g.idleProvider = fn
		g.lastIdleSampleAt = time.Time{}
	}
}

func (g *GuardianAgent) Configure(enabled bool, idleThresholdSec int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.enabled = enabled
	if idleThresholdSec >= 10 {
		g.idleThresholdSec = idleThresholdSec
	}
	if !enabled {
		g.state = "DISABLED"
	} else if g.state == "DISABLED" {
		g.state = "WATCHING"
	}
}

func (g *GuardianAgent) Status() GuardianStatus {
	g.mu.Lock()
	defer g.mu.Unlock()
	return GuardianStatus{
		Enabled:            g.enabled,
		State:              g.state,
		IdleSeconds:        g.cachedIdleSec,
		IdleThresholdSec:   g.idleThresholdSec,
		ActionsTaken:       g.actionsTaken,
		ProactivePrevented: g.proactivePrevented,
		SkippedUnsafe:      g.skippedUnsafe,
		RLAvgReward:        g.rlAvgReward,
		DevBoostActive:     g.devBoostActive,
		FocusContext:       g.focusContext,
		LastAction:         g.lastAction,
		LastCheckMs:        g.lastCheckMs,
	}
}

// Evaluate inspects pending recommendations without an external forecast.
func (g *GuardianAgent) Evaluate(mode Mode, snap engine.Snapshot, recs []store.Recommendation) (string, string, bool) {
	return g.EvaluateWithForecast(mode, snap, recs, nil)
}

// EvaluateWithForecast combines pending recommendations, Transformer/ML forecasts,
// and RL action policies to prevent system parameters from exceeding thresholds.
func (g *GuardianAgent) EvaluateWithForecast(
	mode Mode,
	snap engine.Snapshot,
	recs []store.Recommendation,
	forecast *intelligence.PythonForecast,
) (string, string, bool) {
	now := time.Now()
	activeApp := ""
	if g.orch != nil && g.orch.companion != nil {
		activeApp = g.orch.companion.ActiveApp()
	}
	devBoost := isDevOrKnowledgeApp(activeApp)

	g.mu.Lock()
	g.lastCheckMs = now.UnixMilli()
	g.focusContext = activeApp
	g.devBoostActive = devBoost

	// Online RL reward feedback from previous proactive/fulfilled action
	if g.pendingVerifySnap != nil {
		prev := *g.pendingVerifySnap
		act := g.pendingVerifyAct
		predMem := g.pendingPredMem
		nce := g.nce
		g.pendingVerifySnap = nil
		g.pendingVerifyAct = ""

		stayedSafe := snap.Memory.PressurePct < 75.0 && snap.CPU.TotalUsagePct < 80.0
		deltaMem := prev.Memory.PressurePct - snap.Memory.PressurePct
		deltaCPU := prev.CPU.TotalUsagePct - snap.CPU.TotalUsagePct
		reward := (deltaMem*0.05 + deltaCPU*0.03)
		if stayedSafe {
			reward += 0.5
		} else {
			reward -= 0.4
		}
		g.rlRewardCount++
		g.rlAvgReward = math.Round(((g.rlAvgReward*float64(g.rlRewardCount-1)+reward)/float64(g.rlRewardCount))*1000.0) / 1000.0
		oldQ := g.rlActionScores[act]
		g.rlActionScores[act] = oldQ + 0.25*(reward-oldQ)
		if nce != nil {
			nce.RecordOutcome(act, stayedSafe, predMem, snap.Memory.PressurePct)
		}
	}

	if !g.enabled {
		g.state = "DISABLED"
		g.mu.Unlock()
		return "", "", false
	}

	if g.orch != nil && g.orch.optCtrl != nil && g.orch.optCtrl.IsKilled() {
		g.state = "PAUSED_KILL_SWITCH"
		g.mu.Unlock()
		return "", "", false
	}

	if mode == ModeOff {
		g.state = "OFF"
		g.mu.Unlock()
		return "", "", false
	}

	if now.Sub(g.lastIdleSampleAt) >= 15*time.Second || g.lastIdleSampleAt.IsZero() {
		g.cachedIdleSec = g.idleProvider()
		g.lastIdleSampleAt = now
	}
	idleSec := g.cachedIdleSec
	standbyState := "ACTIVE_GUARD"
	maxHourly := 6
	kindCooldown := 3 * time.Minute
	if mode == ModeAutopilot {
		standbyState = "AUTOPILOT_ACTIVE"
		maxHourly = 12
		kindCooldown = 90 * time.Second
	} else if idleSec >= g.idleThresholdSec {
		standbyState = "WATCHING_AWAY"
	}

	// Prune hourly rate-limit window
	cutoff := now.Add(-1 * time.Hour)
	kept := g.hourlyActions[:0]
	for _, t := range g.hourlyActions {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	g.hourlyActions = kept
	if len(g.hourlyActions) >= maxHourly {
		g.state = "COOLDOWN"
		g.mu.Unlock()
		return "", "", false
	}

	var candidate *store.Recommendation
	for i := range recs {
		rec := recs[i]
		if rec.Status != "" && rec.Status != "pending" {
			continue
		}
		spec := intelligence.BuildActionSpec(rec)
		allowOver50Storage := rec.Kind == "storage_pressure" && snap.Disk.UsedPct > intelligence.DefaultStorageLimitPct
		if spec.Risk == intelligence.ActionRiskIrreversible && !allowOver50Storage {
			g.skippedUnsafe++
			continue
		}
		if lastAt, ok := g.lastKindActionAt[rec.Kind]; ok && now.Sub(lastAt) < kindCooldown {
			continue
		}
		if rec.Severity == "low" && snap.Memory.PressurePct < 70 && snap.CPU.TotalUsagePct < 75 {
			continue
		}
		candidate = &rec
		break
	}

	proactive := false
	rlActionName := ""
	predMemForVerify := snap.Memory.PressurePct
	if candidate == nil && forecast != nil {
		memHeadingHigh := forecast.MemoryForecast.Predicted5mPct >= 75.0 ||
			forecast.MemoryForecast.Predicted15mPct >= 80.0 ||
			(forecast.TransformerRL != nil && forecast.TransformerRL.PredMemPct >= 75.0)
		cpuHeadingHigh := forecast.TransformerRL != nil && forecast.TransformerRL.PredCPUPct >= 80.0
		if forecast.TransformerRL != nil {
			predMemForVerify = forecast.TransformerRL.PredMemPct
		}

		if memHeadingHigh || cpuHeadingHigh {
			// Pick best reversible RL action using offline Q-policy + online reward adjustment
			chosen := "trim_daemon_heap"
			if forecast.TransformerRL != nil && forecast.TransformerRL.OptimalAction != "" && forecast.TransformerRL.OptimalAction != "monitor_only" {
				chosen = forecast.TransformerRL.OptimalAction
			} else if cpuHeadingHigh && !memHeadingHigh {
				chosen = "nice_background_cpu"
			}
			// If online RL learned negative score for nice_background_cpu vs trim_daemon_heap, prefer higher score
			if g.rlActionScores["trim_daemon_heap"] > g.rlActionScores[chosen] && memHeadingHigh {
				chosen = "trim_daemon_heap"
			}

			if chosen == "nice_background_cpu" && len(snap.TopProcesses) > 0 {
				var target engine.ProcessInfo
				for _, p := range snap.TopProcesses {
					if intelligence.IsProtectedProcess(p.PID, p.Name) {
						continue
					}
					if devBoost && (isProcessMatchingApp(p.Name, activeApp) || isProtectedBuildTool(p.Name)) {
						continue
					}
					if p.CPUPct >= target.CPUPct {
						target = p
					}
				}
				if target.PID > 100 {
					if lastAt, ok := g.lastKindActionAt["high_cpu"]; !ok || now.Sub(lastAt) >= 3*time.Minute {
						proactive = true
						rlActionName = chosen
						candidate = &store.Recommendation{
							ID:          fmt.Sprintf("proactive-rl-cpu-%d", now.UnixMilli()%100000),
							CreatedAtMs: now.UnixMilli(),
							Kind:        "high_cpu",
							Severity:    "medium",
							Title:       fmt.Sprintf("Proactive RL CPU throttle: %s", target.Name),
							Detail:      fmt.Sprintf("Transformer/RL forecast predicted CPU/load breach; applying nice +10 to %s (PID %d)", target.Name, target.PID),
							TargetPID:   target.PID,
							TargetName:  target.Name,
							Status:      "pending",
						}
					}
				}
			}

			if candidate == nil && chosen != "preview_stale_caches" {
				if lastAt, ok := g.lastKindActionAt["memory_pressure"]; !ok || now.Sub(lastAt) >= 3*time.Minute {
					proactive = true
					rlActionName = "trim_daemon_heap"
					candidate = &store.Recommendation{
						ID:          fmt.Sprintf("proactive-rl-mem-%d", now.UnixMilli()%100000),
						CreatedAtMs: now.UnixMilli(),
						Kind:        "memory_pressure",
						Severity:    "medium",
						Title:       "Proactive RL memory trim (pre-threshold)",
						Detail:      fmt.Sprintf("Forecast predicted %.1f%% mem pressure; trimming runtime heap before threshold breach.", predMemForVerify),
						Status:      "pending",
					}
				}
			}
		}
	}

	if candidate == nil && snap.Disk.UsedPct > intelligence.DefaultStorageLimitPct {
		if lastAt, ok := g.lastKindActionAt["storage_pressure"]; !ok || now.Sub(lastAt) >= 10*time.Minute {
			proactive = true
			rlActionName = "preview_stale_caches"
			candidate = &store.Recommendation{
				ID:          fmt.Sprintf("proactive-rl-storage-%d", now.UnixMilli()%100000),
				CreatedAtMs: now.UnixMilli(),
				Kind:        "storage_pressure",
				Severity:    "medium",
				Title:       fmt.Sprintf("Guardian Storage Guard (%.1f%% > 50%%)", snap.Disk.UsedPct),
				Detail:      "Storage exceeded 50% limit; cleaning stale non-Apple caches (>3d) and logs (>7d).",
				TargetName:  "/",
				Status:      "pending",
			}
		}
	}

	if candidate == nil || g.orch == nil {
		g.state = standbyState
		g.mu.Unlock()
		return "", "", false
	}

	if rlActionName == "" {
		rlActionName = candidate.Kind
	}
	if proactive && g.store != nil {
		_ = g.store.UpsertRecommendation(*candidate)
	}

	g.state = "ACTING"
	g.lastKindActionAt[candidate.Kind] = now
	g.hourlyActions = append(g.hourlyActions, now)
	orch := g.orch
	st := g.store
	snapCopy := snap
	g.mu.Unlock()

	msg, err := orch.ApproveAndVerify(candidate.ID)

	g.mu.Lock()
	defer g.mu.Unlock()
	if err != nil {
		g.lastAction = "Failed " + candidate.ID + ": " + err.Error()
		g.state = standbyState
		return "", "", false
	}
	g.actionsTaken++
	if proactive {
		g.proactivePrevented++
	}
	g.pendingVerifySnap = &snapCopy
	g.pendingVerifyAct = rlActionName
	g.pendingPredMem = predMemForVerify
	g.lastAction = "Fulfilled " + candidate.Title
	g.state = standbyState

	if st != nil {
		_ = st.AppendAuditLog("GuardianAgent", "autonomous_fulfill", candidate.ID, "ok", candidate.Title)
	}
	return candidate.ID, msg, true
}

var hidIdleRe = regexp.MustCompile(`"HIDIdleTime"\s*=\s*(\d+)`)

func readMacOSIdleSeconds() int {
	out, err := exec.Command("ioreg", "-c", "IOHIDSystem", "-d", "4").Output()
	if err != nil {
		return 0
	}
	m := hidIdleRe.FindSubmatch(out)
	if len(m) < 2 {
		return 0
	}
	ns, err := strconv.ParseUint(string(m[1]), 10, 64)
	if err != nil {
		return 0
	}
	return int(ns / 1_000_000_000)
}

func isDevOrKnowledgeApp(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return false
	}
	for _, kw := range []string{
		"antigravity", "syntrix", "xcode", "code", "cursor", "intellij", "goland",
		"pycharm", "webstorm", "terminal", "iterm", "warp", "obsidian", "notion",
		"safari", "chrome", "arc", "firefox",
	} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func isProtectedBuildTool(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "go", "swiftc", "swift", "clang", "clang++", "rustc", "cargo", "python3",
		"node", "xcodebuild", "java", "javac", "gradle", "mvn", "git", "make":
		return true
	}
	return false
}
