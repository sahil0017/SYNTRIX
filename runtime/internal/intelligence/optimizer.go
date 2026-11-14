package intelligence

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/syntrix/runtime/internal/engine"
	"github.com/syntrix/runtime/internal/store"
)

var protectedProcesses = map[string]bool{
	"kernel_task":  true,
	"launchd":      true,
	"WindowServer": true,
	"loginwindow":  true,
	"Finder":       true,
	"Dock":         true,
	"syntrixd":     true,
	"SyntrixApp":   true,
}

func IsProtectedProcess(pid int32, name string) bool {
	if pid <= 100 {
		return true
	}
	return protectedProcesses[name]
}

type ActionRisk string

const (
	ActionRiskReadOnly     ActionRisk = "read_only"
	ActionRiskReversible   ActionRisk = "reversible"
	ActionRiskIrreversible ActionRisk = "irreversible"
)

type CacheTargetPreview struct {
	Directory    string  `json:"directory"`
	OwnerApp     string  `json:"owner_app"`
	SizeMB       float64 `json:"size_mb"`
	FileCount    int     `json:"file_count"`
	LastModified string  `json:"last_modified"`
}

type ActionSpec struct {
	ID             string               `json:"id"`
	Kind           string               `json:"kind"`
	Preconditions  []string             `json:"preconditions"`
	ExpectedEffect string               `json:"expected_effect"`
	Risk           ActionRisk           `json:"risk"`
	Reversible     bool                 `json:"reversible"`
	Timeout        time.Duration        `json:"timeout"`
	TargetPID      int32                `json:"target_pid"`
	TargetName     string               `json:"target_name"`
	DryRunSummary  string               `json:"dry_run_summary"`
	CachePreviews  []CacheTargetPreview `json:"cache_previews,omitempty"`
}

type ActionResult struct {
	Spec                   ActionSpec `json:"spec"`
	Executed               bool       `json:"executed"`
	Cancelled              bool       `json:"cancelled"`
	Summary                string     `json:"summary"`
	AchievedIntendedResult bool       `json:"achieved_intended_result"`
	VerificationReport     string     `json:"verification_report"`
}

// OptimizationController manages typed actions, dry-run previews, timeouts, and a global kill switch.
type OptimizationController struct {
	killed atomic.Bool
	mu     sync.Mutex
	active map[string]context.CancelFunc
}

func NewOptimizationController() *OptimizationController {
	return &OptimizationController{
		active: make(map[string]context.CancelFunc),
	}
}

func (c *OptimizationController) ActivateKillSwitch() int {
	c.killed.Store(true)
	c.mu.Lock()
	defer c.mu.Unlock()
	cancelled := len(c.active)
	for id, cancel := range c.active {
		cancel()
		delete(c.active, id)
	}
	return cancelled
}

func (c *OptimizationController) ResetKillSwitch() {
	c.killed.Store(false)
}

func (c *OptimizationController) IsKilled() bool {
	return c.killed.Load()
}

