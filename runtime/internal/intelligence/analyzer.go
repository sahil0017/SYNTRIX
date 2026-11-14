package intelligence

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/syntrix/runtime/internal/engine"
	"github.com/syntrix/runtime/internal/store"
)

const maxHistory = 60

type CacheCandidate struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	SizeBytes uint64 `json:"size_bytes"`
}

type Analyzer struct {
	mu             sync.Mutex
	history        []engine.Snapshot
	pidMem         map[int32][]uint64
	pidHighCPU     map[int32]int
	pidIOThrash    map[int32]int
	pidWakeupDrain map[int32]int
	ssdSatStreak   int
	gpuSatStreak   int
}

func NewAnalyzer() *Analyzer {
	return &Analyzer{
		history:        make([]engine.Snapshot, 0, maxHistory),
		pidMem:         make(map[int32][]uint64),
		pidHighCPU:     make(map[int32]int),
		pidIOThrash:    make(map[int32]int),
		pidWakeupDrain: make(map[int32]int),
	}
}

func (a *Analyzer) Ingest(snap engine.Snapshot) []store.Recommendation {
	a.mu.Lock()
	defer a.mu.Unlock()

	if len(a.history) >= maxHistory {
		copy(a.history, a.history[1:])
		a.history[len(a.history)-1] = snap
	} else {
		a.history = append(a.history, snap)
	}

	seen := make(map[int32]bool, len(snap.TopProcesses))
	for _, p := range snap.TopProcesses {
		seen[p.PID] = true
		series := append(a.pidMem[p.PID], p.MemoryBytes)
		if len(series) > 12 {
			series = series[len(series)-12:]
		}
		a.pidMem[p.PID] = series

		if p.CPUPct >= 85.0 || (p.Threads >= 80 && p.CPUPct >= 60.0) {
			a.pidHighCPU[p.PID]++
		} else {
			a.pidHighCPU[p.PID] = 0
		}

		ioTotal := p.DiskReadBps + p.DiskWriteBps
		if p.PageinsPerSec >= 40.0 || ioTotal >= 50*1024*1024 {
			a.pidIOThrash[p.PID]++
		} else {
			a.pidIOThrash[p.PID] = 0
		}

		if p.WakeupsPerSec >= 250.0 {
			a.pidWakeupDrain[p.PID]++
		} else {
			a.pidWakeupDrain[p.PID] = 0
		}
	}
	for pid := range a.pidMem {
		if !seen[pid] {
			delete(a.pidMem, pid)
			delete(a.pidHighCPU, pid)
			delete(a.pidIOThrash, pid)
			delete(a.pidWakeupDrain, pid)
		}
	}

	return a.detectLocked(snap)
}

