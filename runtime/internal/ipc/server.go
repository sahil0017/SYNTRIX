package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/syntrix/runtime/internal/agent"
	"github.com/syntrix/runtime/internal/cognitive"
	"github.com/syntrix/runtime/internal/engine"
	"github.com/syntrix/runtime/internal/governor"
	"github.com/syntrix/runtime/internal/intelligence"
	"github.com/syntrix/runtime/internal/observability"
	"github.com/syntrix/runtime/internal/research"
	"github.com/syntrix/runtime/internal/store"
	"github.com/syntrix/runtime/internal/workflow"
)

type Request struct {
	ID         string               `json:"id,omitempty"`
	Type       string               `json:"type"`
	Prompt     string               `json:"prompt,omitempty"`
	Domain     string               `json:"domain,omitempty"`
	Mode       string               `json:"mode,omitempty"`
	RecID      string               `json:"rec_id,omitempty"`
	Dir        string               `json:"dir,omitempty"`
	Query      string               `json:"query,omitempty"`
	WorkflowID string               `json:"workflow_id,omitempty"`
	ActiveApp  string               `json:"active_app,omitempty"`
	BundleID   string               `json:"bundle_id,omitempty"`
	Workflow         *workflow.Definition `json:"workflow,omitempty"`
	Approved         bool                 `json:"approved,omitempty"`
	Enabled          bool `json:"enabled,omitempty"`
	IdleThresholdSec int  `json:"idle_threshold_sec,omitempty"`
}

type TelemetryPayload struct {
	Mode            agent.Mode                   `json:"mode"`
	AutoMode        bool                         `json:"auto_mode"`
	KnowledgeDocs   int                          `json:"knowledge_docs"`
	Snapshot        engine.Snapshot              `json:"snapshot"`
	Recommendations []store.Recommendation       `json:"recommendations"`
	AuditLogs       []store.AuditEntry           `json:"audit_logs"`
	StorageForecast intelligence.StorageForecast     `json:"storage_forecast"`
	StorageAudit    *intelligence.StorageAuditReport `json:"storage_audit,omitempty"`
	ActiveApp       string                       `json:"active_app"`
	WorkflowRuns    []store.WorkflowRun          `json:"workflow_runs"`
	Governor        *governor.Status              `json:"governor,omitempty"`
	Diagnosis       *intelligence.Diagnosis       `json:"diagnosis,omitempty"`
	PythonForecast  *intelligence.PythonForecast  `json:"python_forecast,omitempty"`
	SelfHealth      *observability.HealthSnapshot `json:"self_health,omitempty"`
	Guardian        *agent.GuardianStatus        `json:"guardian,omitempty"`
	Cognitive       *cognitive.CognitiveState       `json:"cognitive,omitempty"`
	MLProgress      *intelligence.MLProgressStatus  `json:"ml_progress,omitempty"`
	MLHealthHistory []float64                       `json:"ml_health_history,omitempty"`
}

type ResearchPayload struct {
	DocCount   int                  `json:"doc_count"`
	ChunkCount int                  `json:"chunk_count"`
	EdgeCount  int                  `json:"edge_count"`
	TopConcepts []string            `json:"top_concepts,omitempty"`
	Hits       []research.SearchHit `json:"hits,omitempty"`
	GraphEdges []research.GraphEdge `json:"graph_edges,omitempty"`
}

type ServerMessage struct {
	Type         string                         `json:"type"`
	ID           string                         `json:"id,omitempty"`
	Timestamp    int64                          `json:"timestamp_ms"`
	Text         string                         `json:"text,omitempty"`
	Tool         string                         `json:"tool,omitempty"`
	Error        string                         `json:"error,omitempty"`
	Telemetry    *TelemetryPayload              `json:"telemetry,omitempty"`
	Research     *ResearchPayload               `json:"research,omitempty"`
	Notification *agent.InactiveAppNotification `json:"notification,omitempty"`
}

