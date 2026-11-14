package intelligence

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/syntrix/runtime/internal/engine"
)

const DefaultStorageLimitPct = 50.0

// StorageCleanupCandidate describes an unused or stale file/directory that can be removed to free disk space.
type StorageCleanupCandidate struct {
	Path         string  `json:"path"`
	Category     string  `json:"category"`
	Owner        string  `json:"owner"`
	SizeMB       float64 `json:"size_mb"`
	FileCount    int     `json:"file_count"`
	AgeDays      int     `json:"age_days"`
	SafeToAuto   bool    `json:"safe_to_auto"`
	InUseByOwner bool    `json:"in_use_by_owner"`
	CleanupScore float64 `json:"cleanup_score"`
	Reason       string  `json:"reason"`
}

// StorageAuditReport summarizes disk utilization against the 50% ceiling and lists unused reclaimable items.
type StorageAuditReport struct {
	UsedPct       float64                   `json:"used_pct"`
	ThresholdPct  float64                   `json:"threshold_pct"`
	ExceedsLimit  bool                      `json:"exceeds_limit"`
	FreeGB        float64                   `json:"free_gb"`
	TotalGB       float64                   `json:"total_gb"`
	ReclaimableMB float64                   `json:"reclaimable_mb"`
	ModelName     string                    `json:"model_name"`
	Summary       string                    `json:"summary"`
	ScannedAtMs   int64                     `json:"scanned_at_ms"`
	Candidates    []StorageCleanupCandidate `json:"candidates,omitempty"`
}

// StorageMonitor enforces a 50% disk ceiling and caches periodic scans of unused/stale files.
type StorageMonitor struct {
	mu           sync.Mutex
	LimitPct     float64
	ScanInterval time.Duration
	lastScanAt   time.Time
	cachedReport StorageAuditReport
	homeDir      string
	nowFn        func() time.Time
}

func NewStorageMonitor() *StorageMonitor {
	home, _ := os.UserHomeDir()
	return &StorageMonitor{
		LimitPct:     DefaultStorageLimitPct,
		ScanInterval: 30 * time.Second,
		homeDir:      home,
		nowFn:        time.Now,
	}
}

func (m *StorageMonitor) Invalidate() {
	m.mu.Lock()
	m.lastScanAt = time.Time{}
	m.mu.Unlock()
}

// Evaluate returns the current StorageAuditReport, refreshing the filesystem scan when needed.
func (m *StorageMonitor) Evaluate(snap engine.Snapshot) StorageAuditReport {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.nowFn()
	exceeds := snap.Disk.UsedPct > m.LimitPct
	freeGB := float64(snap.Disk.FreeBytes) / (1024 * 1024 * 1024)
	totalGB := float64(snap.Disk.TotalBytes) / (1024 * 1024 * 1024)

	needScan := m.lastScanAt.IsZero() ||
		now.Sub(m.lastScanAt) >= m.ScanInterval ||
		(exceeds && !m.cachedReport.ExceedsLimit)

	if !needScan {
		rep := m.cachedReport
		rep.UsedPct = snap.Disk.UsedPct
		rep.ExceedsLimit = exceeds
		rep.FreeGB = freeGB
		rep.TotalGB = totalGB
		rep.Summary = formatStorageSummary(rep)
		return rep
	}

	candidates := m.scanUnusedLocked(now, snap.TopProcesses)
	var reclaimableMB float64
	for _, c := range candidates {
		reclaimableMB += c.SizeMB
	}
	if len(candidates) > 10 {
		candidates = candidates[:10]
	}

	rep := StorageAuditReport{
		UsedPct:       snap.Disk.UsedPct,
		ThresholdPct:  m.LimitPct,
		ExceedsLimit:  exceeds,
		FreeGB:        freeGB,
		TotalGB:       totalGB,
		ReclaimableMB: reclaimableMB,
		ModelName:     "LogisticStaleClassifier-v1",
		ScannedAtMs:   now.UnixMilli(),
		Candidates:    candidates,
	}
	rep.Summary = formatStorageSummary(rep)
	m.cachedReport = rep
	m.lastScanAt = now
	return rep
}