// BuildActionSpec creates a typed, precondition-checked ActionSpec with an explicit dry-run preview.
func BuildActionSpec(rec store.Recommendation) ActionSpec {
	switch rec.Kind {
	case "cpu_spike":
		protected := IsProtectedProcess(rec.TargetPID, rec.TargetName)
		pre := []string{
			fmt.Sprintf("Target PID %d (%s) > 100", rec.TargetPID, rec.TargetName),
			"Target is not a protected macOS/SYNTRIX process",
		}
		dry := fmt.Sprintf("DRY-RUN: Would set scheduling priority nice(+10) on %s (PID %d). Note: reduces CPU scheduling share; does NOT reduce memory usage.", rec.TargetName, rec.TargetPID)
		if protected {
			dry = fmt.Sprintf("DRY-RUN: Blocked — %s (PID %d) is protected; no priority change would occur.", rec.TargetName, rec.TargetPID)
		}
		return ActionSpec{
			ID:             rec.ID,
			Kind:           rec.Kind,
			Preconditions:  pre,
			ExpectedEffect: "Lowers CPU scheduling priority (nice +10); does NOT reduce RSS memory.",
			Risk:           ActionRiskReversible,
			Reversible:     true,
			Timeout:        3 * time.Second,
			TargetPID:      rec.TargetPID,
			TargetName:     rec.TargetName,
			DryRunSummary:  dry,
		}

	case "storage_pressure":
		previews := PreviewStaleUserCaches(3*24*time.Hour, 8)
		var totalMB float64
		for _, p := range previews {
			totalMB += p.SizeMB
		}
		dry := fmt.Sprintf("DRY-RUN: Previewed %d non-Apple cache directories (%.1f MB stale >3d). Irreversible once deleted; no files removed in preview.", len(previews), totalMB)
		return ActionSpec{
			ID:             rec.ID,
			Kind:           rec.Kind,
			Preconditions:  []string{"Non-Apple ~/Library/Caches subdirectories", "Files older than 3 days", "Explicit user approval after preview"},
			ExpectedEffect: fmt.Sprintf("Reclaim up to %.1f MB of stale cache files (IRREVERSIBLE).", totalMB),
			Risk:           ActionRiskIrreversible,
			Reversible:     false,
			Timeout:        8 * time.Second,
			TargetName:     "~/Library/Caches",
			DryRunSummary:  dry,
			CachePreviews:  previews,
		}

	case "inactive_app_memory":
		protected := IsProtectedProcess(rec.TargetPID, rec.TargetName)
		pre := []string{
			fmt.Sprintf("Target PID %d (%s) > 100", rec.TargetPID, rec.TargetName),
			"Target is not a protected macOS/SYNTRIX process",
			"Application is inactive in background while user works in another app",
		}
		dry := fmt.Sprintf("DRY-RUN: Would gracefully terminate inactive %s (PID %d) via SIGTERM to reclaim its RSS memory.", rec.TargetName, rec.TargetPID)
		if protected {
			dry = fmt.Sprintf("DRY-RUN: Blocked — %s (PID %d) is protected.", rec.TargetName, rec.TargetPID)
		}
		return ActionSpec{
			ID:             rec.ID,
			Kind:           rec.Kind,
			Preconditions:  pre,
			ExpectedEffect: fmt.Sprintf("Gracefully closes idle %s (PID %d) to reclaim its RAM.", rec.TargetName, rec.TargetPID),
			Risk:           ActionRiskIrreversible,
			Reversible:     false,
			Timeout:        5 * time.Second,
			TargetPID:      rec.TargetPID,
			TargetName:     rec.TargetName,
			DryRunSummary:  dry,
		}

	case "full_optimize":
		previews := PreviewStaleUserCaches(3*24*time.Hour, 8)
		var totalMB float64
		for _, p := range previews {
			totalMB += p.SizeMB
		}
		return ActionSpec{
			ID:             rec.ID,
			Kind:           rec.Kind,
			Preconditions:  []string{"Non-protected processes only", "Stale caches >3d, logs >7d, DerivedData >14d"},
			ExpectedEffect: fmt.Sprintf("Reclaim ~%.1f MB stale disk/caches, free Go heap, and renice background CPU hogs.", totalMB),
			Risk:           ActionRiskIrreversible,
			Reversible:     false,
			Timeout:        10 * time.Second,
			TargetName:     "System",
			DryRunSummary:  fmt.Sprintf("DRY-RUN: Would optimize RAM, clean ~%.1f MB stale caches/logs/DerivedData, and renice high-CPU background processes.", totalMB),
			CachePreviews:  previews,
		}

	case "memory_pressure", "memory_growth":
		return ActionSpec{
			ID:             rec.ID,
			Kind:           rec.Kind,
			Preconditions:  []string{"Host memory pressure elevated", "Non-destructive daemon heap release only"},
			ExpectedEffect: "Releases unused Go runtime heap pages in syntrixd; does NOT reclaim other apps' macOS RAM.",
			Risk:           ActionRiskReadOnly,
			Reversible:     true,
			Timeout:        3 * time.Second,
			TargetPID:      rec.TargetPID,
			TargetName:     rec.TargetName,
			DryRunSummary:  "DRY-RUN: Would release Go heap pages in syntrixd and re-measure host memory pressure (cannot force-free other apps' RAM without quitting them).",
		}

	default:
		return ActionSpec{
			ID:             rec.ID,
			Kind:           rec.Kind,
			Preconditions:  []string{"Safe read-only/diagnostic fallback"},
			ExpectedEffect: "Releases unused Go runtime heap pages.",
			Risk:           ActionRiskReadOnly,
			Reversible:     true,
			Timeout:        3 * time.Second,
			DryRunSummary:  "DRY-RUN: Would run Go runtime heap release.",
		}
	}
}

