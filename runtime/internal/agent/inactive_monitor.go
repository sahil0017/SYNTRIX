package agent

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/syntrix/runtime/internal/engine"
	"github.com/syntrix/runtime/internal/intelligence"
	"github.com/syntrix/runtime/internal/store"
)

// InactiveAppNotification describes a macOS notification prompt for cleaning an idle background app's memory.
type InactiveAppNotification struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	Subtitle   string  `json:"subtitle,omitempty"`
	Body       string  `json:"body"`
	RecID      string  `json:"rec_id"`
	TargetPID  int32   `json:"target_pid"`
	TargetName string  `json:"target_name"`
	RSSMB      float64 `json:"rss_mb"`
	ActiveApp  string  `json:"active_app"`
}

// InactiveAppMonitor detects memory-heavy background processes that the user is not actively using
// while focused on another application, and emits a macOS notification + recommendation.
type InactiveAppMonitor struct {
	mu sync.Mutex

	InactivityThreshold time.Duration
	HighPressureThresh  time.Duration
	MinRSSMB            float64
	HighPressureMinRSS  float64
	MaxIdleCPUPct       float64
	ProcessCooldown     time.Duration
	GlobalCooldown      time.Duration
	MinObservedSamples  int

	firstSeen        map[int32]time.Time
	observedCount    map[int32]int
	notifiedProcess  map[string]time.Time
	lastNotification time.Time
	nowFn            func() time.Time
}

// NewInactiveAppMonitor creates a monitor with sensible, non-spammy macOS defaults.
func NewInactiveAppMonitor() *InactiveAppMonitor {
	return &InactiveAppMonitor{
		InactivityThreshold: 2 * time.Minute,
		HighPressureThresh:  45 * time.Second,
		MinRSSMB:            300.0,
		HighPressureMinRSS:  180.0,
		MaxIdleCPUPct:       5.0,
		ProcessCooldown:     15 * time.Minute,
		GlobalCooldown:      90 * time.Second,
		MinObservedSamples:  3,
		firstSeen:           make(map[int32]time.Time),
		observedCount:       make(map[int32]int),
		notifiedProcess:     make(map[string]time.Time),
		nowFn:               time.Now,
	}
}

