package store

/*
#cgo LDFLAGS: -lsqlite3
#include <sqlite3.h>
#include <stdlib.h>

static int syntrix_sqlite_exec(sqlite3 *db, const char *sql, char **err_msg) {
    return sqlite3_exec(db, sql, NULL, NULL, err_msg);
}
*/
import "C"
import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unsafe"

	"github.com/syntrix/runtime/internal/engine"
)

type Recommendation struct {
	ID           string `json:"id"`
	CreatedAtMs  int64  `json:"created_at_ms"`
	Kind         string `json:"kind"`
	Severity     string `json:"severity"`
	Title        string `json:"title"`
	Detail       string `json:"detail"`
	TargetPID    int32  `json:"target_pid"`
	TargetName   string `json:"target_name"`
	Status       string `json:"status"`
	BeforeMetric string `json:"before_metric"`
	AfterMetric  string `json:"after_metric"`
}

type AuditEntry struct {
	ID          int64  `json:"id"`
	TimestampMs int64  `json:"timestamp_ms"`
	Actor       string `json:"actor"`
	Action      string `json:"action"`
	Target      string `json:"target"`
	Status      string `json:"status"`
	Detail      string `json:"detail"`
}

type SnapshotRecord struct {
	TimestampMs    int64   `json:"timestamp_ms"`
	CPUPct         float64 `json:"cpu_pct"`
	MemUsedBytes   uint64  `json:"mem_used_bytes"`
	MemTotalBytes  uint64  `json:"mem_total_bytes"`
	MemPressurePct float64 `json:"mem_pressure_pct"`
	SwapUsedBytes  uint64  `json:"swap_used_bytes"`
	DiskUsedPct    float64 `json:"disk_used_pct"`
	BatteryPct     float64 `json:"battery_pct"`
}

type KnowledgeDoc struct {
	Path        string `json:"path"`
	Title       string `json:"title"`
	Snippet     string `json:"snippet"`
	IndexedAtMs int64  `json:"indexed_at_ms"`
}

type WorkflowRun struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	CurrentStep int    `json:"current_step"`
	TotalSteps  int    `json:"total_steps"`
	Output      string `json:"output"`
	UpdatedAtMs int64  `json:"updated_at_ms"`
}

type Store struct {
	mu           sync.Mutex
	db           *C.sqlite3
	snapInserts  uint64
	auditInserts uint64
}

func Open(dbPath string) (*Store, error) {
	if dbPath != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}

	cPath := C.CString(dbPath)
	defer C.free(unsafe.Pointer(cPath))

	var db *C.sqlite3
	if rc := C.sqlite3_open(cPath, &db); rc != C.SQLITE_OK {
		msg := C.GoString(C.sqlite3_errmsg(db))
		if db != nil {
			C.sqlite3_close(db)
		}
		return nil, fmt.Errorf("sqlite3_open: %s", msg)
	}

	s := &Store{db: db}
	if err := s.initSchema(); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		C.sqlite3_close(s.db)
		s.db = nil
	}
}

func (s *Store) execLocked(sql string) error {
	cSQL := C.CString(sql)
	defer C.free(unsafe.Pointer(cSQL))

	var errMsg *C.char
	if rc := C.syntrix_sqlite_exec(s.db, cSQL, &errMsg); rc != C.SQLITE_OK {
		msg := C.GoString(errMsg)
		C.sqlite3_free(unsafe.Pointer(errMsg))
		return fmt.Errorf("sqlite exec failed: %s", msg)
	}
	return nil
}