type Server struct {
	sockPath  string
	listener  net.Listener
	companion *agent.Companion
	store     *store.Store
	wfEngine  *workflow.Engine
	gov       *governor.Governor
	pred      *intelligence.PredictiveEngine
	pyPred    *intelligence.PythonPredictor
	obs        *observability.Tracker
	nce        *cognitive.NeuralCognitiveEngine
	storageMon *intelligence.StorageMonitor

	mu      sync.Mutex
	clients map[net.Conn]*sync.Mutex
	done    chan struct{}

	forecastMu       sync.Mutex
	cachedForecast   intelligence.StorageForecast
	cachedForecastAt time.Time
}

func NewServer(sockPath string, companion *agent.Companion, st *store.Store) *Server {
	return &Server{
		sockPath:  sockPath,
		companion: companion,
		store:     st,
		clients:   make(map[net.Conn]*sync.Mutex),
		done:      make(chan struct{}),
	}
}

func (s *Server) SetWorkflowEngine(eng *workflow.Engine) {
	s.wfEngine = eng
}

func (s *Server) SetRuntimeProviders(gov *governor.Governor, pred *intelligence.PredictiveEngine, pyPred *intelligence.PythonPredictor, obs *observability.Tracker) {
	s.gov = gov
	s.pred = pred
	s.pyPred = pyPred
	s.obs = obs
}

func (s *Server) Start() error {
	if err := os.MkdirAll(filepath.Dir(s.sockPath), 0o700); err != nil {
		return err
	}
	_ = os.Remove(s.sockPath)

	ln, err := net.Listen("unix", s.sockPath)
	if err != nil {
		return fmt.Errorf("listen unix %s: %w", s.sockPath, err)
	}
	_ = os.Chmod(s.sockPath, 0o600)
	s.listener = ln

	go s.acceptLoop()
	return nil
}

func (s *Server) Stop() {
	close(s.done)
	if s.listener != nil {
		_ = s.listener.Close()
	}
	s.mu.Lock()
	for conn := range s.clients {
		_ = conn.Close()
	}
	s.clients = make(map[net.Conn]*sync.Mutex)
	s.mu.Unlock()
	_ = os.Remove(s.sockPath)
}

func (s *Server) hasClients() bool {
	s.mu.Lock()
	n := len(s.clients)
	s.mu.Unlock()
	return n > 0
}

func (s *Server) BroadcastTelemetry(snap engine.Snapshot) {
	if !s.hasClients() {
		return
	}
	msg := ServerMessage{
		Type:      "telemetry",
		Timestamp: snap.TimestampMs,
		Telemetry: s.buildPayload(snap),
	}
	s.broadcast(msg)
}

func (s *Server) BroadcastActivity(text string) {
	if !s.hasClients() {
		return
	}
	s.broadcast(ServerMessage{
		Type:      "activity",
		Timestamp: time.Now().UnixMilli(),
		Text:      text,
	})
}

func (s *Server) BroadcastNotification(n *agent.InactiveAppNotification) {
	if n == nil || !s.hasClients() {
		return
	}
	s.broadcast(ServerMessage{
		Type:         "notification",
		Timestamp:    time.Now().UnixMilli(),
		Notification: n,
	})
}

func (s *Server) broadcast(msg ServerMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	data = append(data, '\n')

	type clientEntry struct {
		conn net.Conn
		wmu  *sync.Mutex
	}
	s.mu.Lock()
	conns := make([]clientEntry, 0, len(s.clients))
	for c, wmu := range s.clients {
		conns = append(conns, clientEntry{conn: c, wmu: wmu})
	}
	s.mu.Unlock()

	for _, entry := range conns {
		entry.wmu.Lock()
		_ = entry.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		_, err := entry.conn.Write(data)
		entry.wmu.Unlock()
		if err != nil {
			s.removeClient(entry.conn)
		}
	}
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.done:
				return
			default:
				continue
			}
		}
		s.mu.Lock()
		s.clients[conn] = &sync.Mutex{}
		s.mu.Unlock()
		go s.handleClient(conn)
	}
}

