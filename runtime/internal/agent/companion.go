package agent

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/syntrix/runtime/internal/engine"
	"github.com/syntrix/runtime/internal/intelligence"
	"github.com/syntrix/runtime/internal/store"
)

type Mode string

const (
	ModeOff       Mode = "OFF"
	ModeMonitor   Mode = "MONITOR"
	ModeAssist    Mode = "ASSIST"
	ModeAutopilot Mode = "AUTOPILOT"
	ModeLearning  Mode = "LEARNING"
)

func ParseMode(raw string) (Mode, bool) {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case string(ModeOff):
		return ModeOff, true
	case string(ModeMonitor):
		return ModeMonitor, true
	case string(ModeAssist):
		return ModeAssist, true
	case string(ModeAutopilot):
		return ModeAutopilot, true
	case string(ModeLearning):
		return ModeLearning, true
	default:
		return "", false
	}
}

type Companion struct {
	mu               sync.RWMutex
	mode             Mode
	autoMode         bool
	lastAutoSwitchAt time.Time
	store            *store.Store
	analyzer         *intelligence.Analyzer
	orchestrator     *Orchestrator
	supervisor       *Supervisor
	guardian         *GuardianAgent
	lastSnap         *engine.Snapshot
	activeApp        string
	activeBundleID   string
	activeSince      time.Time
	appLastActive    map[string]time.Time
}

func NewCompanion(st *store.Store, analyzer *intelligence.Analyzer) *Companion {
	return &Companion{
		mode:          ModeAssist,
		store:         st,
		analyzer:      analyzer,
		appLastActive: make(map[string]time.Time),
	}
}

func (c *Companion) SetOrchestrator(o *Orchestrator) {
	c.mu.Lock()
	c.orchestrator = o
	c.mu.Unlock()
}

func (c *Companion) SetSupervisor(s *Supervisor) {
	c.mu.Lock()
	c.supervisor = s
	c.mu.Unlock()
}

func (c *Companion) GetSupervisor() *Supervisor {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.supervisor
}

func (c *Companion) SetGuardian(g *GuardianAgent) {
	c.mu.Lock()
	c.guardian = g
	c.mu.Unlock()
}

func (c *Companion) GetGuardian() *GuardianAgent {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.guardian
}

func (c *Companion) SetActiveContext(appName, bundleID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if c.appLastActive == nil {
		c.appLastActive = make(map[string]time.Time)
	}
	if c.activeApp != "" && !strings.EqualFold(c.activeApp, appName) {
		c.appLastActive[strings.ToLower(c.activeApp)] = now
	}
	if appName != "" {
		c.appLastActive[strings.ToLower(appName)] = now
		if !strings.EqualFold(c.activeApp, appName) {
			c.activeSince = now
		}
	} else {
		c.activeSince = time.Time{}
	}
	c.activeApp = appName
	c.activeBundleID = bundleID
}

func (c *Companion) ActiveApp() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.activeApp
}

func (c *Companion) ActiveContextInfo() (activeApp string, activeSince time.Time, lastActive map[string]time.Time) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	cp := make(map[string]time.Time, len(c.appLastActive))
	for k, v := range c.appLastActive {
		cp[k] = v
	}
	return c.activeApp, c.activeSince, cp
}

func (c *Companion) Mode() Mode {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.mode
}

func (c *Companion) SetMode(m Mode, actor string) {
	c.mu.Lock()
	prev := c.mode
	c.mode = m
	if actor == "user" {
		c.autoMode = false
	}
	c.mu.Unlock()

	if c.store != nil {
		_ = c.store.AppendAuditLog(actor, "set_mode", string(m), "ok", fmt.Sprintf("mode %s -> %s", prev, m))
	}
}

func (c *Companion) AutoMode() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.autoMode
}

