package intelligence

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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

type PythonForecast struct {
	ModelBackend         string                `json:"model_backend"`
	ModelVersion         string                `json:"model_version"`
	FeatureSchemaVersion string                `json:"feature_schema_version"`
	SampleCount          int                   `json:"sample_count"`
	MemoryForecast       MemoryForecast        `json:"memory_forecast"`
	Storage              PythonStorageForecast `json:"storage_forecast"`
	WorkloadRegime       *WorkloadRegime       `json:"workload_regime,omitempty"`
	RecommendedActions   []RankedAction        `json:"recommended_actions,omitempty"`
	TransformerRL        *TransformerRLInfo    `json:"transformer_rl,omitempty"`
	AbnormalApps         []AbnormalApp         `json:"abnormal_apps"`
	Summary              string                `json:"summary"`
}

type PythonPredictor struct {
	mu          sync.Mutex
	dbPath      string
	scriptPath  string
	pythonBin   string
	minInterval time.Duration
	lastRun     time.Time
	inflight    bool
	cached      *PythonForecast
}

func NewPythonPredictor(dbPath string) *PythonPredictor {
	return &PythonPredictor{
		dbPath:      dbPath,
		scriptPath:  resolveMLScript(),
		pythonBin:   resolvePythonBin(),
		minInterval: 15 * time.Second,
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
		return nil, err
	}

	var out PythonForecast
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return nil, err
	}

	p.mu.Lock()
	p.cached = &out
	p.lastRun = time.Now()
	p.mu.Unlock()
	return &out, nil
}

// EvaluateCached returns the latest Python ML forecast immediately and refreshes asynchronously every minInterval.
func (p *PythonPredictor) EvaluateCached(snap engine.Snapshot) *PythonForecast {
	p.mu.Lock()
	cached := p.cached
	shouldRun := !p.inflight && (p.lastRun.IsZero() || time.Since(p.lastRun) >= p.minInterval)
	if shouldRun {
		p.inflight = true
	}
	p.mu.Unlock()

	if shouldRun {
		go func(s engine.Snapshot) {
			defer func() {
				p.mu.Lock()
				p.inflight = false
				p.mu.Unlock()
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _ = p.EvaluateSync(ctx, s)
		}(snap)
	}
	return cached
}