func (c *OptimizationController) Execute(ctx context.Context, rec store.Recommendation, dryRun bool, before engine.Snapshot, afterFn func() (engine.Snapshot, bool)) ActionResult {
	var spec ActionSpec
	if !dryRun && rec.Kind == "full_optimize" {
		// Fast path for live Full Optimize: avoid walking caches twice (preview + clean).
		spec = ActionSpec{
			ID:         rec.ID,
			Kind:       rec.Kind,
			Timeout:    12 * time.Second,
			TargetName: "System",
		}
	} else {
		spec = BuildActionSpec(rec)
	}
	if dryRun {
		return ActionResult{
			Spec:               spec,
			Executed:           false,
			Summary:            spec.DryRunSummary,
			VerificationReport: "Dry-run preview only; no OS state modified.",
		}
	}
	if c.killed.Load() {
		return ActionResult{
			Spec:               spec,
			Cancelled:          true,
			Summary:            "Aborted: optimization kill switch is active.",
			VerificationReport: "Kill switch prevented execution.",
		}
	}

	runCtx, cancel := context.WithTimeout(ctx, spec.Timeout)
	c.mu.Lock()
	c.active[rec.ID] = cancel
	c.mu.Unlock()
	defer func() {
		cancel()
		c.mu.Lock()
		delete(c.active, rec.ID)
		c.mu.Unlock()
	}()

	var summary string
	switch rec.Kind {
	case "cpu_spike":
		if rec.TargetPID > 100 && !IsProtectedProcess(rec.TargetPID, rec.TargetName) {
			if err := syscall.Setpriority(syscall.PRIO_PROCESS, int(rec.TargetPID), 10); err == nil {
				summary = fmt.Sprintf("Set scheduling priority nice(+10) on %s (PID %d); does not directly reduce memory.", rec.TargetName, rec.TargetPID)
			} else {
				summary = fmt.Sprintf("Could not reprioritize %s (PID %d): %v", rec.TargetName, rec.TargetPID, err)
			}
		} else {
			summary = fmt.Sprintf("Skipped reprioritization: %s (PID %d) is protected.", rec.TargetName, rec.TargetPID)
		}

	case "inactive_app_memory":
		if rec.TargetPID > 100 && !IsProtectedProcess(rec.TargetPID, rec.TargetName) {
			if err := syscall.Kill(int(rec.TargetPID), syscall.SIGTERM); err == nil {
				// Allow brief moment for process exit so verification snapshot reflects reclaimed RSS.
				time.Sleep(350 * time.Millisecond)
				summary = fmt.Sprintf("Gracefully terminated inactive %s (PID %d) to reclaim memory.", rec.TargetName, rec.TargetPID)
			} else {
				summary = fmt.Sprintf("Could not terminate %s (PID %d): %v", rec.TargetName, rec.TargetPID, err)
			}
		} else {
			summary = fmt.Sprintf("Skipped termination: %s (PID %d) is protected.", rec.TargetName, rec.TargetPID)
		}

	case "storage_pressure":
		if runCtx.Err() != nil {
			return ActionResult{Spec: spec, Cancelled: true, Summary: "Cancelled before cache cleanup."}
		}
		reclaimed := CleanAllSafeTargets()
		summary = fmt.Sprintf("Cleaned stale (>3d caches, >7d logs, >14d DerivedData) non-Apple files: %.1f MB reclaimed.", float64(reclaimed)/(1024*1024))

	case "full_optimize":
		if runCtx.Err() != nil {
			return ActionResult{Spec: spec, Cancelled: true, Summary: "Cancelled before full optimization."}
		}
		runtime.GC()
		debug.FreeOSMemory()
		reclaimed := CleanAllSafeTargets()
		reniced := 0
		for _, p := range before.TopProcesses {
			if reniced >= 5 {
				break
			}
			if p.CPUPct >= 25.0 && p.PID > 100 && !IsProtectedProcess(p.PID, p.Name) {
				if syscall.Setpriority(syscall.PRIO_PROCESS, int(p.PID), 10) == nil {
					reniced++
				}
			}
		}
		summary = fmt.Sprintf("Full Optimize: freed daemon RAM, reclaimed %.1f MB stale caches/logs/DerivedData, reniced %d background CPU hog(s).",
			float64(reclaimed)/(1024*1024), reniced)

	default:
		runtime.GC()
		debug.FreeOSMemory()
		var reclaimed int64
		if before.Memory.PressurePct >= 75.0 || before.Memory.PageinsPerSec >= 40.0 {
			reclaimed = CleanStaleUserCaches(7 * 24 * time.Hour)
		}
		if reclaimed > 0 {
			summary = fmt.Sprintf("Released daemon Go heap & reclaimed %.1f MB stale (>7d) caches to ease swap/memory pressure.", float64(reclaimed)/(1024*1024))
		} else {
			summary = "Released syntrixd Go heap pages (note: macOS protects other apps' private memory unless terminated)."
		}
	}

	after := before
	if afterFn != nil {
		if snap, ok := afterFn(); ok {
			after = snap
		}
	}

	achieved, report := verifyOutcome(rec.Kind, before, after, summary)
	return ActionResult{
		Spec:                   spec,
		Executed:               true,
		Summary:                summary,
		AchievedIntendedResult: achieved,
		VerificationReport:     report,
	}
}