func (c *Companion) SetAutoMode(enabled bool, actor string) {
	c.mu.Lock()
	c.autoMode = enabled
	c.lastAutoSwitchAt = time.Time{}
	c.mu.Unlock()
	if c.store != nil {
		_ = c.store.AppendAuditLog(actor, "set_auto_mode", fmt.Sprintf("%v", enabled), "ok", "dynamic mode switching")
	}
}

func (c *Companion) EvaluateDynamicMode(snap engine.Snapshot, govState string) (bool, Mode, string) {
	c.mu.Lock()
	if !c.autoMode || c.mode == ModeOff {
		c.mu.Unlock()
		return false, "", ""
	}
	now := time.Now()
	if !c.lastAutoSwitchAt.IsZero() && now.Sub(c.lastAutoSwitchAt) < 20*time.Second {
		c.mu.Unlock()
		return false, "", ""
	}
	activeApp := c.activeApp
	activeLower := strings.ToLower(activeApp)
	cur := c.mode
	c.mu.Unlock()

	target := ModeAssist
	reason := "balanced interactive assistance"
	if snap.Memory.PressurePct >= 75.0 || snap.Disk.UsedPct > 50.0 || govState == "ELEVATED" {
		target = ModeAutopilot
		reason = fmt.Sprintf("high pressure (mem %.0f%%, disk %.1f%%)", snap.Memory.PressurePct, snap.Disk.UsedPct)
	} else if isKnowledgeOrDevApp(activeLower) && snap.Memory.PressurePct < 65.0 && snap.CPU.TotalUsagePct < 55.0 {
		target = ModeLearning
		reason = fmt.Sprintf("dev/knowledge context (%s)", activeApp)
	}

	if target == cur {
		return false, cur, ""
	}
	c.mu.Lock()
	c.mode = target
	c.lastAutoSwitchAt = now
	c.mu.Unlock()
	if c.store != nil {
		_ = c.store.AppendAuditLog("dynamic_mode", "set_mode", string(target), "ok", reason)
	}
	return true, target, reason
}

func isKnowledgeOrDevApp(appLower string) bool {
	for _, kw := range []string{"xcode", "code", "cursor", "terminal", "iterm", "obsidian", "notes", "notion", "goland", "intellij"} {
		if strings.Contains(appLower, kw) {
			return true
		}
	}
	return false
}

func (c *Companion) UpdateSnapshot(snap engine.Snapshot) {
	c.mu.Lock()
	c.lastSnap = &snap
	c.mu.Unlock()
}

func (c *Companion) LatestSnapshot() (engine.Snapshot, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.lastSnap == nil {
		return engine.Snapshot{}, false
	}
	return *c.lastSnap, true
}

