package intelligence

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/syntrix/runtime/internal/engine"
)

type MemoryForecast struct {
	CurrentPct        float64 `json:"current_pct"`
	Predicted1mPct    float64 `json:"predicted_1m_pct"`
	Predicted5mPct    float64 `json:"predicted_5m_pct"`
	Predicted15mPct   float64 `json:"predicted_15m_pct"`
	Predicted30mPct   float64 `json:"predicted_30m_pct"`
	UncertaintyPct    float64 `json:"uncertainty_pct"`
	SlopePctPerMin    float64 `json:"slope_pct_per_min"`
	MinutesToCritical float64 `json:"minutes_to_critical"`
	SelectedModel     string  `json:"selected_model"`
	ValidationMAE     float64 `json:"validation_mae"`
	DriftDetected     bool    `json:"drift_detected"`
	DriftScore        float64 `json:"drift_score"`
	Risk              string  `json:"risk"`
}

type PythonStorageForecast struct {
	CurrentPct         float64 `json:"current_pct"`
	GrowthPctPerDay    float64 `json:"growth_pct_per_day"`
	DaysUntil90Pct     int     `json:"days_until_90_pct"`
	DaysUntil90PctLow  int     `json:"days_until_90_pct_low"`
	DaysUntil90PctHigh int     `json:"days_until_90_pct_high"`
	Risk               string  `json:"risk"`
}

type AbnormalApp struct {
	Name         string  `json:"name"`
	PID          int32   `json:"pid"`
	CPUPct       float64 `json:"cpu_pct"`
	MemMB        float64 `json:"mem_mb"`
	AnomalyScore float64 `json:"anomaly_score"`
	Reason       string  `json:"reason"`
}

type WorkloadRegime struct {
	Regime    string `json:"regime"`
	ClusterID int    `json:"cluster_id"`
	Model     string `json:"model"`
}

type RankedAction struct {
	Action  string  `json:"action"`
	Utility float64 `json:"utility"`
}

type TransformerRLInfo struct {
	PredCPUPct    float64 `json:"transformer_pred_cpu_pct"`
	PredMemPct    float64 `json:"transformer_pred_mem_pct"`
	RMSE          float64 `json:"transformer_rmse_pct"`
	AccuracyPct   float64 `json:"transformer_accuracy_pct"`
	OptimalAction string  `json:"rl_optimal_action"`
	QValue        float64 `json:"rl_q_value"`
	Episodes      int     `json:"rl_episodes"`
	AvgReward     float64 `json:"rl_avg_reward"`
	SQLiteRows    int     `json:"sqlite_rows_used"`
}

type BottleneckClassification struct {
	DominantState string  `json:"dominant_state"`
	Confidence    float64 `json:"confidence"`
	Culprit       string  `json:"culprit"`
	Model         string  `json:"model"`
}

type PCAHealthIndex struct {
	HealthScore           float64 `json:"health_score"`
	PrimaryVarianceDriver string  `json:"primary_variance_driver"`
	PCAExplainedVariance  float64 `json:"pca_explained_variance"`
	Model                 string  `json:"model"`
}

type StrategicDirective struct {
	RecommendedDirective  string  `json:"recommended_directive"`
	ExpectedHealthGainPct float64 `json:"expected_health_gain_pct"`
	Rationale             string  `json:"rationale"`
	Policy                string  `json:"policy"`
}

type ProcessHangRisk struct {
	Name          string  `json:"name"`
	PID           int32   `json:"pid"`
	HangRiskScore float64 `json:"hang_risk_score"`
	Cause         string  `json:"cause"`
}

type SuperIntelligenceInfo struct {
	Bottleneck          BottleneckClassification `json:"bottleneck"`
	HealthIndex         PCAHealthIndex           `json:"health_index"`
	StrategicDirective  StrategicDirective       `json:"strategic_directive"`
	HangRiskScore       float64                  `json:"hang_risk_score"`
	AtRiskProcesses     []ProcessHangRisk        `json:"at_risk_processes,omitempty"`
	StorageUrgencyScore float64                  `json:"storage_urgency_score"`
	InferenceMs         float64                  `json:"inference_ms"`
}

type ResourceNeuralInfo struct {
	TCNPredMemPct      float64 `json:"tcn_pred_mem_pct"`
	GRUPredMemPct      float64 `json:"gru_pred_mem_pct"`
	SelectedModel      string  `json:"selected_model"`
	EnsemblePredMemPct float64 `json:"ensemble_pred_mem_pct"`
	Architecture       string  `json:"architecture"`
}