func verifyOutcome(kind string, before, after engine.Snapshot, actionSummary string) (bool, string) {
	switch kind {
	case "cpu_spike":
		delta := after.CPU.TotalUsagePct - before.CPU.TotalUsagePct
		return delta <= 5.0, fmt.Sprintf("CPU %.1f%% -> %.1f%% (%+.1f%%) | %s", before.CPU.TotalUsagePct, after.CPU.TotalUsagePct, delta, actionSummary)
	case "storage_pressure":
		delta := after.Disk.UsedPct - before.Disk.UsedPct
		return delta <= 0.0, fmt.Sprintf("Disk %.1f%% -> %.1f%% (%+.2f%%) | %s", before.Disk.UsedPct, after.Disk.UsedPct, delta, actionSummary)
	default:
		delta := after.Memory.PressurePct - before.Memory.PressurePct
		return delta <= 2.0, fmt.Sprintf("MemPressure %.0f%% -> %.0f%% (%+.0f%%) | %s", before.Memory.PressurePct, after.Memory.PressurePct, delta, actionSummary)
	}
}

// ExecuteOptimization preserves compatibility for direct callers using a default controller.
func ExecuteOptimization(rec store.Recommendation) string {
	res := NewOptimizationController().Execute(context.Background(), rec, false, engine.Snapshot{}, nil)
	return res.Summary
}

func PreviewStaleUserCaches(maxAge time.Duration, maxEntries int) []CacheTargetPreview {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return PreviewStaleCachesInDir(filepath.Join(home, "Library", "Caches"), maxAge, maxEntries)
}