func (s *Store) initSchema() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	pragmas := `
PRAGMA journal_mode=WAL;
PRAGMA synchronous=NORMAL;
PRAGMA busy_timeout=3000;
`
	if err := s.execLocked(pragmas); err != nil {
		return err
	}

	schema := `
CREATE TABLE IF NOT EXISTS snapshots (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	timestamp_ms INTEGER NOT NULL,
	cpu_pct REAL NOT NULL,
	mem_used_bytes INTEGER NOT NULL,
	mem_total_bytes INTEGER NOT NULL,
	mem_pressure_pct REAL NOT NULL,
	swap_used_bytes INTEGER NOT NULL,
	disk_used_pct REAL NOT NULL,
	battery_pct REAL NOT NULL,
	top_procs_json TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_snapshots_ts ON snapshots(timestamp_ms);

CREATE TABLE IF NOT EXISTS recommendations (
	id TEXT PRIMARY KEY,
	created_at_ms INTEGER NOT NULL,
	kind TEXT NOT NULL,
	severity TEXT NOT NULL,
	title TEXT NOT NULL,
	detail TEXT NOT NULL,
	target_pid INTEGER NOT NULL,
	target_name TEXT NOT NULL,
	status TEXT NOT NULL,
	before_metric TEXT NOT NULL,
	after_metric TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_logs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	timestamp_ms INTEGER NOT NULL,
	actor TEXT NOT NULL,
	action TEXT NOT NULL,
	target TEXT NOT NULL,
	status TEXT NOT NULL,
	detail TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_ts ON audit_logs(timestamp_ms);

CREATE TABLE IF NOT EXISTS knowledge_docs (
	path TEXT PRIMARY KEY,
	title TEXT NOT NULL,
	content TEXT NOT NULL,
	indexed_at_ms INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS workflow_runs (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	status TEXT NOT NULL,
	current_step INTEGER NOT NULL,
	total_steps INTEGER NOT NULL,
	output TEXT NOT NULL,
	updated_at_ms INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS event_journal (
	seq INTEGER PRIMARY KEY,
	event_id TEXT NOT NULL,
	schema_version INTEGER NOT NULL,
	priority INTEGER NOT NULL,
	type TEXT NOT NULL,
	correlation_id TEXT NOT NULL,
	source TEXT NOT NULL,
	target TEXT NOT NULL,
	summary TEXT NOT NULL,
	timestamp_ms INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_journal_ts ON event_journal(timestamp_ms);
`
	return s.execLocked(schema)
}

func (s *Store) prepareLocked(sql string) (*C.sqlite3_stmt, error) {
	cSQL := C.CString(sql)
	defer C.free(unsafe.Pointer(cSQL))

	var stmt *C.sqlite3_stmt
	if rc := C.sqlite3_prepare_v2(s.db, cSQL, -1, &stmt, nil); rc != C.SQLITE_OK {
		return nil, fmt.Errorf("sqlite prepare: %s", C.GoString(C.sqlite3_errmsg(s.db)))
	}
	return stmt, nil
}

func bindText(stmt *C.sqlite3_stmt, idx C.int, val string) {
	cStr := C.CString(val)
	C.sqlite3_bind_text(stmt, idx, cStr, -1, (*[0]byte)(C.free))
}