func (s *Server) removeClient(conn net.Conn) {
	s.mu.Lock()
	_, ok := s.clients[conn]
	if ok {
		delete(s.clients, conn)
	}
	s.mu.Unlock()
	if ok {
		_ = conn.Close()
	}
}

func (s *Server) handleClient(conn net.Conn) {
	defer s.removeClient(conn)

	// Send immediate snapshot if available
	if snap, ok := s.companion.LatestSnapshot(); ok {
		s.sendTo(conn, ServerMessage{
			Type:      "telemetry",
			Timestamp: snap.TimestampMs,
			Telemetry: s.buildPayload(snap),
		})
	}

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			s.sendTo(conn, ServerMessage{
				Type:      "error",
				Timestamp: time.Now().UnixMilli(),
				Error:     "invalid json request",
			})
			continue
		}
		switch req.Type {
		case "chat", "index_dir", "search_knowledge", "run_workflow", "run_custom_workflow", "full_optimize", "clean_storage":
			go s.dispatch(conn, req)
		default:
			s.dispatch(conn, req)
		}
	}
}

func (s *Server) SetCognitiveEngine(nce *cognitive.NeuralCognitiveEngine) {
	s.nce = nce
}

func (s *Server) SetStorageMonitor(m *intelligence.StorageMonitor) {
	s.storageMon = m
}

func (s *Server) buildPayload(snap engine.Snapshot) *TelemetryPayload {
	var recs []store.Recommendation
	var logs []store.AuditEntry
	var runs []store.WorkflowRun
	if s.store != nil {
		recs, _ = s.store.ListRecommendations(10)
		logs, _ = s.store.ListAuditLogs(15)
		runs, _ = s.store.ListWorkflowRuns(10)
	}
	s.forecastMu.Lock()
	if s.cachedForecastAt.IsZero() || time.Since(s.cachedForecastAt) >= 30*time.Second {
		s.cachedForecast = intelligence.ForecastStorageGrowth(s.store, snap.Disk.UsedPct)
		s.cachedForecastAt = time.Now()
	}
	fc := s.cachedForecast
	s.forecastMu.Unlock()
	knowledgeDocs := 0
	if sup := s.companion.GetSupervisor(); sup != nil {
		knowledgeDocs, _, _ = sup.RAG().Stats()
	}
	payload := &TelemetryPayload{
		Mode:            s.companion.Mode(),
		AutoMode:        s.companion.AutoMode(),
		KnowledgeDocs:   knowledgeDocs,
		Snapshot:        snap,
		Recommendations: recs,
		AuditLogs:       logs,
		StorageForecast: fc,
		ActiveApp:       s.companion.ActiveApp(),
		WorkflowRuns:    runs,
	}
	var govStatus governor.Status
	if s.gov != nil {
		govStatus = s.gov.Status()
		payload.Governor = &govStatus
	}
	if s.storageMon != nil {
		audit := s.storageMon.Evaluate(snap)
		payload.StorageAudit = &audit
	}
	if s.nce != nil {
		cs := s.nce.Evaluate(snap, govStatus, s.companion.ActiveApp())
		payload.Cognitive = &cs
	}
	if guard := s.companion.GetGuardian(); guard != nil {
		st := guard.Status()
		payload.Guardian = &st
	}
	if s.pred != nil {
		diag := s.pred.Evaluate(snap)
		payload.Diagnosis = &diag
	}
	if s.pyPred != nil {
		allowML := true
		if s.gov != nil && !s.gov.Status().AllowInference {
			allowML = false
		}
		if allowML {
			payload.PythonForecast = s.pyPred.EvaluateCached(snap)
			prog := s.pyPred.Progress()
			payload.MLProgress = &prog
		}
	}
	if s.obs != nil {
		sh := s.obs.Snapshot()
		payload.SelfHealth = &sh
	}
	if s.store != nil {
		if rows, err := s.store.RecentSnapshots(36); err == nil && len(rows) > 0 {
			hist := make([]float64, 0, len(rows))
			for i := len(rows) - 1; i >= 0; i-- {
				r := rows[i]
				diskPen := 0.0
				if r.DiskUsedPct > 30.0 {
					diskPen = r.DiskUsedPct - 30.0
				}
				pen := 0.35*r.CPUPct + 0.45*r.MemPressurePct + 0.20*diskPen
				sc := 100.0 - pen*0.85
				if sc < 5.0 {
					sc = 5.0
				} else if sc > 99.5 {
					sc = 99.5
				}
				hist = append(hist, sc)
			}
			payload.MLHealthHistory = hist
		}
	}
	return payload
}