func (c *Companion) HandlePrompt(prompt string) (string, string) {
	trimmed := strings.TrimSpace(prompt)
	if trimmed == "" {
		return "Awaiting input. Try /status, /top, /scan, /caches, or /help.", "noop"
	}

	lower := strings.ToLower(trimmed)

	// Slash command routing
	if strings.HasPrefix(lower, "/mode ") {
		arg := strings.TrimSpace(trimmed[6:])
		if m, ok := ParseMode(arg); ok {
			c.SetMode(m, "user")
			return fmt.Sprintf("Operating mode switched to %s.", m), "set_mode"
		}
		return "Invalid mode. Valid modes: OFF, MONITOR, ASSIST, AUTOPILOT, LEARNING.", "set_mode"
	}

	switch {
	case lower == "/help" || lower == "help":
		_ = c.audit("companion", "tool:help", "system", "ok", "listed commands")
		return "Commands: /status, /diagnose, /optimize-memory, /top, /scan, /caches, /storage-forecast, /plan <goal>, /startup, /duplicates <dir>, /dev <dir>, /learn <topic>, /mode <OFF|MONITOR|ASSIST|AUTOPILOT|LEARNING>, /audit.", "tool:help"

	case lower == "/diagnose" || strings.HasPrefix(lower, "why is ") || strings.Contains(lower, "causal"):
		snap, ok := c.LatestSnapshot()
		if !ok {
			return "Telemetry is still warming up. Try again in a moment.", "tool:diagnosis"
		}
		diag := intelligence.NewPredictiveEngine().Evaluate(snap)
		var b strings.Builder
		fmt.Fprintf(&b, "Causal Diagnosis (confidence %.0f%%):\n• Summary: %s\n• Root Cause: %s\n• Impact: %s",
			diag.Confidence*100.0, diag.Summary, diag.RootCause, diag.Impact)
		if len(diag.Evidence) > 0 {
			fmt.Fprintf(&b, "\n• Evidence: %s", strings.Join(diag.Evidence, "; "))
		}
		if len(diag.AlternativeExplanations) > 0 {
			fmt.Fprintf(&b, "\n• Alternatives: %s", strings.Join(diag.AlternativeExplanations, "; "))
		}
		_ = c.audit("companion", "tool:diagnosis", "system", "ok", diag.RootCause)
		return b.String(), "tool:diagnosis"

	case strings.HasPrefix(lower, "/plan"):
		goal := strings.TrimSpace(trimmed[len("/plan"):])
		if goal == "" {
			goal = "Inspect system health and propose safe optimizations"
		}
		plan := NewTaskPlanner(nil).BuildPlan(goal, "assistant")
		var b strings.Builder
		fmt.Fprintf(&b, "Agent Plan [risk=%s | status=%s] for: %s\n", plan.Risk, plan.Status, plan.Goal)
		for i, st := range plan.Steps {
			gate := "auto"
			if st.RequiresApprove {
				gate = "APPROVAL REQUIRED"
			}
			fmt.Fprintf(&b, "%d. %s (%s) [%s]\n", i+1, st.Title, st.ToolName, gate)
		}
		_ = c.audit("companion", "tool:plan", plan.Risk, "ok", plan.Goal)
		return strings.TrimSpace(b.String()), "tool:plan"

	case lower == "/storage-forecast" || strings.Contains(lower, "forecast") || strings.Contains(lower, "predict"):
		snap, _ := c.LatestSnapshot()
		fc := intelligence.ForecastStorageGrowth(c.store, snap.Disk.UsedPct)
		_ = c.audit("OptimizationAgent", "storage_forecast", "/", "ok", fc.Summary)
		return fmt.Sprintf("Storage Growth Forecast:\n• %s\n• Growth rate: %.3f%%/day", fc.Summary, fc.GrowthPctPerDay), "tool:storage.forecast"

	case lower == "/startup" || strings.Contains(lower, "startup"):
		if sup := c.GetSupervisor(); sup != nil {
			return sup.InspectStartup(), "tool:hardening.startup_agents"
		}
		return "Supervisor unavailable.", "tool:hardening.startup_agents"

	case strings.HasPrefix(lower, "/duplicates"):
		dir := strings.TrimSpace(trimmed[len("/duplicates"):])
		if dir == "" {
			dir = "."
		}
		if sup := c.GetSupervisor(); sup != nil {
			res, err := sup.FindDuplicates(dir)
			if err != nil {
				return fmt.Sprintf("Duplicate scan failed: %v", err), "tool:hardening.duplicates"
			}
			return res, "tool:hardening.duplicates"
		}
		return "Supervisor unavailable.", "tool:hardening.duplicates"

	case strings.HasPrefix(lower, "/dev"):
		dir := strings.TrimSpace(trimmed[len("/dev"):])
		if sup := c.GetSupervisor(); sup != nil {
			return sup.DeveloperInspect(dir), "tool:developer.inspect"
		}
		return "Supervisor unavailable.", "tool:developer.inspect"

	case strings.HasPrefix(lower, "/learn"):
		topic := strings.TrimSpace(trimmed[len("/learn"):])
		if topic == "" {
			topic = "SYNTRIX Architecture"
		}
		if sup := c.GetSupervisor(); sup != nil {
			return sup.RAG().BuildLearningPath(topic), "tool:learning.path"
		}
		return "Supervisor unavailable.", "tool:learning.path"

	case lower == "/optimize-memory" || (strings.Contains(lower, "memory") && (strings.Contains(lower, "optimi") || strings.Contains(lower, "analy"))) || (strings.Contains(lower, "ram") && (strings.Contains(lower, "optimi") || strings.Contains(lower, "analy"))):
		c.mu.RLock()
		orch := c.orchestrator
		c.mu.RUnlock()
		if orch != nil {
			reply, _ := orch.RunMemoryOptimizationFlow(trimmed)
			return reply, "event:memory_optimization"
		}
		return c.toolTopProcesses(), "tool:list_top_processes"

	case lower == "/status" || strings.Contains(lower, "status") || strings.Contains(lower, "overview") || strings.Contains(lower, "how is my mac"):
		return c.toolStatus(), "tool:get_system_status"

	case strings.HasPrefix(lower, "inspect process") || strings.Contains(lower, "(pid "):
		return c.toolInspectProcess(trimmed), "tool:inspect_process"

	case lower == "/top" || strings.Contains(lower, "process") || strings.Contains(lower, "hog") || strings.Contains(lower, "cpu") || strings.Contains(lower, "memory") || strings.Contains(lower, "ram"):
		return c.toolTopProcesses(), "tool:list_top_processes"

	case lower == "/scan" || strings.Contains(lower, "optimi") || strings.Contains(lower, "diagnos") || strings.Contains(lower, "slow"):
		return c.toolScan(), "tool:run_optimization_scan"

	case lower == "/caches" || strings.Contains(lower, "cache") || strings.Contains(lower, "storage") || strings.Contains(lower, "disk"):
		return c.toolInspectCaches(), "tool:inspect_caches"

	case lower == "/audit" || strings.Contains(lower, "audit") || strings.Contains(lower, "history"):
		return c.toolAuditLog(), "tool:list_audit_log"

	case strings.Contains(lower, "battery") || strings.Contains(lower, "power"):
		return c.toolPower(), "tool:get_power_status"
	}

	_ = c.audit("companion", "chat", "local", "ok", trimmed)
	return c.reasonAndRespond(trimmed)
}

