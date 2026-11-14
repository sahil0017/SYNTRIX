package cognitive

import (
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/syntrix/runtime/internal/engine"
	"github.com/syntrix/runtime/internal/governor"
)

// SchedulingTier mirrors the Resource Governor's cognitive budget on M1 8GB.
type SchedulingTier string

const (
	TierNormal      SchedulingTier = "NORMAL"
	TierConstrained SchedulingTier = "CONSTRAINED"
	TierPressure    SchedulingTier = "PRESSURE"
	TierCritical    SchedulingTier = "CRITICAL"
)

// PerceptionObservation is the fused sensory state produced by the Perception Engine.
type PerceptionObservation struct {
	TimestampMs      int64    `json:"timestamp_ms"`
	ContentionScore  float64  `json:"contention_score"`
	PrimaryCandidate string   `json:"primary_candidate"`
	ActiveApp        string   `json:"active_app,omitempty"`
	Evidence         []string `json:"evidence"`
	Confidence       float64  `json:"confidence"`
	Summary          string   `json:"summary"`
}

// TemporalEdge represents a time-bounded relationship in the World Model graph.
type TemporalEdge struct {
	Source         string  `json:"source"`
	Target         string  `json:"target"`
	Relation       string  `json:"relation"`
	ValidFromMs    int64   `json:"valid_from_ms"`
	ValidToMs      int64   `json:"valid_to_ms"`
	Evidence       string  `json:"evidence"`
	Confidence     float64 `json:"confidence"`
	Classification string  `json:"classification"` // "observed" or "inferred"
}

// TemporalGraph maintains a bounded, continuously updated property graph of the Mac.
type TemporalGraph struct {
	edges []TemporalEdge
	ttlMs int64
}

func NewTemporalGraph(ttlMs int64) *TemporalGraph {
	if ttlMs <= 0 {
		ttlMs = 5 * 60 * 1000 // 5 minutes
	}
	return &TemporalGraph{ttlMs: ttlMs}
}

func (g *TemporalGraph) Upsert(edge TemporalEdge) {
	for i := range g.edges {
		if g.edges[i].Source == edge.Source && g.edges[i].Target == edge.Target && g.edges[i].Relation == edge.Relation {
			g.edges[i] = edge
			return
		}
	}
	g.edges = append(g.edges, edge)
}

func (g *TemporalGraph) Prune(nowMs int64) {
	kept := g.edges[:0]
	for _, e := range g.edges {
		if e.ValidToMs == 0 || e.ValidToMs >= nowMs {
			kept = append(kept, e)
		}
	}
	g.edges = kept
	if len(g.edges) > 128 {
		g.edges = g.edges[len(g.edges)-128:]
	}
}

func (g *TemporalGraph) Edges() []TemporalEdge {
	out := make([]TemporalEdge, len(g.edges))
	copy(out, g.edges)
	return out
}

// EpisodicIncident records a compressed significant system event.
type EpisodicIncident struct {
	TimestampMs int64   `json:"timestamp_ms"`
	Kind        string  `json:"kind"`
	Summary     string  `json:"summary"`
	Confidence  float64 `json:"confidence"`
}

// ProcedureStats tracks success/failure outcomes for Procedural Memory & Learning.
type ProcedureStats struct {
	Action    string  `json:"action"`
	Successes int     `json:"successes"`
	Failures  int     `json:"failures"`
	Score     float64 `json:"score"`
}

// CognitiveState is the unified snapshot returned by the Neural Cognitive Engine.
type CognitiveState struct {
	SchedulingTier SchedulingTier        `json:"scheduling_tier"`
	AllowNeuralML  bool                  `json:"allow_neural_ml"`
	Observation    PerceptionObservation `json:"observation"`
	GraphEdgeCount int                   `json:"graph_edge_count"`
	WorkingMemLen  int                   `json:"working_memory_len"`
	EpisodicLen    int                   `json:"episodic_memory_len"`
	TopActions     []ProcedureStats      `json:"top_actions"`
	CalibrationMAE float64               `json:"calibration_mae"`
}

// NeuralCognitiveEngine coordinates Perception, World Model, 4-Tier Memory,
// Learning/Calibration, and Governor-aware Scheduling.
type NeuralCognitiveEngine struct {
	mu            sync.Mutex
	world         *TemporalGraph
	workingMem    []PerceptionObservation
	episodicMem   []EpisodicIncident
	procedural    map[string]*ProcedureStats
	predErrors    []float64
	totalOutcomes int
}

func NewNeuralCognitiveEngine() *NeuralCognitiveEngine {
	return &NeuralCognitiveEngine{
		world:      NewTemporalGraph(5 * 60 * 1000),
		procedural: make(map[string]*ProcedureStats),
	}
}