func (s *Server) dispatch(conn net.Conn, req Request) {
	now := time.Now().UnixMilli()
	switch req.Type {
	case "chat":
		reply, tool := s.companion.HandlePrompt(req.Prompt)
		s.sendTo(conn, ServerMessage{
			Type:      "chat_reply",
			ID:        req.ID,
			Timestamp: now,
			Text:      reply,
			Tool:      tool,
		})
		s.BroadcastActivity(fmt.Sprintf("[%s] executed on prompt", tool))
		if snap, ok := s.companion.LatestSnapshot(); ok {
			s.BroadcastTelemetry(snap)
		}

	case "set_mode":
		m, ok := agent.ParseMode(req.Mode)
		if !ok {
			s.sendTo(conn, ServerMessage{
				Type:      "error",
				ID:        req.ID,
				Timestamp: now,
				Error:     "invalid mode",
			})
			return
		}
		s.companion.SetMode(m, "user")
		s.sendTo(conn, ServerMessage{
			Type:      "mode_changed",
			ID:        req.ID,
			Timestamp: now,
			Text:      string(m),
		})
		s.BroadcastActivity(fmt.Sprintf("Mode changed to %s", m))
		if snap, ok := s.companion.LatestSnapshot(); ok {
			s.BroadcastTelemetry(snap)
		}

	case "set_auto_mode":
		s.companion.SetAutoMode(req.Enabled, "user")
		s.BroadcastActivity(fmt.Sprintf("Dynamic AUTO mode %s", map[bool]string{true: "enabled", false: "disabled"}[req.Enabled]))
		if snap, ok := s.companion.LatestSnapshot(); ok {
			s.BroadcastTelemetry(snap)
		}

	case "approve_rec":
		msg, err := s.companion.ApproveRecommendation(req.RecID)
		if err != nil {
			s.sendTo(conn, ServerMessage{Type: "error", ID: req.ID, Timestamp: now, Error: err.Error()})
			return
		}
		s.sendTo(conn, ServerMessage{Type: "chat_reply", ID: req.ID, Timestamp: now, Text: msg, Tool: "approve_recommendation"})
		if s.storageMon != nil {
			s.storageMon.Invalidate()
		}
		s.BroadcastActivity("Approved recommendation: " + req.RecID)
		if snap, ok := s.companion.LatestSnapshot(); ok {
			s.BroadcastTelemetry(snap)
		}

	case "preview_rec":
		msg := s.companion.PreviewRecommendation(req.RecID)
		s.sendTo(conn, ServerMessage{Type: "chat_reply", ID: req.ID, Timestamp: now, Text: msg, Tool: "preview_recommendation"})
		s.BroadcastActivity("Dry-run preview: " + req.RecID)

	case "kill_switch":
		cancelled := s.companion.ActivateKillSwitch()
		msg := fmt.Sprintf("Kill switch activated: cancelled %d active optimization(s).", cancelled)
		s.sendTo(conn, ServerMessage{Type: "chat_reply", ID: req.ID, Timestamp: now, Text: msg, Tool: "kill_switch"})
		s.BroadcastActivity(msg)

	case "full_optimize":
		msg := s.companion.RunFullOptimize()
		if s.storageMon != nil {
			s.storageMon.Invalidate()
		}
		s.sendTo(conn, ServerMessage{Type: "chat_reply", ID: req.ID, Timestamp: now, Text: msg, Tool: "full_optimize"})
		s.BroadcastActivity("Full Optimize: " + msg)
		if snap, ok := s.companion.LatestSnapshot(); ok {
			s.BroadcastTelemetry(snap)
		}

	case "clean_storage":
		reclaimed := intelligence.CleanAllSafeTargets()
		if s.storageMon != nil {
			s.storageMon.Invalidate()
		}
		var msg string
		if reclaimed > 0 {
			msg = fmt.Sprintf("Storage Guard: reclaimed %.1f MB of stale caches, logs, and DerivedData.", float64(reclaimed)/(1024*1024))
		} else {
			msg = "Storage Guard: 0.0 MB reclaimed (remaining items may be in use, in Trash/Downloads, or protected by macOS)."
		}
		if s.store != nil {
			_ = s.store.AppendAuditLog("user", "clean_storage", "Storage", "ok", msg)
		}
		s.sendTo(conn, ServerMessage{Type: "chat_reply", ID: req.ID, Timestamp: now, Text: msg, Tool: "clean_storage"})
		s.BroadcastActivity(msg)
		if snap, ok := s.companion.LatestSnapshot(); ok {
			s.BroadcastTelemetry(snap)
		}

	case "dismiss_rec":
		msg, err := s.companion.DismissRecommendation(req.RecID)
		if err != nil {
			s.sendTo(conn, ServerMessage{Type: "error", ID: req.ID, Timestamp: now, Error: err.Error()})
			return
		}
		s.sendTo(conn, ServerMessage{Type: "chat_reply", ID: req.ID, Timestamp: now, Text: msg, Tool: "dismiss_recommendation"})
		if snap, ok := s.companion.LatestSnapshot(); ok {
			s.BroadcastTelemetry(snap)
		}

	case "get_state":
		if snap, ok := s.companion.LatestSnapshot(); ok {
			s.sendTo(conn, ServerMessage{
				Type:      "telemetry",
				ID:        req.ID,
				Timestamp: snap.TimestampMs,
				Telemetry: s.buildPayload(snap),
			})
		}

	case "index_dir":
		sup := s.companion.GetSupervisor()
		if sup == nil {
			s.sendTo(conn, ServerMessage{Type: "error", ID: req.ID, Timestamp: now, Error: "supervisor unavailable"})
			return
		}
		dir := req.Dir
		if dir == "" {
			dir = "."
		}
		count, err := sup.IndexDirectory(dir)
		if err != nil {
			s.sendTo(conn, ServerMessage{Type: "error", ID: req.ID, Timestamp: now, Error: err.Error()})
			return
		}
		docs, chunks, edges := sup.RAG().Stats()
		s.sendTo(conn, ServerMessage{
			Type:      "research_result",
			ID:        req.ID,
			Timestamp: now,
			Text:      fmt.Sprintf("Indexed %d files from %s (%d vector chunks, %d knowledge graph edges).", count, dir, chunks, edges),
			Research: &ResearchPayload{
				DocCount:    docs,
				ChunkCount:  chunks,
				EdgeCount:   edges,
				TopConcepts: sup.RAG().TopConcepts(12),
				GraphEdges:  sup.RAG().KnowledgeGraphSummary(15),
			},
		})
		s.BroadcastActivity(fmt.Sprintf("Indexed %d files in %s", count, filepath.Base(dir)))

	case "search_knowledge":
		sup := s.companion.GetSupervisor()
		if sup == nil {
			s.sendTo(conn, ServerMessage{Type: "error", ID: req.ID, Timestamp: now, Error: "supervisor unavailable"})
			return
		}
		res, err := sup.SearchKnowledge(req.Query)
		if err != nil {
			s.sendTo(conn, ServerMessage{Type: "error", ID: req.ID, Timestamp: now, Error: err.Error()})
			return
		}
		docs, chunks, edges := sup.RAG().Stats()
		s.sendTo(conn, ServerMessage{
			Type:      "research_result",
			ID:        req.ID,
			Timestamp: now,
			Text:      res,
			Research: &ResearchPayload{
				DocCount:    docs,
				ChunkCount:  chunks,
				EdgeCount:   edges,
				TopConcepts: sup.RAG().TopConcepts(12),
				Hits:        sup.RAG().SemanticSearch(req.Query, 8),
				GraphEdges:  sup.RAG().KnowledgeGraphSummary(15),
			},
		})

	case "run_workflow":
		sup := s.companion.GetSupervisor()
		if sup == nil {
			s.sendTo(conn, ServerMessage{Type: "error", ID: req.ID, Timestamp: now, Error: "supervisor unavailable"})
			return
		}
		res, err := sup.RunWorkflow(req.WorkflowID, req.Dir)
		if err != nil {
			s.sendTo(conn, ServerMessage{Type: "error", ID: req.ID, Timestamp: now, Error: err.Error()})
			return
		}
		s.sendTo(conn, ServerMessage{
			Type:      "workflow_result",
			ID:        req.ID,
			Timestamp: now,
			Text:      res,
		})
		if snap, ok := s.companion.LatestSnapshot(); ok {
			s.BroadcastTelemetry(snap)
		}

	case "set_context":
		s.companion.SetActiveContext(req.ActiveApp, req.BundleID)
		if req.ActiveApp != "" {
			s.BroadcastActivity(fmt.Sprintf("[NSWorkspace] Active app: %s", req.ActiveApp))
		}
		if snap, ok := s.companion.LatestSnapshot(); ok {
			s.BroadcastTelemetry(snap)
		}

	case "set_guardian":
		if guard := s.companion.GetGuardian(); guard != nil {
			guard.Configure(req.Enabled, req.IdleThresholdSec)
			s.BroadcastActivity(fmt.Sprintf("Guardian Agent %s (away threshold %ds)", map[bool]string{true: "enabled", false: "disabled"}[req.Enabled], req.IdleThresholdSec))
			if snap, ok := s.companion.LatestSnapshot(); ok {
				s.BroadcastTelemetry(snap)
			}
		}

	case "run_custom_workflow":
		if s.wfEngine == nil || req.Workflow == nil {
			s.sendTo(conn, ServerMessage{Type: "error", ID: req.ID, Timestamp: now, Error: "workflow engine or definition missing"})
			return
		}
		snap, _ := s.companion.LatestSnapshot()
		run := s.wfEngine.Execute(*req.Workflow, snap, 0, req.Approved)
		s.sendTo(conn, ServerMessage{
			Type:      "workflow_result",
			ID:        req.ID,
			Timestamp: now,
			Text:      fmt.Sprintf("[%s] %s (%d/%d steps)\n\n%s", run.Status, run.Name, run.CurrentStep, run.TotalSteps, run.Output),
		})
		if snap.TimestampMs > 0 {
			s.BroadcastTelemetry(snap)
		}

	default:
		s.sendTo(conn, ServerMessage{
			Type:      "error",
			ID:        req.ID,
			Timestamp: now,
			Error:     "unknown request type",
		})
	}
}

func (s *Server) sendTo(conn net.Conn, msg ServerMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	data = append(data, '\n')
	s.mu.Lock()
	wmu := s.clients[conn]
	s.mu.Unlock()
	if wmu != nil {
		wmu.Lock()
		defer wmu.Unlock()
	}
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, _ = conn.Write(data)
}
