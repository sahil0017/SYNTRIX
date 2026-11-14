# SYNTRIX

**Your Mac. More intelligent.**

Native macOS desktop intelligence combining a **C17 system engine**, a **Go local runtime** (`syntrixd`) with **WAL-mode SQLite persistence**, **Python ML forecasting & Reinforcement Learning**, and a **dark-first SwiftUI HUD** (`SYNTRIX.app`).

---

## Core Capabilities

- **C17 Native System Engine (`engine/`):**
  - `cpu.c`: Per-core and aggregate CPU utilization via Mach `host_processor_info`
  - `memory.c`: Active, wired, compressed, swap, and memory pressure via `host_statistics64` and `sysctl`
  - `process.c`: Live process list, RSS memory, thread count, and delta CPU% via `libproc`
  - `system.c`: Disk (`statfs`), network throughput (`getifaddrs`), and battery/AC power (`IOKit`)
- **Always-On Guardian Agent & RL (`runtime/internal/agent/`):**
  - Monitors system health continuously regardless of user idle state.
  - **Transformer + Q-Learning RL (`ml/transformer_rl.py`)** trained on live SQLite telemetry snapshots (`96.31%` accuracy) to proactively prevent CPU/memory spikes before crossing thresholds.
  - **50% Storage Ceiling Guard:** Monitors disk usage and uses `LogisticStaleClassifier-v1` (heuristic-logistic stale/unused file classifier across caches, logs, Xcode `DerivedData`, and old installers) to surface cleanable storage recommendations and macOS notifications.
  - **Inactive-App Memory Alerts:** Tracks active `NSWorkspace` focus and sends macOS notifications when background apps hold excessive memory while idle.
  - **Generic App Hang & Process-Tree Diagnosis (`analyzer.go`):** Detects sustained single-process CPU/thread spins and multi-process helper tree bloat (browsers, Electron/IDE helpers) generically across any macOS app.
- **Dynamic Operating Modes (`AUTO`, `OFF`, `MONITOR`, `ASSIST`, `AUTOPILOT`, `LEARNING`):**
  - **`AUTO`:** Dynamically switches between `ASSIST`, `AUTOPILOT` (high memory/disk pressure), and `LEARNING` (active dev/knowledge apps with headroom).
  - **`AUTOPILOT`:** Raises Guardian's autonomous rate cap to `12/hr` with `90s` cooldowns.
  - **`LEARNING`:** Increases SQLite snapshot cadence (`2x`), speeds up Python ML retraining/inference, and runs background workspace RAG/knowledge-graph indexing.
- **Offline Companion, RAG & Workflows (`runtime/internal/research/`, `workflow/`):**
  - Natural-language chat reasoning backed by local Developer Inspection, SQLite Vector RAG + Knowledge Graph, and conditional multi-step workflow execution.
- **Native SwiftUI HUD (`app/`):**
  - Overview, Assistant, Telemetry, Research, Workflows, and Security/Audit Trail panels connected over Unix domain socket NDJSON (`/tmp/syntrixd.sock`).

---

## Build, Test & Run

```sh
# Build C17 engine, Go daemon, and SwiftUI app bundle (build/SYNTRIX.app)
make bundle

# Run C17 & Go tests
make test

# Launch the bundled macOS app (automatically manages syntrixd)
open build/SYNTRIX.app
```

Or run components individually in development:

```sh
./runtime/build/syntrixd
./app/.build/debug/SyntrixApp
```
