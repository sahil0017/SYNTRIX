package intelligence

import (
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/syntrix/runtime/internal/engine"
)

type SuggestedAction struct {
	Label  string `json:"label"`
	Prompt string `json:"prompt"`
	Kind   string `json:"kind"`
}

type Hypothesis struct {
	Cause      string   `json:"cause"`
	Confidence float64  `json:"confidence"`
	Evidence   []string `json:"evidence"`
}

type Diagnosis struct {
	Summary                 string            `json:"summary"`
	RootCause               string            `json:"root_cause"`
	Impact                  string            `json:"impact"`
	Confidence              float64           `json:"confidence"`
	Evidence                []string          `json:"evidence"`
	AlternativeExplanations []string          `json:"alternative_explanations,omitempty"`
	Hypotheses              []Hypothesis      `json:"hypotheses,omitempty"`
	HealthTree              []string          `json:"health_tree"`
	RecommendedActions      []SuggestedAction `json:"recommended_actions"`
}

type PredictiveEngine struct {
	mu           sync.Mutex
	ewmaCPU      float64
	ewmaPressure float64
	initialized  bool
	cpuWindow    []float64
	pressWindow  []float64
	procRSS      map[string][]uint64
}

func NewPredictiveEngine() *PredictiveEngine {
	return &PredictiveEngine{
		procRSS: make(map[string][]uint64),
	}
}

