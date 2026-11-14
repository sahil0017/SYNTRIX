package observability

import (
	"runtime"
	"sync"
	"sync/atomic"
)

type HealthSnapshot struct {
	CollectorLatencyUs uint64  `json:"collector_latency_us"`
	SQLiteWriteUs      uint64  `json:"sqlite_write_us"`
	EventCount         uint64  `json:"event_count"`
	HeapAllocMB        float64 `json:"heap_alloc_mb"`
	Goroutines         int     `json:"goroutines"`
}

type Tracker struct {
	collectorUs atomic.Uint64
	sqliteUs    atomic.Uint64
	eventCount  atomic.Uint64
	mu          sync.Mutex
}

func NewTracker() *Tracker {
	return &Tracker{}
}

func (t *Tracker) RecordCollectorUs(us uint64) {
	t.collectorUs.Store(us)
}

func (t *Tracker) RecordSQLiteWriteUs(us uint64) {
	t.sqliteUs.Store(us)
}

func (t *Tracker) IncEvents() {
	t.eventCount.Add(1)
}

func (t *Tracker) Snapshot() HealthSnapshot {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return HealthSnapshot{
		CollectorLatencyUs: t.collectorUs.Load(),
		SQLiteWriteUs:      t.sqliteUs.Load(),
		EventCount:         t.eventCount.Load(),
		HeapAllocMB:        float64(ms.HeapAlloc) / (1024 * 1024),
		Goroutines:         runtime.NumGoroutine(),
	}
}