func (a *Analyzer) detectLocked(snap engine.Snapshot) []store.Recommendation {
	var recs []store.Recommendation

	// 1. Memory pressure & swap detection
	if snap.Memory.PressureLevel == engine.PressureCritical || snap.Memory.PressurePct >= 80.0 {
		topName := "unknown"
		var topPID int32
		var topMem uint64
		for _, p := range snap.TopProcesses {
			if p.MemoryBytes > topMem {
				topMem = p.MemoryBytes
				topName = p.Name
				topPID = p.PID
			}
		}
		recs = append(recs, store.Recommendation{
			ID:           "mem-pressure-high",
			CreatedAtMs:  snap.TimestampMs,
			Kind:         "memory_pressure",
			Severity:     "high",
			Title:        fmt.Sprintf("Elevated memory pressure (%.0f%%)", snap.Memory.PressurePct),
			Detail:       fmt.Sprintf("Largest observed consumer is %s (PID %d, %.0f MB). Review or close unused tabs/apps.", topName, topPID, float64(topMem)/(1024*1024)),
			TargetPID:    topPID,
			TargetName:   topName,
			Status:       "pending",
			BeforeMetric: fmt.Sprintf("pressure=%.1f%% swap=%.0fMB", snap.Memory.PressurePct, float64(snap.Memory.SwapUsedBytes)/(1024*1024)),
		})
	}

	// 2. High CPU spike detection
	if snap.CPU.TotalUsagePct >= 80.0 && len(snap.TopProcesses) > 0 {
		top := snap.TopProcesses[0]
		recs = append(recs, store.Recommendation{
			ID:           fmt.Sprintf("cpu-spike-%d", top.PID),
			CreatedAtMs:  snap.TimestampMs,
			Kind:         "cpu_spike",
			Severity:     "medium",
			Title:        fmt.Sprintf("High CPU load (%.1f%%) led by %s", snap.CPU.TotalUsagePct, top.Name),
			Detail:       fmt.Sprintf("%s (PID %d) is using %.1f%% CPU across %d threads.", top.Name, top.PID, top.CPUPct, top.Threads),
			TargetPID:    top.PID,
			TargetName:   top.Name,
			Status:       "pending",
			BeforeMetric: fmt.Sprintf("cpu=%.1f%% proc_cpu=%.1f%%", snap.CPU.TotalUsagePct, top.CPUPct),
		})
	}

	// 3. Memory growth / leak-like trend detection (>= 50 MB growth across >= 6 samples)
	for _, p := range snap.TopProcesses {
		series := a.pidMem[p.PID]
		if len(series) < 6 {
			continue
		}
		first := series[0]
		last := series[len(series)-1]
		if last > first+50*1024*1024 && isMostlyIncreasing(series) {
			growthMB := float64(last-first) / (1024 * 1024)
			recs = append(recs, store.Recommendation{
				ID:           fmt.Sprintf("mem-growth-%d", p.PID),
				CreatedAtMs:  snap.TimestampMs,
				Kind:         "memory_growth",
				Severity:     "medium",
				Title:        fmt.Sprintf("Sustained memory growth in %s (+%.0f MB)", p.Name, growthMB),
				Detail:       fmt.Sprintf("%s (PID %d) grew from %.0f MB to %.0f MB over recent samples.", p.Name, p.PID, float64(first)/(1024*1024), float64(last)/(1024*1024)),
				TargetPID:    p.PID,
				TargetName:   p.Name,
				Status:       "pending",
				BeforeMetric: fmt.Sprintf("rss=%.0fMB", float64(last)/(1024*1024)),
			})
		}
	}

	// 3b. Generic sustained single-process CPU spin / UI stall detection
	for _, p := range snap.TopProcesses {
		streak := a.pidHighCPU[p.PID]
		if streak >= 6 {
			recs = append(recs, store.Recommendation{
				ID:           fmt.Sprintf("proc-stall-%d", p.PID),
				CreatedAtMs:  snap.TimestampMs,
				Kind:         "cpu_spike",
				Severity:     "high",
				Title:        fmt.Sprintf("Possible hang/spin in %s (PID %d)", p.Name, p.PID),
				Detail:       fmt.Sprintf("%s sustained %.1f%% CPU across %d threads (%.0f MB RSS) for %d samples — typical of a frozen tab, busy-wait loop, or stalled thread.", p.Name, p.CPUPct, p.Threads, float64(p.MemoryBytes)/(1024*1024), streak),
				TargetPID:    p.PID,
				TargetName:   p.Name,
				Status:       "pending",
				BeforeMetric: fmt.Sprintf("proc_cpu=%.1f%% threads=%d rss=%.0fMB", p.CPUPct, p.Threads, float64(p.MemoryBytes)/(1024*1024)),
			})
		}

		if a.pidIOThrash[p.PID] >= 4 {
			footprintMB := float64(p.PhysFootprintBytes) / (1024 * 1024)
			if footprintMB == 0 {
				footprintMB = float64(p.MemoryBytes) / (1024 * 1024)
			}
			ioMBs := (p.DiskReadBps + p.DiskWriteBps) / (1024 * 1024)
			recs = append(recs, store.Recommendation{
				ID:           fmt.Sprintf("proc-io-thrash-%d", p.PID),
				CreatedAtMs:  snap.TimestampMs,
				Kind:         "memory_pressure",
				Severity:     "high",
				Title:        fmt.Sprintf("Swap/I/O thrashing hang in %s (PID %d)", p.Name, p.PID),
				Detail:       fmt.Sprintf("%s is thrashing (%.0f pageins/s, %.1f MB/s disk I/O, %.0f MB footprint) — causes UI freezes even at low CPU%%.", p.Name, p.PageinsPerSec, ioMBs, footprintMB),
				TargetPID:    p.PID,
				TargetName:   p.Name,
				Status:       "pending",
				BeforeMetric: fmt.Sprintf("pageins=%.0f/s io=%.1fMB/s footprint=%.0fMB", p.PageinsPerSec, ioMBs, footprintMB),
			})
		}

		if snap.Power.HasBattery && !snap.Power.OnACPower && a.pidWakeupDrain[p.PID] >= 6 {
			recs = append(recs, store.Recommendation{
				ID:           fmt.Sprintf("proc-wakeups-%d", p.PID),
				CreatedAtMs:  snap.TimestampMs,
				Kind:         "cpu_spike",
				Severity:     "medium",
				Title:        fmt.Sprintf("Battery drain: high wakeups in %s (%.0f/s)", p.Name, p.WakeupsPerSec),
				Detail:       fmt.Sprintf("%s (PID %d) is triggering %.0f idle wakeups/s on battery, preventing CPU sleep.", p.Name, p.PID, p.WakeupsPerSec),
				TargetPID:    p.PID,
				TargetName:   p.Name,
				Status:       "pending",
				BeforeMetric: fmt.Sprintf("wakeups=%.0f/s cpu=%.1f%%", p.WakeupsPerSec, p.CPUPct),
			})
		}
	}

	// 3c. Generic multi-process app-tree bloat/thrashing (e.g. browsers, Electron/IDE helpers)
	type treeAgg struct {
		count      int
		totalMem   uint64
		totalCPU   float64
		heaviest   engine.ProcessInfo
	}
	trees := make(map[string]*treeAgg)
	for _, p := range snap.TopProcesses {
		fam := normalizeAppFamily(p.Name)
		if fam == "" {
			continue
		}
		ag := trees[fam]
		if ag == nil {
			ag = &treeAgg{heaviest: p}
			trees[fam] = ag
		}
		ag.count++
		ag.totalMem += p.MemoryBytes
		ag.totalCPU += p.CPUPct
		if p.MemoryBytes > ag.heaviest.MemoryBytes {
			ag.heaviest = p
		}
	}
	for fam, ag := range trees {
		totalMB := float64(ag.totalMem) / (1024 * 1024)
		if ag.count >= 3 && (totalMB >= 2048 || ag.totalCPU >= 120.0) &&
			(snap.Memory.PressurePct >= 65.0 || snap.Memory.SwapUsedBytes >= 512*1024*1024) {
			slug := strings.ToLower(strings.ReplaceAll(fam, " ", "-"))
			recs = append(recs, store.Recommendation{
				ID:           "app-tree-" + slug,
				CreatedAtMs:  snap.TimestampMs,
				Kind:         "memory_pressure",
				Severity:     "medium",
				Title:        fmt.Sprintf("%s process tree heavy (%d procs, %.0f MB)", fam, ag.count, totalMB),
				Detail:       fmt.Sprintf("%s and its helpers use %.0f MB RAM and %.1f%% CPU (heaviest: %s PID %d). Close idle tabs/windows or trim helper.", fam, totalMB, ag.totalCPU, ag.heaviest.Name, ag.heaviest.PID),
				TargetPID:    ag.heaviest.PID,
				TargetName:   ag.heaviest.Name,
				Status:       "pending",
				BeforeMetric: fmt.Sprintf("tree_procs=%d tree_mem=%.0fMB tree_cpu=%.1f%%", ag.count, totalMB, ag.totalCPU),
			})
		}
	}

		if snap.Disk.ReadBytesPerSec+snap.Disk.WriteBytesPerSec >= 250*1024*1024 ||
			snap.Disk.ReadIOPS+snap.Disk.WriteIOPS >= 5000 {
			a.ssdSatStreak++
		} else {
			a.ssdSatStreak = 0
		}

		// 4b. SSD I/O saturation detection (e.g. Spotlight mds_stores, Time Machine, heavy build)
		if a.ssdSatStreak >= 4 {
			topIO := "unknown"
			var topIOPID int32
			var maxProcIO float64
			for _, p := range snap.TopProcesses {
				pio := p.DiskReadBps + p.DiskWriteBps
				if pio > maxProcIO {
					maxProcIO = pio
					topIO = p.Name
					topIOPID = p.PID
				}
			}
			totalMBs := (snap.Disk.ReadBytesPerSec + snap.Disk.WriteBytesPerSec) / (1024 * 1024)
			totalIOPS := snap.Disk.ReadIOPS + snap.Disk.WriteIOPS
			recs = append(recs, store.Recommendation{
				ID:           "ssd-io-saturation",
				CreatedAtMs:  snap.TimestampMs,
				Kind:         "storage_pressure",
				Severity:     "high",
				Title:        fmt.Sprintf("SSD I/O saturation (%.0f MB/s, %.0f IOPS)", totalMBs, totalIOPS),
				Detail:       fmt.Sprintf("Top disk consumer: %s (PID %d, %.1f MB/s). Heavy SSD I/O can stall UI responsiveness.", topIO, topIOPID, maxProcIO/(1024*1024)),
				TargetPID:    topIOPID,
				TargetName:   topIO,
				Status:       "pending",
				BeforeMetric: fmt.Sprintf("ssd_mbs=%.0f iops=%.0f top=%s", totalMBs, totalIOPS, topIO),
			})
		}

		if snap.GPU.Available && (snap.GPU.DeviceUtilPct >= 85.0 || snap.GPU.RendererUtilPct >= 85.0) {
			a.gpuSatStreak++
		} else {
			a.gpuSatStreak = 0
		}

		// 4c. GPU-bound hang / saturation detection (e.g. Chrome GPU Helper, WebGL, video calls, ML)
		if a.gpuSatStreak >= 4 {
			topName := "GPU workload"
			var topPID int32
			if len(snap.TopProcesses) > 0 {
				topName = snap.TopProcesses[0].Name
				topPID = snap.TopProcesses[0].PID
			}
			recs = append(recs, store.Recommendation{
				ID:           "gpu-saturation",
				CreatedAtMs:  snap.TimestampMs,
				Kind:         "cpu_spike",
				Severity:     "medium",
				Title:        fmt.Sprintf("GPU saturation (Device %.0f%%, Renderer %.0f%%)", snap.GPU.DeviceUtilPct, snap.GPU.RendererUtilPct),
				Detail:       fmt.Sprintf("Apple GPU/Accelerator is heavily loaded (Tiler %.0f%%, %.0f MB GPU mem). Top active process: %s (PID %d).", snap.GPU.TilerUtilPct, float64(snap.GPU.InUseMemoryBytes)/(1024*1024), topName, topPID),
				TargetPID:    topPID,
				TargetName:   topName,
				Status:       "pending",
				BeforeMetric: fmt.Sprintf("gpu_dev=%.0f%% gpu_ren=%.0f%%", snap.GPU.DeviceUtilPct, snap.GPU.RendererUtilPct),
			})
		}

	// 4. 50% storage ceiling warning
	if snap.Disk.UsedPct > DefaultStorageLimitPct {
		sev := "medium"
		if snap.Disk.UsedPct >= 75.0 {
			sev = "high"
		}
		recs = append(recs, store.Recommendation{
			ID:           "disk-usage-high",
			CreatedAtMs:  snap.TimestampMs,
			Kind:         "storage_pressure",
			Severity:     sev,
			Title:        fmt.Sprintf("Storage at %.1f%% (>50%% limit)", snap.Disk.UsedPct),
			Detail:       "Storage exceeded 50% limit. Review unused caches, logs, and old Downloads installers or approve safe stale-cache cleanup.",
			TargetPID:    0,
			TargetName:   "/",
			Status:       "pending",
			BeforeMetric: fmt.Sprintf("disk_used=%.1f%%", snap.Disk.UsedPct),
		})
	}

	return recs
}