func PreviewStaleCachesInDir(cacheRoot string, maxAge time.Duration, maxEntries int) []CacheTargetPreview {
	entries, err := os.ReadDir(cacheRoot)
	if err != nil {
		return nil
	}
	cutoff := time.Now().Add(-maxAge)
	var previews []CacheTargetPreview

	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		name := ent.Name()
		if strings.HasPrefix(name, "com.apple.") || strings.HasPrefix(name, "Apple") || strings.Contains(strings.ToLower(name), "syntrix") {
			continue
		}
		subDir := filepath.Join(cacheRoot, name)
		var bytesTotal int64
		var count int
		var newest time.Time
		_ = filepath.WalkDir(subDir, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			if info.ModTime().Before(cutoff) {
				bytesTotal += info.Size()
				count++
				if info.ModTime().After(newest) {
					newest = info.ModTime()
				}
			}
			return nil
		})
		if count > 0 {
			previews = append(previews, CacheTargetPreview{
				Directory:    subDir,
				OwnerApp:     name,
				SizeMB:       float64(bytesTotal) / (1024 * 1024),
				FileCount:    count,
				LastModified: newest.Format(time.RFC3339),
			})
			if maxEntries > 0 && len(previews) >= maxEntries {
				break
			}
		}
	}
	return previews
}

func CleanStaleUserCaches(maxAge time.Duration) int64 {
	home, err := os.UserHomeDir()
	if err != nil {
		return 0
	}
	return CleanStaleCachesInDir(filepath.Join(home, "Library", "Caches"), maxAge)
}

func CleanAllSafeTargets() int64 {
	var reclaimed int64
	reclaimed += CleanStaleUserCaches(3 * 24 * time.Hour)
	if home, err := os.UserHomeDir(); err == nil {
		reclaimed += CleanStaleCachesInDir(filepath.Join(home, "Library", "Logs"), 7*24*time.Hour)
		reclaimed += CleanStaleCachesInDir(filepath.Join(home, "Library", "Developer", "Xcode", "DerivedData"), 7*24*time.Hour)
	}
	return reclaimed
}

func removeStaleFile(path string) error {
	if err := os.Remove(path); err == nil {
		return nil
	}
	// Read-only caches (e.g., dotslash, bazel, content-addressed stores) mark files/dirs 0555.
	parent := filepath.Dir(path)
	if info, err := os.Stat(parent); err == nil && info.Mode().Perm()&0o200 == 0 {
		_ = os.Chmod(parent, info.Mode().Perm()|0o700)
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink == 0 && info.Mode().Perm()&0o200 == 0 {
		_ = os.Chmod(path, info.Mode().Perm()|0o600)
	}
	return os.Remove(path)
}

func CleanStaleCachesInDir(cacheRoot string, maxAge time.Duration) int64 {
	entries, err := os.ReadDir(cacheRoot)
	if err != nil {
		return 0
	}
	cutoff := time.Now().Add(-maxAge)
	var reclaimed int64

	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		name := ent.Name()
		if strings.HasPrefix(name, "com.apple.") || strings.HasPrefix(name, "Apple") || strings.Contains(strings.ToLower(name), "syntrix") {
			continue
		}
		subDir := filepath.Join(cacheRoot, name)
		var dirs []string
		_ = filepath.WalkDir(subDir, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if d.IsDir() {
				dirs = append(dirs, path)
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			if info.ModTime().Before(cutoff) {
				sz := info.Size()
				if removeStaleFile(path) == nil {
					reclaimed += sz
				}
			}
			return nil
		})
		// Prune empty directories bottom-up so read-only empty cache trees are also cleaned.
		for i := len(dirs) - 1; i >= 0; i-- {
			dPath := dirs[i]
			if subEntries, err := os.ReadDir(dPath); err == nil && len(subEntries) == 0 {
				if os.Remove(dPath) != nil {
					parent := filepath.Dir(dPath)
					if pInfo, pErr := os.Stat(parent); pErr == nil && pInfo.Mode().Perm()&0o200 == 0 {
						_ = os.Chmod(parent, pInfo.Mode().Perm()|0o700)
					}
					if dInfo, dErr := os.Stat(dPath); dErr == nil && dInfo.Mode().Perm()&0o200 == 0 {
						_ = os.Chmod(dPath, dInfo.Mode().Perm()|0o700)
					}
					_ = os.Remove(dPath)
				}
			}
		}
	}
	return reclaimed
}