func (s *Store) InsertSnapshot(snap engine.Snapshot) error {
	topJSON, err := json.Marshal(snap.TopProcesses)
	if err != nil {
		topJSON = []byte("[]")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return fmt.Errorf("store closed")
	}

	stmt, err := s.prepareLocked(`
		INSERT INTO snapshots (
			timestamp_ms, cpu_pct, mem_used_bytes, mem_total_bytes,
			mem_pressure_pct, swap_used_bytes, disk_used_pct, battery_pct, top_procs_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer C.sqlite3_finalize(stmt)

	C.sqlite3_bind_int64(stmt, 1, C.sqlite3_int64(snap.TimestampMs))
	C.sqlite3_bind_double(stmt, 2, C.double(snap.CPU.TotalUsagePct))
	C.sqlite3_bind_int64(stmt, 3, C.sqlite3_int64(snap.Memory.UsedBytes))
	C.sqlite3_bind_int64(stmt, 4, C.sqlite3_int64(snap.Memory.TotalBytes))
	C.sqlite3_bind_double(stmt, 5, C.double(snap.Memory.PressurePct))
	C.sqlite3_bind_int64(stmt, 6, C.sqlite3_int64(snap.Memory.SwapUsedBytes))
	C.sqlite3_bind_double(stmt, 7, C.double(snap.Disk.UsedPct))
	C.sqlite3_bind_double(stmt, 8, C.double(snap.Power.BatteryPct))
	bindText(stmt, 9, string(topJSON))

	if rc := C.sqlite3_step(stmt); rc != C.SQLITE_DONE {
		return fmt.Errorf("insert snapshot: %s", C.GoString(C.sqlite3_errmsg(s.db)))
	}

	// Keep bounded history (last 5000 snapshots), pruned periodically to avoid write amplification.
	s.snapInserts++
	if s.snapInserts%50 == 0 {
		_ = s.execLocked(`DELETE FROM snapshots WHERE id NOT IN (SELECT id FROM snapshots ORDER BY id DESC LIMIT 5000);`)
	}
	return nil
}

func (s *Store) RecentSnapshots(limit int) ([]SnapshotRecord, error) {
	if limit <= 0 {
		limit = 60
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil, fmt.Errorf("store closed")
	}

	stmt, err := s.prepareLocked(`
		SELECT timestamp_ms, cpu_pct, mem_used_bytes, mem_total_bytes,
		       mem_pressure_pct, swap_used_bytes, disk_used_pct, battery_pct
		FROM snapshots
		ORDER BY timestamp_ms DESC
		LIMIT ?
	`)
	if err != nil {
		return nil, err
	}
	defer C.sqlite3_finalize(stmt)

	C.sqlite3_bind_int(stmt, 1, C.int(limit))

	var out []SnapshotRecord
	for {
		rc := C.sqlite3_step(stmt)
		if rc == C.SQLITE_ROW {
			out = append(out, SnapshotRecord{
				TimestampMs:    int64(C.sqlite3_column_int64(stmt, 0)),
				CPUPct:         float64(C.sqlite3_column_double(stmt, 1)),
				MemUsedBytes:   uint64(C.sqlite3_column_int64(stmt, 2)),
				MemTotalBytes:  uint64(C.sqlite3_column_int64(stmt, 3)),
				MemPressurePct: float64(C.sqlite3_column_double(stmt, 4)),
				SwapUsedBytes:  uint64(C.sqlite3_column_int64(stmt, 5)),
				DiskUsedPct:    float64(C.sqlite3_column_double(stmt, 6)),
				BatteryPct:     float64(C.sqlite3_column_double(stmt, 7)),
			})
		} else if rc == C.SQLITE_DONE {
			break
		} else {
			return nil, fmt.Errorf("query snapshots: %s", C.GoString(C.sqlite3_errmsg(s.db)))
		}
	}
	return out, nil
}

func (s *Store) UpsertRecommendation(rec Recommendation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return fmt.Errorf("store closed")
	}

	stmt, err := s.prepareLocked(`
		INSERT INTO recommendations (
			id, created_at_ms, kind, severity, title, detail,
			target_pid, target_name, status, before_metric, after_metric
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			created_at_ms = excluded.created_at_ms,
			severity = excluded.severity,
			detail = excluded.detail,
			before_metric = excluded.before_metric
		WHERE recommendations.status = 'pending'
	`)
	if err != nil {
		return err
	}
	defer C.sqlite3_finalize(stmt)

	bindText(stmt, 1, rec.ID)
	C.sqlite3_bind_int64(stmt, 2, C.sqlite3_int64(rec.CreatedAtMs))
	bindText(stmt, 3, rec.Kind)
	bindText(stmt, 4, rec.Severity)
	bindText(stmt, 5, rec.Title)
	bindText(stmt, 6, rec.Detail)
	C.sqlite3_bind_int(stmt, 7, C.int(rec.TargetPID))
	bindText(stmt, 8, rec.TargetName)
	bindText(stmt, 9, rec.Status)
	bindText(stmt, 10, rec.BeforeMetric)
	bindText(stmt, 11, rec.AfterMetric)

	if rc := C.sqlite3_step(stmt); rc != C.SQLITE_DONE {
		return fmt.Errorf("upsert recommendation: %s", C.GoString(C.sqlite3_errmsg(s.db)))
	}
	return nil
}

func (s *Store) UpdateRecommendationStatus(id, status, afterMetric string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return fmt.Errorf("store closed")
	}

	stmt, err := s.prepareLocked(`
		UPDATE recommendations
		SET status = ?, after_metric = ?
		WHERE id = ?
	`)
	if err != nil {
		return err
	}
	defer C.sqlite3_finalize(stmt)

	bindText(stmt, 1, status)
	bindText(stmt, 2, afterMetric)
	bindText(stmt, 3, id)

	if rc := C.sqlite3_step(stmt); rc != C.SQLITE_DONE {
		return fmt.Errorf("update recommendation: %s", C.GoString(C.sqlite3_errmsg(s.db)))
	}
	return nil
}

func (s *Store) GetRecommendation(id string) (Recommendation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return Recommendation{}, fmt.Errorf("store closed")
	}

	stmt, err := s.prepareLocked(`
		SELECT id, created_at_ms, kind, severity, title, detail,
		       target_pid, target_name, status, before_metric, after_metric
		FROM recommendations
		WHERE id = ?
		LIMIT 1
	`)
	if err != nil {
		return Recommendation{}, err
	}
	defer C.sqlite3_finalize(stmt)

	bindText(stmt, 1, id)
	if rc := C.sqlite3_step(stmt); rc == C.SQLITE_ROW {
		return Recommendation{
			ID:           C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 0)))),
			CreatedAtMs:  int64(C.sqlite3_column_int64(stmt, 1)),
			Kind:         C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 2)))),
			Severity:     C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 3)))),
			Title:        C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 4)))),
			Detail:       C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 5)))),
			TargetPID:    int32(C.sqlite3_column_int(stmt, 6)),
			TargetName:   C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 7)))),
			Status:       C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 8)))),
			BeforeMetric: C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 9)))),
			AfterMetric:  C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 10)))),
		}, nil
	}
	return Recommendation{}, fmt.Errorf("recommendation %q not found", id)
}

func (s *Store) ListRecommendations(limit int) ([]Recommendation, error) {
	if limit <= 0 {
		limit = 20
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil, fmt.Errorf("store closed")
	}

	// Expire stale pending recommendations older than 10 minutes so dead-process alerts don't pile up.
	staleCutoffMs := time.Now().Add(-10 * time.Minute).UnixMilli()
	_ = s.execLocked(fmt.Sprintf("DELETE FROM recommendations WHERE status = 'pending' AND created_at_ms < %d;", staleCutoffMs))

	stmt, err := s.prepareLocked(`
		SELECT id, created_at_ms, kind, severity, title, detail,
		       target_pid, target_name, status, before_metric, after_metric
		FROM recommendations
		WHERE status != 'dismissed'
		ORDER BY CASE WHEN status = 'pending' THEN 0 ELSE 1 END, created_at_ms DESC
		LIMIT ?
	`)
	if err != nil {
		return nil, err
	}
	defer C.sqlite3_finalize(stmt)

	C.sqlite3_bind_int(stmt, 1, C.int(limit))

	var out []Recommendation
	for {
		rc := C.sqlite3_step(stmt)
		if rc == C.SQLITE_ROW {
			out = append(out, Recommendation{
				ID:           C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 0)))),
				CreatedAtMs:  int64(C.sqlite3_column_int64(stmt, 1)),
				Kind:         C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 2)))),
				Severity:     C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 3)))),
				Title:        C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 4)))),
				Detail:       C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 5)))),
				TargetPID:    int32(C.sqlite3_column_int(stmt, 6)),
				TargetName:   C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 7)))),
				Status:       C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 8)))),
				BeforeMetric: C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 9)))),
				AfterMetric:  C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 10)))),
			})
		} else if rc == C.SQLITE_DONE {
			break
		} else {
			return nil, fmt.Errorf("list recommendations: %s", C.GoString(C.sqlite3_errmsg(s.db)))
		}
	}
	return out, nil
}

func (s *Store) AppendAuditLog(actor, action, target, status, detail string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return fmt.Errorf("store closed")
	}

	stmt, err := s.prepareLocked(`
		INSERT INTO audit_logs (timestamp_ms, actor, action, target, status, detail)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer C.sqlite3_finalize(stmt)

	C.sqlite3_bind_int64(stmt, 1, C.sqlite3_int64(time.Now().UnixMilli()))
	bindText(stmt, 2, actor)
	bindText(stmt, 3, action)
	bindText(stmt, 4, target)
	bindText(stmt, 5, status)
	bindText(stmt, 6, detail)

	if rc := C.sqlite3_step(stmt); rc != C.SQLITE_DONE {
		return fmt.Errorf("append audit log: %s", C.GoString(C.sqlite3_errmsg(s.db)))
	}
	s.auditInserts++
	if s.auditInserts%100 == 0 {
		_ = s.execLocked(`DELETE FROM audit_logs WHERE id NOT IN (SELECT id FROM audit_logs ORDER BY id DESC LIMIT 5000);`)
	}
	return nil
}

func (s *Store) ListAuditLogs(limit int) ([]AuditEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil, fmt.Errorf("store closed")
	}

	stmt, err := s.prepareLocked(`
		SELECT id, timestamp_ms, actor, action, target, status, detail
		FROM audit_logs
		ORDER BY id DESC
		LIMIT ?
	`)
	if err != nil {
		return nil, err
	}
	defer C.sqlite3_finalize(stmt)

	C.sqlite3_bind_int(stmt, 1, C.int(limit))

	var out []AuditEntry
	for {
		rc := C.sqlite3_step(stmt)
		if rc == C.SQLITE_ROW {
			out = append(out, AuditEntry{
				ID:          int64(C.sqlite3_column_int64(stmt, 0)),
				TimestampMs: int64(C.sqlite3_column_int64(stmt, 1)),
				Actor:       C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 2)))),
				Action:      C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 3)))),
				Target:      C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 4)))),
				Status:      C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 5)))),
				Detail:      C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 6)))),
			})
		} else if rc == C.SQLITE_DONE {
			break
		} else {
			return nil, fmt.Errorf("list audit logs: %s", C.GoString(C.sqlite3_errmsg(s.db)))
		}
	}
	return out, nil
}