func isMostlyIncreasing(series []uint64) bool {
	if len(series) < 2 {
		return false
	}
	increases := 0
	for i := 1; i < len(series); i++ {
		if series[i] >= series[i-1] {
			increases++
		}
	}
	return increases >= len(series)-2
}

// InspectUserCaches scans top-level folders in ~/Library/Caches read-only.
func InspectUserCaches(limit int) ([]CacheCandidate, uint64, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, 0, err
	}
	cacheRoot := filepath.Join(home, "Library", "Caches")
	entries, err := os.ReadDir(cacheRoot)
	if err != nil {
		return nil, 0, err
	}

	var candidates []CacheCandidate
	var total uint64
	for _, entry := range entries {
		full := filepath.Join(cacheRoot, entry.Name())
		sz := dirSize(full, 3)
		if sz == 0 {
			continue
		}
		total += sz
		candidates = append(candidates, CacheCandidate{
			Name:      entry.Name(),
			Path:      full,
			SizeBytes: sz,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].SizeBytes > candidates[j].SizeBytes
	})
	if limit > 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates, total, nil
}

func dirSize(path string, maxDepth int) uint64 {
	info, err := os.Lstat(path)
	if err != nil {
		return 0
	}
	if !info.IsDir() {
		return uint64(info.Size())
	}
	if maxDepth <= 0 {
		return 0
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0
	}
	var sum uint64
	for _, e := range entries {
		sum += dirSize(filepath.Join(path, e.Name()), maxDepth-1)
	}
	return sum
}

func normalizeAppFamily(name string) string {
	n := strings.TrimSpace(name)
	for _, suffix := range []string{
		" Helper (Renderer)",
		" Helper (GPU)",
		" Helper (Plugin)",
		" Helper (Alerts)",
		" Helper",
		" Web Content",
		" Networking",
		" Graphics and Media",
	} {
		if idx := strings.Index(n, suffix); idx > 0 {
			return strings.TrimSpace(n[:idx])
		}
	}
	return n
}
