package intelligence

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/syntrix/runtime/internal/engine"
)

func TestStorageMonitor50PctLimitAndUnusedScan(t *testing.T) {
	tmpHome := t.TempDir()
	cacheDir := filepath.Join(tmpHome, "Library", "Caches", "com.example.heavyapp")
	downloadsDir := filepath.Join(tmpHome, "Downloads")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(downloadsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write a 6MB stale cache file (>3 days old)
	staleCache := filepath.Join(cacheDir, "cache.bin")
	if err := os.WriteFile(staleCache, make([]byte, 6*1024*1024), 0o644); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-5 * 24 * time.Hour)
	_ = os.Chtimes(staleCache, oldTime, oldTime)

	// Write a 55MB unused installer (>14 days old)
	oldDmg := filepath.Join(downloadsDir, "OldInstaller.dmg")
	if err := os.WriteFile(oldDmg, make([]byte, 55*1024*1024), 0o644); err != nil {
		t.Fatal(err)
	}
	veryOld := time.Now().Add(-20 * 24 * time.Hour)
	_ = os.Chtimes(oldDmg, veryOld, veryOld)

	mon := NewStorageMonitor()
	mon.homeDir = tmpHome

	snap := engine.Snapshot{
		Disk: engine.DiskMetrics{
			UsedPct:    62.5,
			FreeBytes:  90 * 1024 * 1024 * 1024,
			TotalBytes: 256 * 1024 * 1024 * 1024,
		},
	}

	rep := mon.Evaluate(snap)
	if !rep.ExceedsLimit {
		t.Fatalf("expected ExceedsLimit=true at 62.5%% (>50%%)")
	}
	if rep.ThresholdPct != 50.0 {
		t.Fatalf("expected ThresholdPct=50.0, got %.1f", rep.ThresholdPct)
	}
	if len(rep.Candidates) < 2 {
		t.Fatalf("expected >=2 cleanup candidates, got %d: %+v", len(rep.Candidates), rep.Candidates)
	}
	if rep.ReclaimableMB < 60.0 {
		t.Fatalf("expected ReclaimableMB >= 60MB, got %.1f", rep.ReclaimableMB)
	}
}