func (s *Store) UpsertKnowledgeDoc(path, title, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return fmt.Errorf("store closed")
	}
	stmt, err := s.prepareLocked(`
		INSERT INTO knowledge_docs (path, title, content, indexed_at_ms)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			title = excluded.title,
			content = excluded.content,
			indexed_at_ms = excluded.indexed_at_ms
	`)
	if err != nil {
		return err
	}
	defer C.sqlite3_finalize(stmt)

	bindText(stmt, 1, path)
	bindText(stmt, 2, title)
	bindText(stmt, 3, content)
	C.sqlite3_bind_int64(stmt, 4, C.sqlite3_int64(time.Now().UnixMilli()))

	if rc := C.sqlite3_step(stmt); rc != C.SQLITE_DONE {
		return fmt.Errorf("upsert knowledge doc: %s", C.GoString(C.sqlite3_errmsg(s.db)))
	}
	return nil
}

func (s *Store) SearchKnowledgeDocs(query string, limit int) ([]KnowledgeDoc, error) {
	if limit <= 0 {
		limit = 10
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil, fmt.Errorf("store closed")
	}

	stmt, err := s.prepareLocked(`
		SELECT path, title, substr(content, 1, 220), indexed_at_ms
		FROM knowledge_docs
		WHERE content LIKE ? OR title LIKE ? OR path LIKE ?
		ORDER BY indexed_at_ms DESC
		LIMIT ?
	`)
	if err != nil {
		return nil, err
	}
	defer C.sqlite3_finalize(stmt)

	pat := "%" + query + "%"
	bindText(stmt, 1, pat)
	bindText(stmt, 2, pat)
	bindText(stmt, 3, pat)
	C.sqlite3_bind_int(stmt, 4, C.int(limit))

	var out []KnowledgeDoc
	for {
		rc := C.sqlite3_step(stmt)
		if rc == C.SQLITE_ROW {
			out = append(out, KnowledgeDoc{
				Path:        C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 0)))),
				Title:       C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 1)))),
				Snippet:     C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 2)))),
				IndexedAtMs: int64(C.sqlite3_column_int64(stmt, 3)),
			})
		} else if rc == C.SQLITE_DONE {
			break
		} else {
			return nil, fmt.Errorf("search knowledge docs: %s", C.GoString(C.sqlite3_errmsg(s.db)))
		}
	}
	return out, nil
}