func formatStorageSummary(rep StorageAuditReport) string {
	if rep.ExceedsLimit {
		if rep.ReclaimableMB >= 1024 {
			return fmt.Sprintf("Storage %.1f%% exceeds %.0f%% limit — %.2f GB unused/stale found (%d targets)",
				rep.UsedPct, rep.ThresholdPct, rep.ReclaimableMB/1024.0, len(rep.Candidates))
		}
		if rep.ReclaimableMB > 0 {
			return fmt.Sprintf("Storage %.1f%% exceeds %.0f%% limit — %.0f MB unused/stale found (%d targets)",
				rep.UsedPct, rep.ThresholdPct, rep.ReclaimableMB, len(rep.Candidates))
		}
		return fmt.Sprintf("Storage %.1f%% exceeds %.0f%% limit — no stale user caches >3d found", rep.UsedPct, rep.ThresholdPct)
	}
	return fmt.Sprintf("Storage %.1f%% within %.0f%% limit (%.1f GB free)", rep.UsedPct, rep.ThresholdPct, rep.FreeGB)
}

func (m *StorageMonitor) scanUnusedLocked(now time.Time, procs []engine.ProcessInfo) []StorageCleanupCandidate {
	if m.homeDir == "" {
		return nil
	}

	var raw []StorageCleanupCandidate
	raw = append(raw, scanDirEntriesByAge(filepath.Join(m.homeDir, "Library", "Caches"), "stale_cache", now.Add(-3*24*time.Hour), true, "Unused app cache >3 days", now)...)
	raw = append(raw, scanDirEntriesByAge(filepath.Join(m.homeDir, "Library", "Logs"), "stale_logs", now.Add(-7*24*time.Hour), true, "Old app logs >7 days", now)...)
	raw = append(raw, scanDirEntriesByAge(filepath.Join(m.homeDir, "Library", "Developer", "Xcode", "DerivedData"), "xcode_derived", now.Add(-7*24*time.Hour), false, "Stale Xcode DerivedData >7 days", now)...)
	raw = append(raw, scanOldDownloadInstallers(filepath.Join(m.homeDir, "Downloads"), now.Add(-14*24*time.Hour), 50.0, now)...)
	raw = append(raw, scanTrash(filepath.Join(m.homeDir, ".Trash"), now)...)

	var scored []StorageCleanupCandidate
	for _, c := range raw {
		score, inUse, reason := scoreCleanupCandidate(c, procs)
		c.CleanupScore = score
		c.InUseByOwner = inUse
		c.Reason = reason
		if inUse {
			c.SafeToAuto = false
		}
		if score >= 0.55 {
			scored = append(scored, c)
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		ui := scored[i].CleanupScore * math.Log1p(scored[i].SizeMB)
		uj := scored[j].CleanupScore * math.Log1p(scored[j].SizeMB)
		return ui > uj
	})
	return scored
}

// scoreCleanupCandidate applies a logistic multi-feature utility classifier over
// file age (atime+mtime), log-size, category safety prior, and live process ownership.
func scoreCleanupCandidate(c StorageCleanupCandidate, procs []engine.ProcessInfo) (float64, bool, string) {
	ownerLower := strings.ToLower(strings.TrimSpace(c.Owner))
	inUse := false
	for _, p := range procs {
		pName := strings.ToLower(strings.TrimSpace(p.Name))
		if pName == "" || len(pName) < 3 {
			continue
		}
		if strings.Contains(ownerLower, pName) || strings.Contains(pName, ownerLower) {
			inUse = true
			break
		}
	}

	xAge := math.Min(1.0, float64(c.AgeDays)/30.0)
	xSize := math.Min(1.0, math.Log1p(c.SizeMB)/7.0)
	var catPrior float64
	switch c.Category {
	case "stale_cache":
		catPrior = 1.2
	case "stale_logs":
		catPrior = 1.1
	case "trash":
		catPrior = 0.9
	case "xcode_derived":
		catPrior = 0.6
	default:
		catPrior = 0.5
	}
	xInUse := 0.0
	if inUse {
		xInUse = 1.0
	}

	z := -1.1 + 2.2*xAge + 0.9*xSize + catPrior - 2.4*xInUse
	score := math.Round((1.0/(1.0+math.Exp(-z)))*100.0) / 100.0

	ownerState := "owner idle"
	if inUse {
		ownerState = "owner running (protected)"
	}
	reason := fmt.Sprintf("Score %.2f (LogisticStaleClassifier): %dd untouched, %.0f MB, %s, %s",
		score, c.AgeDays, c.SizeMB, ownerState, c.Category)
	return score, inUse, reason
}

