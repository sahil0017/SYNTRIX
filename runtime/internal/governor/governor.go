package governor

import (
	"runtime"
	"sync"
	"time"

	"github.com/syntrix/runtime/internal/engine"
)

type State string

const (
	StateNormal      State = "NORMAL"
	StateConstrained State = "CONSTRAINED"
	StatePressure    State = "PRESSURE"
	StateCritical    State = "CRITICAL"
)

type Status struct {
	State              State   `json:"state"`
	MemoryPressurePct  float64 `json:"memory_pressure_pct"`
	GoHeapAllocMB      float64 `json:"go_heap_alloc_mb"`
	Goroutines         int     `json:"goroutines"`
	MaxConcurrentAgent int     `json:"max_concurrent_agents"`
	AllowIndexing      bool    `json:"allow_indexing"`
	AllowInference     bool    `json:"allow_inference"`
}

type Governor struct {
	mu            sync.RWMutex
	state         State
	pendingState  State
	pendingStreak int
	lastStatus    Status
}

func New() *Governor {
	return &Governor{
		state: StateNormal,
		lastStatus: Status{
			State:              StateNormal,
			MaxConcurrentAgent: 4,
			AllowIndexing:      true,
			AllowInference:     true,
		},
	}
}

// Evaluate updates the governor state with hysteresis based on host pressure and Syntrix self-metrics.
func (g *Governor) Evaluate(snap engine.Snapshot) Status {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	heapMB := float64(ms.HeapAlloc) / (1024 * 1024)
	goroutines := runtime.NumGoroutine()
	pressure := snap.Memory.PressurePct

	g.mu.Lock()
	defer g.mu.Unlock()

	target := classifyTarget(g.state, pressure, heapMB)
	if target != g.state {
		if target == g.pendingState {
			g.pendingStreak++
		} else {
			g.pendingState = target
			g.pendingStreak = 1
		}
		// Escalate after 2 consecutive samples; recover after 3 consecutive samples.
		needed := 2
		if severityRank(target) < severityRank(g.state) {
			needed = 3
		}
		if g.pendingStreak >= needed {
			g.state = target
			g.pendingStreak = 0
		}
	} else {
		g.pendingStreak = 0
	}

	st := Status{
		State:             g.state,
		MemoryPressurePct: pressure,
		GoHeapAllocMB:     heapMB,
		Goroutines:        goroutines,
	}
	switch g.state {
	case StateNormal:
		st.MaxConcurrentAgent = 4
		st.AllowIndexing = true
		st.AllowInference = true
	case StateConstrained:
		st.MaxConcurrentAgent = 2
		st.AllowIndexing = true
		st.AllowInference = true
	case StatePressure:
		st.MaxConcurrentAgent = 1
		st.AllowIndexing = false
		st.AllowInference = true
	case StateCritical:
		st.MaxConcurrentAgent = 1
		st.AllowIndexing = false
		st.AllowInference = false
	}
	g.lastStatus = st
	return st
}

func (g *Governor) Status() Status {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.lastStatus
}

func (g *Governor) SamplingInterval(base time.Duration) time.Duration {
	g.mu.RLock()
	st := g.state
	g.mu.RUnlock()

	switch st {
	case StateConstrained:
		return base * 2
	case StatePressure:
		return base * 3
	case StateCritical:
		return base * 4
	default:
		return base
	}
}

func classifyTarget(cur State, pressurePct, heapMB float64) State {
	// Hysteresis band: require 5% lower pressure to step down.
	if pressurePct >= 90 || heapMB >= 48 {
		return StateCritical
	}
	if pressurePct >= 80 || heapMB >= 36 || (cur == StateCritical && pressurePct >= 85) {
		return StatePressure
	}
	if pressurePct >= 65 || heapMB >= 24 || (cur == StatePressure && pressurePct >= 75) {
		return StateConstrained
	}
	if cur == StateConstrained && pressurePct >= 60 {
		return StateConstrained
	}
	return StateNormal
}

func severityRank(s State) int {
	switch s {
	case StateNormal:
		return 0
	case StateConstrained:
		return 1
	case StatePressure:
		return 2
	case StateCritical:
		return 3
	default:
		return 0
	}
}