func (s *Store) UpsertWorkflowRun(run WorkflowRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return fmt.Errorf("store closed")
	}
	stmt, err := s.prepareLocked(`
		INSERT INTO workflow_runs (id, name, status, current_step, total_steps, output, updated_at_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			status = excluded.status,
			current_step = excluded.current_step,
			total_steps = excluded.total_steps,
			output = excluded.output,
			updated_at_ms = excluded.updated_at_ms
	`)
	if err != nil {
		return err
	}
	defer C.sqlite3_finalize(stmt)

	bindText(stmt, 1, run.ID)
	bindText(stmt, 2, run.Name)
	bindText(stmt, 3, run.Status)
	C.sqlite3_bind_int(stmt, 4, C.int(run.CurrentStep))
	C.sqlite3_bind_int(stmt, 5, C.int(run.TotalSteps))
	bindText(stmt, 6, run.Output)
	C.sqlite3_bind_int64(stmt, 7, C.sqlite3_int64(run.UpdatedAtMs))

	if rc := C.sqlite3_step(stmt); rc != C.SQLITE_DONE {
		return fmt.Errorf("upsert workflow run: %s", C.GoString(C.sqlite3_errmsg(s.db)))
	}
	return nil
}

func (s *Store) ListWorkflowRuns(limit int) ([]WorkflowRun, error) {
	if limit <= 0 {
		limit = 15
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil, fmt.Errorf("store closed")
	}
	stmt, err := s.prepareLocked(`
		SELECT id, name, status, current_step, total_steps, output, updated_at_ms
		FROM workflow_runs
		ORDER BY updated_at_ms DESC
		LIMIT ?
	`)
	if err != nil {
		return nil, err
	}
	defer C.sqlite3_finalize(stmt)
	C.sqlite3_bind_int(stmt, 1, C.int(limit))

	var out []WorkflowRun
	for {
		rc := C.sqlite3_step(stmt)
		if rc == C.SQLITE_ROW {
			out = append(out, WorkflowRun{
				ID:          C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 0)))),
				Name:        C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 1)))),
				Status:      C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 2)))),
				CurrentStep: int(C.sqlite3_column_int(stmt, 3)),
				TotalSteps:  int(C.sqlite3_column_int(stmt, 4)),
				Output:      C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, 5)))),
				UpdatedAtMs: int64(C.sqlite3_column_int64(stmt, 6)),
			})
		} else if rc == C.SQLITE_DONE {
			break
		} else {
			return nil, fmt.Errorf("list workflow runs: %s", C.GoString(C.sqlite3_errmsg(s.db)))
		}
	}
	return out, nil
}