func (c *Companion) reasonAndRespond(prompt string) (string, string) {
	lower := strings.ToLower(strings.TrimSpace(prompt))
	snap, hasSnap := c.LatestSnapshot()
	activeApp := c.ActiveApp()

	c.mu.RLock()
	sup := c.supervisor
	guard := c.guardian
	c.mu.RUnlock()

	// 1. Software dev or knowledge work intent -> use local Supervisor Dev & RAG engines
	isDev := containsAny(lower, "code", "build", "git", "repo", "bug", "debug", "refactor", "compile", "test", "swift", "golang", "python", "develop", "architecture")
	isKnowledge := containsAny(lower, "research", "doc", "note", "summarize", "explain", "knowledge", "study", "write", "report", "compare", "why", "how does", "algorithm", "model")

	if sup != nil && isDev {
		return sup.DeveloperInspect("."), "tool:developer.inspect"
	}
	if sup != nil && isKnowledge {
		hits := sup.RAG().SemanticSearch(prompt, 4)
		if len(hits) > 0 {
			var b strings.Builder
			b.WriteString("SYNTRIX Knowledge Reasoning:\n")
			for _, h := range hits {
				fmt.Fprintf(&b, "• [%s] %s (sim %.2f)\n", h.Title, strings.TrimSpace(h.Snippet), h.Score)
			}
			return strings.TrimSpace(b.String()), "tool:rag.search"
		}
		return sup.RAG().BuildLearningPath(prompt), "tool:learning.path"
	}

	if !hasSnap {
		return "SYNTRIX core is online in " + string(c.Mode()) + " mode. Telemetry is warming up.", "chat"
	}

	// 2. Reason over live telemetry, causal diagnosis, and Guardian state
	diag := intelligence.NewPredictiveEngine().Evaluate(snap)
	guardState := "OFF"
	if guard != nil {
		guardState = guard.Status().State
	}
	var topProc string
	if len(snap.TopProcesses) > 0 {
		p := snap.TopProcesses[0]
		topProc = fmt.Sprintf("%s (PID %d, %.1f%% CPU, %.0f MB)", p.Name, p.PID, p.CPUPct, float64(p.MemoryBytes)/(1024*1024))
	}
	return fmt.Sprintf(
		"SYNTRIX Reasoning [%s | Guardian:%s | Active:%s]:\n• Diagnosis: %s (confidence %.0f%%)\n• Live Load: CPU %.1f%% · Mem Pressure %.0f%% · Disk %.1f%%/50%%\n• Heaviest Process: %s\n• Next: Ask me to optimize memory, clean unused storage, or run /antigravity for dev & knowledge tasks.",
		c.Mode(), guardState, fallbackStr(activeApp, "Desktop"),
		diag.Summary, diag.Confidence*100.0,
		snap.CPU.TotalUsagePct, snap.Memory.PressurePct, snap.Disk.UsedPct,
		fallbackStr(topProc, "None"),
	), "chat:reasoned"
}

