package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/syntrix/runtime/internal/agent"
	"github.com/syntrix/runtime/internal/cognitive"
	"github.com/syntrix/runtime/internal/engine"
	"github.com/syntrix/runtime/internal/events"
	"github.com/syntrix/runtime/internal/governor"
	"github.com/syntrix/runtime/internal/intelligence"
	"github.com/syntrix/runtime/internal/ipc"
	"github.com/syntrix/runtime/internal/observability"
	"github.com/syntrix/runtime/internal/policy"
	"github.com/syntrix/runtime/internal/store"
	"github.com/syntrix/runtime/internal/tools"
	"github.com/syntrix/runtime/internal/workflow"
)

func defaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/syntrix.db"
	}
	return filepath.Join(home, ".syntrix", "syntrix.db")
}

func main() {
	dbPath := flag.String("db", defaultDBPath(), "SQLite database path")
	sockPath := flag.String("sock", "/tmp/syntrixd.sock", "Unix domain socket path")
	interval := flag.Duration("interval", time.Second, "Telemetry sampling interval")
	modeFlag := flag.String("mode", string(agent.ModeAssist), "Initial mode: OFF|MONITOR|ASSIST|AUTOPILOT|LEARNING")
	flag.Parse()

	collector, err := engine.NewCollector()
	if err != nil {
		log.Fatalf("init engine: %v", err)
	}
	defer collector.Close()

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	bus := events.NewBus()
	obs := observability.NewTracker()
	bus.SetJournalHook(func(ev events.Event) {
		obs.IncEvents()
		_ = st.InsertEventJournal(
			ev.Seq, ev.EventID, ev.SchemaVersion, int(ev.Priority),
			string(ev.Type), ev.CorrelationID, ev.Source, ev.Target, ev.Summary, ev.TimestampMs,
		)
	})
	gov := governor.New()
	pred := intelligence.NewPredictiveEngine()
	pol := policy.NewEngine(bus)
	reg := tools.NewRegistry()
	analyzer := intelligence.NewAnalyzer()
	companion := agent.NewCompanion(st, analyzer)
	orch := agent.NewOrchestrator(bus, pol, collector, st, companion)
	sup := agent.NewSupervisor(bus, reg, st, companion)
	nce := cognitive.NewNeuralCognitiveEngine()
	guardian := agent.NewGuardianAgent(orch, st)
	guardian.SetCognitiveEngine(nce)
	inactiveMon := agent.NewInactiveAppMonitor()
	companion.SetOrchestrator(orch)
	companion.SetSupervisor(sup)
	companion.SetGuardian(guardian)
	if m, ok := agent.ParseMode(*modeFlag); ok {
		companion.SetMode(m, "system")
	}

	pyPred := intelligence.NewPythonPredictor(*dbPath)
	companion.SetPythonPredictor(pyPred)
	storageMon := intelligence.NewStorageMonitor()
	srv := ipc.NewServer(*sockPath, companion, st)
	srv.SetWorkflowEngine(workflow.NewEngine(bus, reg, st))
	srv.SetRuntimeProviders(gov, pred, pyPred, obs)
	srv.SetCognitiveEngine(nce)
	srv.SetStorageMonitor(storageMon)
	if err := srv.Start(); err != nil {
		log.Fatalf("start ipc server: %v", err)
	}
	defer srv.Stop()

	bus.SubscribeAll(func(ev events.Event) {
		srv.BroadcastActivity(fmt.Sprintf("[%s] %s -> %s: %s", ev.Type, ev.Source, ev.Target, ev.Summary))
	})

	_ = st.AppendAuditLog("system", "daemon_start", *sockPath, "ok", "SYNTRIX runtime online")
	log.Printf("syntrixd online (sock=%s, db=%s, mode=%s)", *sockPath, *dbPath, companion.Mode())

	// Warm-up sample so delta-based CPU/network metrics and Python ML are ready immediately.
	if snap, err := collector.Collect(12); err == nil {
		companion.UpdateSnapshot(snap)
		_ = pyPred.EvaluateCached(snap)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	sampleCount := 0
	var lastStorageAlertAt time.Time
	var lastLearnIndexAt time.Time
	for {
		mode := companion.Mode()
		waitDur := *interval

		if mode != agent.ModeOff {
			t0 := time.Now()
			snap, err := collector.Collect(12)
			obs.RecordCollectorUs(uint64(time.Since(t0).Microseconds()))
			if err != nil {
				log.Printf("collect error: %v", err)
			} else {
				companion.UpdateSnapshot(snap)
				govStatus := gov.Evaluate(snap)
				sampleCount++

				if switched, nextMode, reason := companion.EvaluateDynamicMode(snap, string(govStatus.State)); switched {
					mode = nextMode
					srv.BroadcastActivity(fmt.Sprintf("[AUTO] Switched mode -> %s (%s)", nextMode, reason))
				}

				if mode == agent.ModeLearning {
					pyPred.SetMinInterval(10 * time.Second)
					if govStatus.AllowIndexing && (lastLearnIndexAt.IsZero() || time.Since(lastLearnIndexAt) >= 5*time.Minute) {
						if sup := companion.GetSupervisor(); sup != nil {
							lastLearnIndexAt = time.Now()
							go func() {
								if count, err := sup.IndexDirectory("."); err == nil && count > 0 {
									docs, chunks, edges := sup.RAG().Stats()
									srv.BroadcastActivity(fmt.Sprintf("[LEARNING] Indexed %d files (%d docs, %d chunks, %d graph edges)", count, docs, chunks, edges))
								}
							}()
						}
					}
				} else {
					pyPred.SetMinInterval(20 * time.Second)
				}

				if mode != agent.ModeMonitor && govStatus.State != governor.StateCritical {
					recs := analyzer.Ingest(snap)
					for _, rec := range recs {
						_ = st.UpsertRecommendation(rec)
						srv.BroadcastActivity("Detected: " + rec.Title)
					}
					activeApp, activeSince, lastActive := companion.ActiveContextInfo()
					if rec, notif := inactiveMon.Evaluate(snap, activeApp, activeSince, lastActive); rec != nil && notif != nil {
						_ = st.UpsertRecommendation(*rec)
						srv.BroadcastActivity("Inactive App Memory: " + rec.Title)
						srv.BroadcastNotification(notif)
					}
					storageAudit := storageMon.Evaluate(snap)
					pendingRecs, _ := st.ListRecommendations(10)
					hasPendingStorage := false
					for _, r := range pendingRecs {
						if r.Kind == "storage_pressure" && (r.Status == "" || r.Status == "pending") {
							hasPendingStorage = true
							break
						}
					}
					if storageAudit.ExceedsLimit && storageAudit.ReclaimableMB >= 50.0 &&
						(lastStorageAlertAt.IsZero() || time.Since(lastStorageAlertAt) >= 15*time.Minute) {
						lastStorageAlertAt = time.Now()
						recID := fmt.Sprintf("storage-clean-%d", lastStorageAlertAt.Unix())
						if !hasPendingStorage {
							_ = st.UpsertRecommendation(store.Recommendation{
								ID:           recID,
								CreatedAtMs:  lastStorageAlertAt.UnixMilli(),
								Kind:         "storage_pressure",
								Severity:     "medium",
								Title:        fmt.Sprintf("Storage %.1f%% (>50%%) — %.0f MB cleanable", storageAudit.UsedPct, storageAudit.ReclaimableMB),
								Detail:       storageAudit.Summary,
								TargetPID:    0,
								TargetName:   "Storage",
								Status:       "pending",
								BeforeMetric: fmt.Sprintf("disk_used=%.1f%%, reclaimable=%.0fMB", storageAudit.UsedPct, storageAudit.ReclaimableMB),
							})
							pendingRecs, _ = st.ListRecommendations(10)
						}
						srv.BroadcastActivity(storageAudit.Summary)
						srv.BroadcastNotification(&agent.InactiveAppNotification{
							ID:         fmt.Sprintf("storage-50-%d", lastStorageAlertAt.UnixMilli()),
							Title:      fmt.Sprintf("Storage %.1f%% (>50%% Limit)", storageAudit.UsedPct),
							Subtitle:   fmt.Sprintf("%.0f MB unused caches & files found", storageAudit.ReclaimableMB),
							Body:       fmt.Sprintf("%s. Review & Approve in OPTIMIZATIONS to remove stale caches/logs safely.", storageAudit.Summary),
							RecID:      recID,
							TargetPID:  0,
							TargetName: "Storage",
							RSSMB:      storageAudit.ReclaimableMB,
							ActiveApp:  activeApp,
						})
					}
					if len(pendingRecs) > 0 || govStatus.AllowInference {
						var pyFC *intelligence.PythonForecast
						if govStatus.AllowInference {
							pyFC = pyPred.EvaluateCached(snap)
						}
						if actedID, _, didAct := guardian.EvaluateWithForecast(mode, snap, pendingRecs, pyFC); didAct {
							srv.BroadcastActivity("Guardian fulfilled: " + actedID)
						}
					}
				}

				// Persist to SQLite every 2nd sample in LEARNING mode, or every 5th sample otherwise.
				persistEvery := 5
				if mode == agent.ModeLearning {
					persistEvery = 2
				}
				if sampleCount%persistEvery == 0 {
					w0 := time.Now()
					_ = st.InsertSnapshot(snap)
					obs.RecordSQLiteWriteUs(uint64(time.Since(w0).Microseconds()))
				}

				srv.BroadcastTelemetry(snap)
				waitDur = gov.SamplingInterval(*interval)
			}
		} else {
			waitDur = 500 * time.Millisecond
		}

		select {
		case <-sigCh:
			log.Println("syntrixd shutting down...")
			_ = st.AppendAuditLog("system", "daemon_stop", *sockPath, "ok", "graceful shutdown")
			return
		case <-time.After(waitDur):
		}
	}
}