func (s *Store) InsertEventJournal(
	seq uint64,
	eventID string,
	schemaVersion int,
	priority int,
	evType, corrID, source, target, summary string,
	tsMs int64,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return fmt.Errorf("store closed")
	}
	stmt, err := s.prepareLocked(`
		INSERT OR IGNORE INTO event_journal (
			seq, event_id, schema_version, priority, type, correlation_id, source, target, summary, timestamp_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer C.sqlite3_finalize(stmt)

	C.sqlite3_bind_int64(stmt, 1, C.sqlite3_int64(seq))
	bindText(stmt, 2, eventID)
	C.sqlite3_bind_int(stmt, 3, C.int(schemaVersion))
	C.sqlite3_bind_int(stmt, 4, C.int(priority))
	bindText(stmt, 5, evType)
	bindText(stmt, 6, corrID)
	bindText(stmt, 7, source)
	bindText(stmt, 8, target)
	bindText(stmt, 9, summary)
	C.sqlite3_bind_int64(stmt, 10, C.sqlite3_int64(tsMs))

	if rc := C.sqlite3_step(stmt); rc != C.SQLITE_DONE {
		return fmt.Errorf("insert event journal: %s", C.GoString(C.sqlite3_errmsg(s.db)))
	}
	_ = s.execLocked(`DELETE FROM event_journal WHERE seq NOT IN (SELECT seq FROM event_journal ORDER BY seq DESC LIMIT 2000);`)
	return nil
}
