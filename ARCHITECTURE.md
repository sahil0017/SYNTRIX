# SYNTRIX Architecture (HLD & Event-Driven Flow)

This document defines the 6-layer High-Level Design (HLD) and the event-driven signal flow used as the blueprint for SYNTRIX.

---

## 1. High-Level Design (6 Layers)

```
User
 │
 ▼
┌─────────────────────────────────────────────────────────────────┐
│ 1. SYNTRIX — Native macOS Application (Swift / SwiftUI)         │
│    AI Companion · System Dashboard · Research & Learning        │
│    Settings & Permissions · Agentic Workflow UI                 │
└───────────────────────────┬─────────────────────────────────────┘
                            │ IPC / XPC (Unix Socket NDJSON)
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│ 2. Local Intelligence Platform (Go Runtime)                     │
│  • Telemetry Processing      • Agent Orchestrator               │
│  • Anomaly Detection         • Context & Memory Manager         │
│  • Resource-aware Scheduler  • Tool Registry / MCP              │
│  • Optimization & Policy     • Controlled Action Executor       │
│  • Event Bus (events.Bus)    • SQLite Local Storage (WAL)       │
└──────┬───────────────────────┬──────────────────────────┬───────┘
       │ cgo                   │ Events / Tools           │ Secure HTTPS/MQTT
       ▼                       ▼                          ▼
┌─────────────────────┐ ┌────────────────────────┐ ┌──────────────┐
│ 3. C17 System Engine│ │ 4. Agentic Intelligence│ │ 6. AWS Cloud │
│  • CPU Collector    │ │  • Supervisor Agent    │ │  (Optional)  │
│  • Memory Collector │ │  • Research Agent      │ │  • Cognito   │
│  • Process Collector│ │  • Developer Agent     │ │  • Go Svcs   │
│  • Power & Thermal  │ │  • Learning Agent      │ │  • Bedrock   │
│  • Storage Collector│ │  • System Agent        │ │  • SQS/EB    │
│  • Network Collector│ │  • Workflow Agent      │ │  • S3/DB     │
└─────────┬───────────┘ └────────────────────────┘ └──────────────┘
          │
          ▼
┌─────────────────────────────────────────────────────────────────┐
│ 5. macOS Operating System                                       │
│    Mach · IOKit · sysctl · libproc · FS & Net · Keychain/TCC    │
└─────────────────────────────────────────────────────────────────┘
```

---

## 2. Layer Connections & Boundaries

1. **SwiftUI App ↔ Go Runtime (`IPC / XPC`):**
   - Sends user requests, mode changes, and explicit approvals/dismissals.
   - Receives live `telemetry` snapshots, `chat_reply` messages, and `activity` event-bus logs.
2. **Go Runtime ↔ C17 System Engine (`cgo`):**
   - `Resource-aware Scheduler` triggers C17 collectors at adaptive intervals (paused in `OFF`, backed off under high CPU).
   - `Controlled Action Executor` requests post-action verification snapshots.
3. **C17 System Engine ↔ macOS OS APIs:**
   - `CPU`, `Memory`, `Process`, `Power/Thermal` call `Mach`, `IOKit`, `sysctl`, `libproc`.
   - `Storage` and `Network` call `statfs` and `getifaddrs`.
4. **Go Runtime ↔ Agentic Intelligence:**
   - `Agent Orchestrator` dispatches requests to `Supervisor Agent` and specialist agents (`System`, `Optimization`, `Research`, `Developer`, `Learning`, `Workflow`).
   - Specialist agents operate strictly through `Tool Registry / MCP` and are gated by `Optimization & Policy Engine`.
5. **Go Runtime ↔ SQLite Local Storage:**
   - Offline-first WAL-mode persistence (`~/.syntrix/syntrix.db`) for snapshots, recommendations, and audit logs.
6. **Go Runtime ↔ AWS (Optional):**
   - Off-critical-path sync, backup, and optional Bedrock inference over HTTPS/MQTT. Core monitoring and authorization never depend on AWS.

---

## 3. Event-Driven Signal Flow (Example: Memory Optimization)

1. `user.request.submitted` — `SwiftUI` → `GoRuntime` (`"Analyze my memory usage"` / `/optimize-memory`)
2. `telemetry.snapshot.requested` — `GoRuntime` → `CSystemEngine`
3. `telemetry.snapshot.collected` — `CSystemEngine` → `GoRuntime` (memory + process metrics)
4. `analysis.requested` — `GoRuntime` → `OptimizationAgent`
5. `recommendation.proposed` — `OptimizationAgent` → `GoRuntime` (bottleneck identified)
6. `policy.validation.requested` — `GoRuntime` → `PolicyEngine`
7. `policy.approval.required` — `PolicyEngine` → `GoRuntime`
8. `recommendation.presented` — `GoRuntime` → `SwiftUI` (displayed in `OPTIMIZATION` panel)
9. `action.approval.received` — `SwiftUI` → `GoRuntime` (user clicks **Approve**)
10. `policy.revalidation.requested` → `policy.authorized` — `GoRuntime` ↔ `PolicyEngine`
11. `action.execution.requested` → `action.executed` — `GoRuntime` ↔ `ControlledExecutor`
12. `verification.snapshot.requested` → `verification.snapshot.collected` — `GoRuntime` ↔ `CSystemEngine`
13. `outcome.measured` — `GoRuntime` → `SwiftUI` (before-and-after metrics persisted & displayed)

---

## 4. Implementation Status Map

| Layer / Module | Status | Location |
|---|---|---|
| C17 CPU, Memory, Process, Disk, Net, Battery | Implemented | [engine/src/](file:///Users/sahilkanungo/Desktop/SYNTRIX/engine/src) |
| C17 Thermal / Throttling Collector | Planned | — |
| Go Event Bus, Policy Engine, Orchestrator | Implemented | [runtime/internal/events/](file:///Users/sahilkanungo/Desktop/SYNTRIX/runtime/internal/events), [policy/](file:///Users/sahilkanungo/Desktop/SYNTRIX/runtime/internal/policy), [agent/](file:///Users/sahilkanungo/Desktop/SYNTRIX/runtime/internal/agent) |
| Go SQLite Store, Analyzer, Scheduler, IPC | Implemented | [runtime/internal/store/](file:///Users/sahilkanungo/Desktop/SYNTRIX/runtime/internal/store), [intelligence/](file:///Users/sahilkanungo/Desktop/SYNTRIX/runtime/internal/intelligence), [ipc/](file:///Users/sahilkanungo/Desktop/SYNTRIX/runtime/internal/ipc) |
| SwiftUI Dashboard, Companion, Activity, Audit | Implemented | [app/Sources/SyntrixApp/](file:///Users/sahilkanungo/Desktop/SYNTRIX/app/Sources/SyntrixApp) |
| MCP External Integration & Specialist Agents | Planned (Phase 5) | — |
| Research & Learning Workspace / RAG | Planned (Phase 6) | — |
| Agentic Workflow UI & Engine | Planned (Phase 7) | — |
| AWS Optional Cloud Services | Planned (Phase 8) | — |