func (p *PredictiveEngine) Evaluate(snap engine.Snapshot) Diagnosis {
	p.mu.Lock()
	defer p.mu.Unlock()

	const alpha = 0.25
	if !p.initialized {
		p.ewmaCPU = snap.CPU.TotalUsagePct
		p.ewmaPressure = snap.Memory.PressurePct
		p.initialized = true
	} else {
		p.ewmaCPU = alpha*snap.CPU.TotalUsagePct + (1-alpha)*p.ewmaCPU
		p.ewmaPressure = alpha*snap.Memory.PressurePct + (1-alpha)*p.ewmaPressure
	}

	p.cpuWindow = appendBounded(p.cpuWindow, snap.CPU.TotalUsagePct, 30)
	p.pressWindow = appendBounded(p.pressWindow, snap.Memory.PressurePct, 30)

	cpuMed, cpuMAD := medianAndMAD(p.cpuWindow)
	pressMed, pressMAD := medianAndMAD(p.pressWindow)

	var evidence []string
	var tree []string
	confidence := 0.55

	var topCPU engine.ProcessInfo
	var topMem engine.ProcessInfo
	var growthProc engine.ProcessInfo
	var maxGrowthMB float64
	activeKeys := make(map[string]bool, len(snap.TopProcesses))
	for _, proc := range snap.TopProcesses {
		key := fmt.Sprintf("%d:%d", proc.PID, proc.StartTimeUs)
		activeKeys[key] = true
		hist := append(p.procRSS[key], proc.MemoryBytes)
		if len(hist) > 20 {
			hist = hist[len(hist)-20:]
		}
		p.procRSS[key] = hist
		if len(hist) >= 4 {
			deltaMB := float64(int64(hist[len(hist)-1])-int64(hist[0])) / (1024 * 1024)
			if deltaMB > maxGrowthMB {
				maxGrowthMB = deltaMB
				growthProc = proc
			}
		}
		if proc.MemoryBytes > topMem.MemoryBytes {
			topMem = proc
		}
		if proc.CPUPct > topCPU.CPUPct {
			topCPU = proc
		}
	}
	for k := range p.procRSS {
		if !activeKeys[k] {
			delete(p.procRSS, k)
		}
	}

	cpuSpike := snap.CPU.TotalUsagePct > cpuMed+3.0*math.Max(cpuMAD, 5.0) && snap.CPU.TotalUsagePct > 65
	memHigh := snap.Memory.PressurePct > pressMed+2.5*math.Max(pressMAD, 4.0) || snap.Memory.PressurePct >= 75
	swapHigh := snap.Memory.SwapUsedBytes >= 1024*1024*1024
	diskHigh := snap.Disk.UsedPct > DefaultStorageLimitPct

	if cpuSpike {
		evidence = append(evidence, fmt.Sprintf("High CPU load (%.0f%% vs %.0f%% normal)", snap.CPU.TotalUsagePct, cpuMed))
		confidence += 0.15
	}
	if memHigh {
		evidence = append(evidence, fmt.Sprintf("Memory pressure at %.0f%% (compressed %.0f MB, swap %.0f MB)",
			snap.Memory.PressurePct,
			float64(snap.Memory.CompressedBytes)/(1024*1024),
			float64(snap.Memory.SwapUsedBytes)/(1024*1024)))
		confidence += 0.15
	} else if swapHigh {
		evidence = append(evidence, fmt.Sprintf("Heavy swap usage (%.1f GB on disk)", float64(snap.Memory.SwapUsedBytes)/(1024*1024*1024)))
		confidence += 0.10
	}
	if snap.Thermal.IsThrottled {
		evidence = append(evidence, fmt.Sprintf("Thermal throttling active (%s, CPU capped at %.0f%%)", snap.Thermal.StateLabel, snap.Thermal.CPUSpeedLimitPct))
		confidence += 0.10
	}
	if diskHigh {
		evidence = append(evidence, fmt.Sprintf("Disk usage is high (%.0f%% full)", snap.Disk.UsedPct))
		confidence += 0.10
	}

	if topMem.Name != "" {
		tree = append(tree,
			fmt.Sprintf("Process: %s (pid=%d, start=%d)", topMem.Name, topMem.PID, topMem.StartTimeUs),
			fmt.Sprintf("  ├─ CPU: %.1f%% | Threads: %d", topMem.CPUPct, topMem.Threads),
			fmt.Sprintf("  └─ RSS: %.0f MB | Virtual: %.0f MB", float64(topMem.MemoryBytes)/(1024*1024), float64(topMem.VirtualBytes)/(1024*1024)),
		)
	}
	tree = append(tree,
		fmt.Sprintf("System Pressure: %.0f%% [%s]", snap.Memory.PressurePct, snap.Memory.PressureLevel),
		fmt.Sprintf("  ├─ Compressed: %.0f MB | Swap Used: %.0f MB", float64(snap.Memory.CompressedBytes)/(1024*1024), float64(snap.Memory.SwapUsedBytes)/(1024*1024)),
		fmt.Sprintf("  └─ Thermal: %s (%.0f%% speed)", snap.Thermal.StateLabel, snap.Thermal.CPUSpeedLimitPct),
	)

	if confidence > 0.95 {
		confidence = 0.95
	}

	rootCause := "Nominal system operation"
	summary := "Your Mac is running smoothly"
	impact := fmt.Sprintf("Healthy headroom (CPU %.0f%% · Memory pressure %.0f%%).", snap.CPU.TotalUsagePct, snap.Memory.PressurePct)

	var actions []SuggestedAction
	if memHigh || swapHigh {
		actions = append(actions, SuggestedAction{
			Label:  "Free Memory Now",
			Prompt: "/optimize-memory",
			Kind:   "primary",
		})
	}
	if cpuSpike && topCPU.Name != "" {
		actions = append(actions, SuggestedAction{
			Label:  fmt.Sprintf("Inspect %s", topCPU.Name),
			Prompt: fmt.Sprintf("Inspect process %s (PID %d)", topCPU.Name, topCPU.PID),
			Kind:   "primary",
		})
	} else if memHigh && topMem.Name != "" {
		actions = append(actions, SuggestedAction{
			Label:  fmt.Sprintf("Inspect %s", topMem.Name),
			Prompt: fmt.Sprintf("Inspect process %s (PID %d)", topMem.Name, topMem.PID),
			Kind:   "secondary",
		})
	}
	if diskHigh {
		actions = append(actions, SuggestedAction{
			Label:  "Scan Caches",
			Prompt: "/caches",
			Kind:   "primary",
		})
	}
	if len(actions) == 0 {
		actions = append(actions,
			SuggestedAction{Label: "Run Health Check", Prompt: "/status", Kind: "secondary"},
			SuggestedAction{Label: "Check Caches", Prompt: "/caches", Kind: "secondary"},
		)
	}

	if len(evidence) >= 2 {
		lead := topMem
		if cpuSpike && topCPU.Name != "" {
			lead = topCPU
		}
		rootCause = fmt.Sprintf("Correlated resource contention led by %s (PID %d)", lead.Name, lead.PID)
		summary = fmt.Sprintf("Slowdown risk: %s & system contention", lead.Name)
		impact = "Multiple subsystems are under strain — freeing RAM or inspecting the top app can restore responsiveness."
	} else if len(evidence) == 1 {
		rootCause = evidence[0]
		switch {
		case memHigh || swapHigh:
			summary = "Memory pressure is elevated"
			impact = "Apps may stutter or swap to disk. Freeing memory or closing heavy tabs will help."
		case cpuSpike:
			summary = fmt.Sprintf("CPU spike driven by %s", topCPU.Name)
			impact = "Sustained high CPU can drain battery and heat up your Mac."
		case snap.Thermal.IsThrottled:
			summary = "Thermal throttling is active"
			impact = fmt.Sprintf("macOS has capped CPU speed to %.0f%% to cool down.", snap.Thermal.CPUSpeedLimitPct)
		case diskHigh:
			summary = "Storage space is running low"
			impact = "Low free space slows macOS swap and updates. Clearing caches can reclaim GBs."
		}
	}

	var alternatives []string
	var hypotheses []Hypothesis
	if maxGrowthMB >= 100 && growthProc.Name != "" {
		msg := fmt.Sprintf("%s (PID %d) grew by %.0f MB over recent samples (CPU %.1f%%) — consistent with a memory-heavy workload, does not establish a leak.",
			growthProc.Name, growthProc.PID, maxGrowthMB, growthProc.CPUPct)
		evidence = append(evidence, msg)
		hypotheses = append(hypotheses, Hypothesis{
			Cause:      fmt.Sprintf("Working-set expansion in %s", growthProc.Name),
			Confidence: 0.72,
			Evidence:   []string{msg},
		})
		alternatives = append(alternatives,
			"Normal working-set expansion for active workload or open tabs/projects",
			"macOS compressed cache retention prior to page-out",
		)
	}

	return Diagnosis{
		Summary:                 summary,
		RootCause:               rootCause,
		Impact:                  impact,
		Confidence:              confidence,
		Evidence:                evidence,
		AlternativeExplanations: alternatives,
		Hypotheses:              hypotheses,
		HealthTree:              tree,
		RecommendedActions:      actions,
	}
}

func appendBounded(s []float64, v float64, max int) []float64 {
	s = append(s, v)
	if len(s) > max {
		return s[len(s)-max:]
	}
	return s
}

func medianAndMAD(vals []float64) (float64, float64) {
	if len(vals) == 0 {
		return 0, 0
	}
	cp := append([]float64(nil), vals...)
	sort.Float64s(cp)
	med := cp[len(cp)/2]

	devs := make([]float64, len(cp))
	for i, v := range cp {
		devs[i] = math.Abs(v - med)
	}
	sort.Float64s(devs)
	return med, devs[len(devs)/2]
}
