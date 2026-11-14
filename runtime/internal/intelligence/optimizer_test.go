package intelligence

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/syntrix/runtime/internal/engine"
	"github.com/syntrix/runtime/internal/store"
)

func TestProtectedProcesses(t *testing.T) {
	if !IsProtectedProcess(1, "launchd") {
		t.Fatalf("expected PID 1 launchd to be protected")
	}
	if !IsProtectedProcess(500, "WindowServer") {
		t.Fatalf("expected WindowServer to be protected")
	}
	if IsProtectedProcess(4242, "clang") {
		t.Fatalf("expected user process clang (PID 4242) not to be protected")
	}
}

func TestCleanStaleCachesInDirAndPreview(t *testing.T) {
	root := t.TempDir()
	appCache := filepath.Join(root, "com.example.browser")
	appleCache := filepath.Join(root, "com.apple.Safari")
	if err := os.MkdirAll(appCache, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(appleCache, 0o755); err != nil {
		t.Fatal(err)
	}

	staleFile := filepath.Join(appCache, "old.tmp")
	freshFile := filepath.Join(appCache, "new.tmp")
	appleFile := filepath.Join(appleCache, "old_apple.tmp")

	data := make([]byte, 4096)
	for _, f := range []string{staleFile, freshFile, appleFile} {
		if err := os.WriteFile(f, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	_ = os.Chtimes(staleFile, oldTime, oldTime)
	_ = os.Chtimes(appleFile, oldTime, oldTime)

	previews := PreviewStaleCachesInDir(root, 7*24*time.Hour, 5)
	if len(previews) != 1 || previews[0].OwnerApp != "com.example.browser" || previews[0].FileCount != 1 {
		t.Fatalf("unexpected cache preview: %+v", previews)
	}

	reclaimed := CleanStaleCachesInDir(root, 7*24*time.Hour)
	if reclaimed != 4096 {
		t.Fatalf("expected 4096 bytes reclaimed, got %d", reclaimed)
	}
	if _, err := os.Stat(staleFile); !os.IsNotExist(err) {
		t.Fatalf("expected stale file to be removed")
	}
	if _, err := os.Stat(freshFile); err != nil {
		t.Fatalf("expected fresh file to be preserved: %v", err)
	}
	if _, err := os.Stat(appleFile); err != nil {
		t.Fatalf("expected com.apple.* cache file to be preserved: %v", err)
	}
}

func TestOptimizationDryRunAndKillSwitch(t *testing.T) {
	ctrl := NewOptimizationController()
	rec := store.Recommendation{
		ID:         "cpu-spike-999999",
		Kind:       "cpu_spike",
		TargetPID:  999999,
		TargetName: "clang",
	}

	dry := ctrl.Execute(context.Background(), rec, true, engine.Snapshot{}, nil)
	if dry.Executed || dry.Cancelled {
		t.Fatalf("expected dry-run not to execute or cancel: %+v", dry)
	}

	ctrl.ActivateKillSwitch()
	blocked := ctrl.Execute(context.Background(), rec, false, engine.Snapshot{}, nil)
	if !blocked.Cancelled || blocked.Executed {
		t.Fatalf("expected kill switch to cancel execution: %+v", blocked)
	}
	ctrl.ResetKillSwitch()
}