func lastTouchedTime(info os.FileInfo) time.Time {
	t := info.ModTime()
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		atime := time.Unix(st.Atimespec.Sec, st.Atimespec.Nsec)
		if atime.After(t) {
			t = atime
		}
	}
	return t
}

func scanDirEntriesByAge(root, category string, cutoff time.Time, safeToAuto bool, reason string, now time.Time) []StorageCleanupCandidate {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []StorageCleanupCandidate
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		name := ent.Name()
		if strings.HasPrefix(name, "com.apple.") || strings.HasPrefix(name, "Apple") || strings.Contains(strings.ToLower(name), "syntrix") {
			continue
		}
		subDir := filepath.Join(root, name)
		var totalBytes int64
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
			touched := lastTouchedTime(info)
			if touched.Before(cutoff) {
				totalBytes += info.Size()
				count++
				if touched.After(newest) {
					newest = touched
				}
			}
			return nil
		})
		sizeMB := float64(totalBytes) / (1024 * 1024)
		if count > 0 && sizeMB >= 5.0 {
			ageDays := int(now.Sub(newest).Hours() / 24)
			out = append(out, StorageCleanupCandidate{
				Path:       subDir,
				Category:   category,
				Owner:      name,
				SizeMB:     sizeMB,
				FileCount:  count,
				AgeDays:    ageDays,
				SafeToAuto: safeToAuto,
				Reason:     reason,
			})
		}
	}
	return out
}

func scanOldDownloadInstallers(downloadsDir string, cutoff time.Time, minMB float64, now time.Time) []StorageCleanupCandidate {
	entries, err := os.ReadDir(downloadsDir)
	if err != nil {
		return nil
	}
	var out []StorageCleanupCandidate
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		lower := strings.ToLower(ent.Name())
		if !strings.HasSuffix(lower, ".dmg") && !strings.HasSuffix(lower, ".pkg") &&
			!strings.HasSuffix(lower, ".iso") && !strings.HasSuffix(lower, ".zip") &&
			!strings.HasSuffix(lower, ".tar.gz") {
			continue
		}
		info, err := ent.Info()
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		sizeMB := float64(info.Size()) / (1024 * 1024)
		if sizeMB < minMB {
			continue
		}
		out = append(out, StorageCleanupCandidate{
			Path:       filepath.Join(downloadsDir, ent.Name()),
			Category:   "unused_installer",
			Owner:      ent.Name(),
			SizeMB:     sizeMB,
			FileCount:  1,
			AgeDays:    int(now.Sub(info.ModTime()).Hours() / 24),
			SafeToAuto: false,
			Reason:     "Unused installer/archive in Downloads (>14 days)",
		})
	}
	return out
}

func scanTrash(trashDir string, now time.Time) []StorageCleanupCandidate {
	entries, err := os.ReadDir(trashDir)
	if err != nil || len(entries) == 0 {
		return nil
	}
	var totalBytes int64
	var count int
	var oldest time.Time
	_ = filepath.WalkDir(trashDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		totalBytes += info.Size()
		count++
		if oldest.IsZero() || info.ModTime().Before(oldest) {
			oldest = info.ModTime()
		}
		return nil
	})
	sizeMB := float64(totalBytes) / (1024 * 1024)
	if sizeMB < 10.0 {
		return nil
	}
	ageDays := 0
	if !oldest.IsZero() {
		ageDays = int(now.Sub(oldest).Hours() / 24)
	}
	return []StorageCleanupCandidate{{
		Path:       trashDir,
		Category:   "trash",
		Owner:      "Trash",
		SizeMB:     sizeMB,
		FileCount:  count,
		AgeDays:    ageDays,
		SafeToAuto: false,
		Reason:     "Items sitting in ~/.Trash",
	}}
}