// Evaluate runs Perception -> World Model -> Memory -> Reasoning/Learning under Governor constraints.
func (n *NeuralCognitiveEngine) Evaluate(snap engine.Snapshot, gov governor.Status, activeApp string) CognitiveState {
	n.mu.Lock()
	defer n.mu.Unlock()

	tier := mapSchedulingTier(gov)
	allowML := tier == TierNormal || tier == TierConstrained

	obs := fusePerception(snap, activeApp)
	n.workingMem = append(n.workingMem, obs)
	if len(n.workingMem) > 20 {
		n.workingMem = n.workingMem[len(n.workingMem)-20:]
	}

	nowMs := snap.TimestampMs
	validTo := nowMs + n.world.ttlMs
	n.world.Upsert(TemporalEdge{
		Source:         "macbook",
		Target:         "memory",
		Relation:       "pressure_state",
		ValidFromMs:    nowMs,
		ValidToMs:      validTo,
		Evidence:       fmt.Sprintf("pressure=%.1f%% swap=%.0fMB", snap.Memory.PressurePct, float64(snap.Memory.SwapUsedBytes)/(1024*1024)),
		Confidence:     0.95,
		Classification: "observed",
	})
	if obs.PrimaryCandidate != "" {
		n.world.Upsert(TemporalEdge{
			Source:         obs.PrimaryCandidate,
			Target:         "memory",
			Relation:       "consumes_primary",
			ValidFromMs:    nowMs,
			ValidToMs:      validTo,
			Evidence:       obs.Summary,
			Confidence:     obs.Confidence,
			Classification: "inferred",
		})
	}
	if activeApp != "" {
		n.world.Upsert(TemporalEdge{
			Source:         "user",
			Target:         "app:" + activeApp,
			Relation:       "focused_on",
			ValidFromMs:    nowMs,
			ValidToMs:      validTo,
			Evidence:       "NSWorkspace",
			Confidence:     1.0,
			Classification: "observed",
		})
	}
	n.world.Prune(nowMs)

	if obs.ContentionScore >= 0.65 {
		n.episodicMem = append(n.episodicMem, EpisodicIncident{
			TimestampMs: nowMs,
			Kind:        "contention",
			Summary:     obs.Summary,
			Confidence:  obs.Confidence,
		})
		if len(n.episodicMem) > 50 {
			n.episodicMem = n.episodicMem[len(n.episodicMem)-50:]
		}
	}

	return CognitiveState{
		SchedulingTier: tier,
		AllowNeuralML:  allowML,
		Observation:    obs,
		GraphEdgeCount: len(n.world.Edges()),
		WorkingMemLen:  len(n.workingMem),
		EpisodicLen:    len(n.episodicMem),
		TopActions:     n.rankActionsLocked(),
		CalibrationMAE: n.meanPredErrorLocked(),
	}
}

// RecordOutcome updates Procedural Memory and contextual bandit scores from real execution feedback.
func (n *NeuralCognitiveEngine) RecordOutcome(action string, success bool, predictedPct, actualPct float64) {
	n.mu.Lock()
	defer n.mu.Unlock()

	st, ok := n.procedural[action]
	if !ok {
		st = &ProcedureStats{Action: action}
		n.procedural[action] = st
	}
	if success {
		st.Successes++
	} else {
		st.Failures++
	}
	n.totalOutcomes++
	n.predErrors = append(n.predErrors, math.Abs(predictedPct-actualPct))
	if len(n.predErrors) > 50 {
		n.predErrors = n.predErrors[len(n.predErrors)-50:]
	}

	total := float64(st.Successes + st.Failures)
	mean := float64(st.Successes) / math.Max(1.0, total)
	explore := math.Sqrt(2.0 * math.Log(float64(maxInt(2, n.totalOutcomes))) / math.Max(1.0, total))
	st.Score = math.Round((mean+0.25*explore)*100.0) / 100.0
}

func (n *NeuralCognitiveEngine) rankActionsLocked() []ProcedureStats {
	out := make([]ProcedureStats, 0, len(n.procedural))
	for _, v := range n.procedural {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

func (n *NeuralCognitiveEngine) meanPredErrorLocked() float64 {
	if len(n.predErrors) == 0 {
		return 0
	}
	var sum float64
	for _, e := range n.predErrors {
		sum += e
	}
	return math.Round((sum/float64(len(n.predErrors)))*100.0) / 100.0
}

func fusePerception(snap engine.Snapshot, activeApp string) PerceptionObservation {
	var top engine.ProcessInfo
	for _, p := range snap.TopProcesses {
		if p.MemoryBytes > top.MemoryBytes {
			top = p
		}
	}

	cpuFrac := math.Min(1.0, snap.CPU.TotalUsagePct/100.0)
	memFrac := math.Min(1.0, snap.Memory.PressurePct/100.0)
	swapMB := float64(snap.Memory.SwapUsedBytes) / (1024 * 1024)
	swapFrac := math.Min(1.0, swapMB/2048.0)

	contention := math.Round((0.35*cpuFrac+0.45*memFrac+0.20*swapFrac)*100.0) / 100.0
	var ev []string
	if snap.CPU.TotalUsagePct >= 65 {
		ev = append(ev, fmt.Sprintf("CPU %.1f%%", snap.CPU.TotalUsagePct))
	}
	if snap.Memory.PressurePct >= 65 {
		ev = append(ev, fmt.Sprintf("Memory pressure %.0f%%", snap.Memory.PressurePct))
	}
	if swapMB >= 256 {
		ev = append(ev, fmt.Sprintf("Swap %.0f MB", swapMB))
	}
	if snap.Thermal.IsThrottled {
		ev = append(ev, "Thermal throttling active")
	}

	candidate := ""
	if top.Name != "" {
		candidate = fmt.Sprintf("process:%s:%d", top.Name, top.PID)
	}
	summary := "System operating within normal parameters"
	if contention >= 0.65 && top.Name != "" {
		summary = fmt.Sprintf("Resource contention detected; primary candidate %s (%.0f MB RSS)", top.Name, float64(top.MemoryBytes)/(1024*1024))
	}

	conf := math.Min(0.95, 0.55+0.1*float64(len(ev)))
	return PerceptionObservation{
		TimestampMs:      snap.TimestampMs,
		ContentionScore:  contention,
		PrimaryCandidate: candidate,
		ActiveApp:        activeApp,
		Evidence:         ev,
		Confidence:       math.Round(conf*100.0) / 100.0,
		Summary:          summary,
	}
}

func mapSchedulingTier(gov governor.Status) SchedulingTier {
	switch gov.State {
	case governor.StateCritical:
		return TierCritical
	case governor.StatePressure:
		return TierPressure
	case governor.StateConstrained:
		return TierConstrained
	default:
		return TierNormal
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