// Evaluate inspects the current snapshot and active app context. When the user is actively
// using an app and another memory-heavy process has been idle in the background, it returns
// a recommendation and notification payload.
func (m *InactiveAppMonitor) Evaluate(
	snap engine.Snapshot,
	activeApp string,
	activeSince time.Time,
	lastActive map[string]time.Time,
) (*store.Recommendation, *InactiveAppNotification) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if strings.TrimSpace(activeApp) == "" || len(snap.TopProcesses) == 0 {
		return nil, nil
	}

	now := m.nowFn()
	if !m.lastNotification.IsZero() && now.Sub(m.lastNotification) < m.GlobalCooldown {
		return nil, nil
	}

	inactivityReq := m.InactivityThreshold
	minRSS := m.MinRSSMB
	if snap.Memory.PressurePct >= 70.0 {
		inactivityReq = m.HighPressureThresh
		minRSS = m.HighPressureMinRSS
	}

	// Track live PIDs so maps don't grow unboundedly.
	livePIDs := make(map[int32]bool, len(snap.TopProcesses))
	var candidates []engine.ProcessInfo

	for _, p := range snap.TopProcesses {
		livePIDs[p.PID] = true
		if _, ok := m.firstSeen[p.PID]; !ok {
			m.firstSeen[p.PID] = now
		}
		m.observedCount[p.PID]++

		if m.observedCount[p.PID] < m.MinObservedSamples {
			continue
		}
		if intelligence.IsProtectedProcess(p.PID, p.Name) || isSystemBackgroundDaemon(p.Name) {
			continue
		}
		if isProcessMatchingApp(p.Name, activeApp) {
			continue
		}
		rssMB := float64(p.MemoryBytes) / (1024 * 1024)
		if rssMB < minRSS || p.CPUPct >= m.MaxIdleCPUPct {
			continue
		}

		// Determine how long this process has been inactive relative to user focus.
		idleFor := m.processIdleDuration(now, p.Name, p.PID, activeSince, lastActive)
		if idleFor < inactivityReq {
			continue
		}

		procKey := fmt.Sprintf("%d:%s", p.PID, strings.ToLower(p.Name))
		if prev, ok := m.notifiedProcess[procKey]; ok && now.Sub(prev) < m.ProcessCooldown {
			continue
		}

		candidates = append(candidates, p)
	}

	// Prune exited PIDs.
	for pid := range m.firstSeen {
		if !livePIDs[pid] {
			delete(m.firstSeen, pid)
			delete(m.observedCount, pid)
		}
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	// Pick the heaviest inactive process by RSS.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].MemoryBytes > candidates[j].MemoryBytes
	})
	target := candidates[0]
	targetRSSMB := float64(target.MemoryBytes) / (1024 * 1024)

	procKey := fmt.Sprintf("%d:%s", target.PID, strings.ToLower(target.Name))
	m.notifiedProcess[procKey] = now
	m.lastNotification = now

	recID := fmt.Sprintf("inactive-mem-%d-%s", target.PID, sanitizeRecToken(target.Name))
	severity := "warning"
	if snap.Memory.PressurePct >= 75.0 {
		severity = "critical"
	}

	rec := &store.Recommendation{
		ID:         recID,
		Title:      fmt.Sprintf("Clean %s memory (%.0f MB idle)", target.Name, targetRSSMB),
		Detail:     fmt.Sprintf("You are actively using %s while %s is idle in the background (%.0f MB RAM, %.1f%% CPU). Clean/close %s to reclaim ~%.0f MB.", activeApp, target.Name, targetRSSMB, target.CPUPct, target.Name, targetRSSMB),
		Severity:   severity,
		Kind:       "inactive_app_memory",
		TargetPID:  target.PID,
		TargetName: target.Name,
		Status:     "pending",
	}

	notif := &InactiveAppNotification{
		ID:         fmt.Sprintf("notif-%d-%d", target.PID, now.UnixMilli()),
		Title:      fmt.Sprintf("Clean %s Memory (%.0f MB)", target.Name, targetRSSMB),
		Subtitle:   fmt.Sprintf("Active in %s • %s idle", activeApp, target.Name),
		Body:       fmt.Sprintf("%s is using %.0f MB RAM in the background (%.1f%% CPU) while you work in %s. Click Clean Memory to reclaim it.", target.Name, targetRSSMB, target.CPUPct, activeApp),
		RecID:      recID,
		TargetPID:  target.PID,
		TargetName: target.Name,
		RSSMB:      targetRSSMB,
		ActiveApp:  activeApp,
	}

	return rec, notif
}

func (m *InactiveAppMonitor) processIdleDuration(
	now time.Time,
	procName string,
	pid int32,
	activeSince time.Time,
	lastActive map[string]time.Time,
) time.Duration {
	procLower := strings.ToLower(strings.TrimSpace(procName))
	var matchedLast time.Time
	for appKey, ts := range lastActive {
		if appKey == "" {
			continue
		}
		if appKey == procLower || (len(appKey) >= 3 && strings.Contains(procLower, appKey)) || (len(procLower) >= 3 && strings.Contains(appKey, procLower)) {
			if ts.After(matchedLast) {
				matchedLast = ts
			}
		}
	}
	if !matchedLast.IsZero() {
		return now.Sub(matchedLast)
	}
	// Never focused during this session: use the shorter of time since activeApp became active and process firstSeen.
	first := m.firstSeen[pid]
	if !activeSince.IsZero() && activeSince.After(first) {
		return now.Sub(activeSince)
	}
	return now.Sub(first)
}

func isProcessMatchingApp(procName, activeApp string) bool {
	p := strings.ToLower(strings.TrimSpace(procName))
	a := strings.ToLower(strings.TrimSpace(activeApp))
	if p == "" || a == "" {
		return false
	}
	if p == a {
		return true
	}
	if len(a) >= 3 && strings.Contains(p, a) {
		return true
	}
	if len(p) >= 3 && strings.Contains(a, p) {
		return true
	}
	// Handle "Visual Studio Code" <-> "Code Helper"
	if strings.Contains(a, "visual studio code") && strings.Contains(p, "code") {
		return true
	}
	return false
}

func isSystemBackgroundDaemon(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "mds", "mds_stores", "mdworker", "mdworker_shared", "spotlight",
		"coreaudiod", "bluetoothd", "airportd", "trustd", "syspolicyd",
		"cfprefsd", "distnoted", "notifyd", "usbd", "diskarbitrationd",
		"powerd", "thermald", "logd", "syslogd", "configd", "usereventagent":
		return true
	}
	return false
}

func sanitizeRecToken(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "app"
	}
	return out
}