func containsAny(s string, terms ...string) bool {
	for _, t := range terms {
		if strings.Contains(s, t) {
			return true
		}
	}
	return false
}

func fallbackStr(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func (c *Companion) ApproveRecommendation(id string) (string, error) {
	c.mu.RLock()
	orch := c.orchestrator
	c.mu.RUnlock()
	if orch != nil {
		return orch.ApproveAndVerify(id)
	}
	if c.store == nil {
		return "", fmt.Errorf("store unavailable")
	}
	snap, hasSnap := c.LatestSnapshot()
	after := "verified"
	if hasSnap {
		after = fmt.Sprintf("cpu=%.1f%% pressure=%.1f%%", snap.CPU.TotalUsagePct, snap.Memory.PressurePct)
	}
	if err := c.store.UpdateRecommendationStatus(id, "approved", after); err != nil {
		return "", err
	}
	_ = c.audit("user", "approve_recommendation", id, "approved", "explicit user approval; "+after)
	return fmt.Sprintf("Recommendation '%s' approved and logged (%s). No destructive action was taken automatically.", id, after), nil
}

func (c *Companion) PreviewRecommendation(id string) string {
	c.mu.RLock()
	orch := c.orchestrator
	c.mu.RUnlock()
	if orch != nil {
		return orch.PreviewRecommendation(id)
	}
	return fmt.Sprintf("Dry-run preview unavailable for '%s'.", id)
}

func (c *Companion) ActivateKillSwitch() int {
	c.mu.RLock()
	orch := c.orchestrator
	c.mu.RUnlock()
	if orch != nil {
		return orch.ActivateKillSwitch()
	}
	return 0
}

func (c *Companion) DismissRecommendation(id string) (string, error) {
	if c.store == nil {
		return "", fmt.Errorf("store unavailable")
	}
	if err := c.store.UpdateRecommendationStatus(id, "dismissed", "dismissed_by_user"); err != nil {
		return "", err
	}
	_ = c.audit("user", "dismiss_recommendation", id, "dismissed", "dismissed by user")
	return fmt.Sprintf("Recommendation '%s' dismissed.", id), nil
}

func (c *Companion) toolStatus() string {
	snap, ok := c.LatestSnapshot()
	if !ok {
		_ = c.audit("SystemAgent", "get_system_status", "host", "pending", "no snapshot yet")
		return "Telemetry snapshot not yet available."
	}
	_ = c.audit("SystemAgent", "get_system_status", "host", "ok", fmt.Sprintf("cpu=%.1f%% mem=%.1f%%", snap.CPU.TotalUsagePct, snap.Memory.UsedPct))
	usedGB := float64(snap.Memory.UsedBytes) / (1024 * 1024 * 1024)
	totalGB := float64(snap.Memory.TotalBytes) / (1024 * 1024 * 1024)
	swapMB := float64(snap.Memory.SwapUsedBytes) / (1024 * 1024)
	pwr := "AC Power"
	if snap.Power.HasBattery {
		state := "discharging"
		if snap.Power.IsCharging {
			state = "charging"
		}
		pwr = fmt.Sprintf("%.0f%% (%s)", snap.Power.BatteryPct, state)
	}
	return fmt.Sprintf(
		"System Status [%s]:\n• CPU: %.1f%% (%d cores; user %.1f%%, sys %.1f%%)\n• Memory: %.2f / %.2f GB (%.1f%%, pressure %.0f%% [%s], swap %.0f MB)\n• Thermal: %s (CPU speed limit %.0f%%)\n• Disk (/): %.1f%% used\n• Power: %s\n• Processes: %d active",
		c.Mode(),
		snap.CPU.TotalUsagePct, snap.CPU.CoreCount, snap.CPU.UserPct, snap.CPU.SystemPct,
		usedGB, totalGB, snap.Memory.UsedPct, snap.Memory.PressurePct, snap.Memory.PressureLevel, swapMB,
		snap.Thermal.StateLabel, snap.Thermal.CPUSpeedLimitPct,
		snap.Disk.UsedPct,
		pwr,
		snap.ProcessCountTotal,
	)
}

func (c *Companion) toolInspectProcess(prompt string) string {
	snap, ok := c.LatestSnapshot()
	if !ok || len(snap.TopProcesses) == 0 {
		return "No live process snapshot available yet."
	}

	var targetPID int32
	if idx := strings.Index(strings.ToLower(prompt), "pid "); idx >= 0 {
		rest := prompt[idx+4:]
		_, _ = fmt.Sscanf(rest, "%d", &targetPID)
	}

	lower := strings.ToLower(prompt)
	for _, p := range snap.TopProcesses {
		if (targetPID > 0 && p.PID == targetPID) || (targetPID == 0 && strings.Contains(lower, strings.ToLower(p.Name))) {
			rssMB := float64(p.MemoryBytes) / (1024 * 1024)
			virtMB := float64(p.VirtualBytes) / (1024 * 1024)
			memPct := 0.0
			if snap.Memory.TotalBytes > 0 {
				memPct = float64(p.MemoryBytes) * 100.0 / float64(snap.Memory.TotalBytes)
			}
			prot := "User process (safe to throttle if runaway)"
			if intelligence.IsProtectedProcess(p.PID, p.Name) {
				prot = "Protected macOS/SYNTRIX system process"
			}
			advice := "Running within normal bounds."
			if p.CPUPct >= 60 {
				advice = "High CPU load — consider throttling or closing heavy tabs/builds."
			} else if rssMB >= 1024 {
				advice = "High memory footprint — consider freeing RAM or restarting if idle."
			}
			_ = c.audit("SystemAgent", "inspect_process", fmt.Sprintf("%s(%d)", p.Name, p.PID), "ok", fmt.Sprintf("cpu=%.1f%% rss=%.0fMB", p.CPUPct, rssMB))
			return fmt.Sprintf(
				"Process Inspection: %s (PID %d)\n• CPU: %.1f%% | Threads: %d\n• Resident RAM: %.0f MB (%.1f%% of total) | Virtual: %.0f MB\n• Classification: %s\n• Recommendation: %s",
				p.Name, p.PID, p.CPUPct, p.Threads, rssMB, memPct, virtMB, prot, advice,
			)
		}
	}
	return fmt.Sprintf("Process (PID %d) is no longer in the top active processes or has exited.", targetPID)
}

func (c *Companion) toolTopProcesses() string {
	snap, ok := c.LatestSnapshot()
	if !ok || len(snap.TopProcesses) == 0 {
		_ = c.audit("SystemAgent", "list_top_processes", "host", "empty", "no process data")
		return "No process samples collected yet."
	}
	_ = c.audit("SystemAgent", "list_top_processes", "host", "ok", fmt.Sprintf("returned %d procs", minInt(5, len(snap.TopProcesses))))
	var b strings.Builder
	b.WriteString("Top Active Processes:\n")
	limit := minInt(6, len(snap.TopProcesses))
	for i := 0; i < limit; i++ {
		p := snap.TopProcesses[i]
		b.WriteString(fmt.Sprintf("%d. %s (PID %d) — CPU %.1f%% | RAM %.1f MB | %d threads\n",
			i+1, p.Name, p.PID, p.CPUPct, float64(p.MemoryBytes)/(1024*1024), p.Threads))
	}
	return strings.TrimSpace(b.String())
}

func (c *Companion) toolScan() string {
	_ = c.audit("OptimizationAgent", "run_optimization_scan", "host", "ok", "scanned recommendations")
	if c.store == nil {
		return "Persistence store unavailable."
	}
	recs, err := c.store.ListRecommendations(5)
	if err != nil {
		return fmt.Sprintf("Scan failed: %v", err)
	}
	if len(recs) == 0 {
		return "Optimization Scan: No active anomalies or bottlenecks detected. System is operating within healthy bounds."
	}
	var b strings.Builder
	b.WriteString("Optimization Findings:\n")
	for _, r := range recs {
		b.WriteString(fmt.Sprintf("• [%s/%s] %s — %s\n", strings.ToUpper(r.Severity), r.Status, r.Title, r.Detail))
	}
	return strings.TrimSpace(b.String())
}

func (c *Companion) toolInspectCaches() string {
	cands, total, err := intelligence.InspectUserCaches(5)
	if err != nil {
		_ = c.audit("OptimizationAgent", "inspect_caches", "~/Library/Caches", "error", err.Error())
		return fmt.Sprintf("Cache inspection error: %v", err)
	}
	_ = c.audit("OptimizationAgent", "inspect_caches", "~/Library/Caches", "ok", fmt.Sprintf("total=%.1fMB", float64(total)/(1024*1024)))
	var b strings.Builder
	b.WriteString(fmt.Sprintf("User Cache Inspection (~/Library/Caches, read-only): %.2f GB total\n", float64(total)/(1024*1024*1024)))
	for _, cand := range cands {
		b.WriteString(fmt.Sprintf("• %s — %.1f MB\n", cand.Name, float64(cand.SizeBytes)/(1024*1024)))
	}
	return strings.TrimSpace(b.String())
}

func (c *Companion) toolAuditLog() string {
	if c.store == nil {
		return "Audit store unavailable."
	}
	logs, err := c.store.ListAuditLogs(6)
	if err != nil {
		return fmt.Sprintf("Failed to read audit logs: %v", err)
	}
	if len(logs) == 0 {
		return "Audit log is currently empty."
	}
	var b strings.Builder
	b.WriteString("Recent Audit Trail:\n")
	for _, l := range logs {
		b.WriteString(fmt.Sprintf("• [%s] %s -> %s (%s): %s\n", l.Actor, l.Action, l.Target, l.Status, l.Detail))
	}
	return strings.TrimSpace(b.String())
}

func (c *Companion) toolPower() string {
	snap, ok := c.LatestSnapshot()
	if !ok {
		return "Power telemetry not yet available."
	}
	_ = c.audit("SystemAgent", "get_power_status", "battery", "ok", fmt.Sprintf("battery=%.0f%%", snap.Power.BatteryPct))
	if !snap.Power.HasBattery {
		return "Power Status: Running on AC Power (no internal battery detected)."
	}
	return fmt.Sprintf("Power Status: Battery %.0f%% | Charging: %v | AC Power: %v", snap.Power.BatteryPct, snap.Power.IsCharging, snap.Power.OnACPower)
}

func (c *Companion) audit(actor, action, target, status, detail string) error {
	if c.store == nil {
		return nil
	}
	return c.store.AppendAuditLog(actor, action, target, status, detail)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