type BehaviourNeuralInfo struct {
	MaxReconError  float64 `json:"max_recon_error"`
	AnomalousCount int     `json:"anomalous_count"`
	TopProcess     string  `json:"top_process"`
	Model          string  `json:"model"`
}

type ContextNeuralInfo struct {
	WorkingPattern    string  `json:"working_pattern"`
	Confidence        float64 `json:"confidence"`
	SequenceStability float64 `json:"sequence_stability"`
	Model             string  `json:"model"`
}

type CausalNeuralInfo struct {
	RootCause           string   `json:"root_cause"`
	CausalChain         []string `json:"causal_chain"`
	ContributingFactors []string `json:"contributing_factors"`
	CausalStrength      float64  `json:"causal_strength"`
	Model               string   `json:"model"`
}

type DecisionNeuralInfo struct {
	TopAction        string `json:"top_action"`
	RootCauseContext string `json:"root_cause_context"`
	Policy           string `json:"policy"`
}

type NeuralSubsystemsInfo struct {
	Resource ResourceNeuralInfo  `json:"resource_intelligence"`
	Behaviour BehaviourNeuralInfo `json:"behaviour_intelligence"`
	Context  ContextNeuralInfo   `json:"context_intelligence"`
	Causal   CausalNeuralInfo    `json:"causal_intelligence"`
	Decision DecisionNeuralInfo  `json:"decision_intelligence"`
	IntentModel    string        `json:"intent_model"`
	KnowledgeModel string        `json:"knowledge_model"`
	LatencyMs      float64       `json:"latency_ms"`
}

type ImprovementTip struct {
	Category string `json:"category"`
	Title    string `json:"title"`
	Action   string `json:"action"`
	Command  string `json:"command"`
}

type AppAdvisorInfo struct {
	PlainEnglishSummary string           `json:"plain_english_summary"`
	UnderstandingNotes  []string         `json:"understanding_notes"`
	ImprovementTips     []ImprovementTip `json:"improvement_tips"`
}

type PythonForecast struct {
	ModelBackend         string                 `json:"model_backend"`
	ModelVersion         string                 `json:"model_version"`
	FeatureSchemaVersion string                 `json:"feature_schema_version"`
	SampleCount          int                    `json:"sample_count"`
	MemoryForecast       MemoryForecast         `json:"memory_forecast"`
	Storage              PythonStorageForecast  `json:"storage_forecast"`
	WorkloadRegime       *WorkloadRegime        `json:"workload_regime,omitempty"`
	RecommendedActions   []RankedAction         `json:"recommended_actions,omitempty"`
	TransformerRL        *TransformerRLInfo     `json:"transformer_rl,omitempty"`
	SuperIntelligence    *SuperIntelligenceInfo `json:"super_intelligence,omitempty"`
	NeuralSubsystems     *NeuralSubsystemsInfo  `json:"neural_subsystems,omitempty"`
	AppAdvisor           *AppAdvisorInfo        `json:"app_advisor,omitempty"`
	AbnormalApps         []AbnormalApp          `json:"abnormal_apps"`
	Summary              string                 `json:"summary"`
	InferenceMs          float64                `json:"inference_ms,omitempty"`
}

type MLProgressStatus struct {
	Evaluating  bool    `json:"evaluating"`
	ElapsedMs   int64   `json:"elapsed_ms"`
	EstimatedMs int64   `json:"estimated_ms"`
	ProgressPct float64 `json:"progress_pct"`
	Stage       string  `json:"stage"`
	LastError   string  `json:"last_error,omitempty"`
}

type PythonPredictor struct {
	mu          sync.Mutex
	dbPath      string
	scriptPath  string
	pythonBin   string
	minInterval time.Duration
	lastRun     time.Time
	startedAt   time.Time
	estimatedMs int64
	lastError   string
	inflight    bool
	cached      *PythonForecast
}

func NewPythonPredictor(dbPath string) *PythonPredictor {
	return &PythonPredictor{
		dbPath:      dbPath,
		scriptPath:  resolveMLScript(),
		pythonBin:   resolvePythonBin(),
		minInterval: 15 * time.Second,
		estimatedMs: 3200,
	}
}

func (p *PythonPredictor) SetMinInterval(d time.Duration) {
	p.mu.Lock()
	p.minInterval = d
	p.mu.Unlock()
}

func resolvePythonBin() string {
	if env := os.Getenv("SYNTRIX_PYTHON"); env != "" {
		return env
	}
	if pathPython, err := exec.LookPath("python3"); err == nil {
		return pathPython
	}
	for _, candidate := range []string{"/opt/homebrew/bin/python3", "/usr/local/bin/python3", "python3"} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "python3"
}

func resolveMLScript() string {
	if env := os.Getenv("SYNTRIX_ML_SCRIPT"); env != "" {
		return env
	}
	candidates := []string{
		"ml/predictive_model.py",
		"../ml/predictive_model.py",
		"../../ml/predictive_model.py",
		"../../../ml/predictive_model.py",
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "ml", "predictive_model.py"),
			filepath.Join(dir, "..", "Resources", "predictive_model.py"),
		)
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return "ml/predictive_model.py"
}

func (p *PythonPredictor) EvaluateSync(ctx context.Context, snap engine.Snapshot) (*PythonForecast, error) {
	payload := map[string]any{
		"timestamp_ms":     snap.TimestampMs,
		"cpu_pct":          snap.CPU.TotalUsagePct,
		"mem_pressure_pct": snap.Memory.PressurePct,
		"disk_used_pct":    snap.Disk.UsedPct,
		"pageins_per_sec":  snap.Memory.PageinsPerSec,
		"pageouts_per_sec": snap.Memory.PageoutsPerSec,
		"gpu_util_pct":     snap.GPU.DeviceUtilPct,
		"top_processes":    snap.TopProcesses,
	}
	inBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	args := []string{p.scriptPath}
	if p.dbPath != "" {
		args = append(args, "--db", p.dbPath)
	}

	cmd := exec.CommandContext(ctx, p.pythonBin, args...)
	cmd.Stdin = bytes.NewReader(inBytes)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		p.mu.Lock()
		p.lastError = msg
		p.mu.Unlock()
		return nil, err
	}

	var out PythonForecast
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		p.mu.Lock()
		p.lastError = err.Error()
		p.mu.Unlock()
		return nil, err
	}

	p.mu.Lock()
	p.cached = &out
	p.lastRun = time.Now()
	p.lastError = ""
	if !p.startedAt.IsZero() {
		actual := time.Since(p.startedAt).Milliseconds()
		if actual > 100 {
			p.estimatedMs = (p.estimatedMs*3 + actual*7) / 10
		}
	}
	p.mu.Unlock()
	return &out, nil
}

// Progress returns live ML evaluation timing & progress percentage for UI progress bars.
func (p *PythonPredictor) Progress() MLProgressStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	est := p.estimatedMs
	if est <= 0 {
		est = 3200
	}
	if !p.inflight {
		pct := 0.0
		stage := "Queued for first ML run"
		if p.cached != nil {
			pct = 100.0
			stage = "Ready"
		}
		return MLProgressStatus{
			Evaluating:  false,
			ElapsedMs:   0,
			EstimatedMs: est,
			ProgressPct: pct,
			Stage:       stage,
			LastError:   p.lastError,
		}
	}
	elapsed := time.Since(p.startedAt).Milliseconds()
	pct := float64(elapsed) / float64(est) * 100.0
	if pct > 95.0 {
		pct = 95.0
	}
	stage := "Loading sklearn + SQLite history"
	if pct >= 65.0 {
		stage = "Running 2-Layer Transformer + 7 Neural Subsystems"
	} else if pct >= 30.0 {
		stage = "Training Ridge/RF/GBR/MLP + IsolationForest"
	}
	return MLProgressStatus{
		Evaluating:  true,
		ElapsedMs:   elapsed,
		EstimatedMs: est,
		ProgressPct: pct,
		Stage:       stage,
		LastError:   p.lastError,
	}
}

// EvaluateCached returns the latest Python ML forecast immediately and refreshes asynchronously every minInterval.
func (p *PythonPredictor) EvaluateCached(snap engine.Snapshot) *PythonForecast {
	p.mu.Lock()
	cached := p.cached
	shouldRun := !p.inflight && (p.lastRun.IsZero() || time.Since(p.lastRun) >= p.minInterval)
	if shouldRun {
		p.inflight = true
		p.startedAt = time.Now()
	}
	p.mu.Unlock()

	if shouldRun {
		go func(s engine.Snapshot) {
			defer func() {
				p.mu.Lock()
				p.inflight = false
				p.mu.Unlock()
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_, _ = p.EvaluateSync(ctx, s)
		}(snap)
	}
	return cached
}
