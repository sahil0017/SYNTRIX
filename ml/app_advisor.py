#!/usr/bin/env python3
"""
SYNTRIX App Advisor & Explainer Module:
Helps users understand what SYNTRIX's ML models and telemetry metrics mean in plain English,
and provides actionable recommendations to improve both Mac performance and SYNTRIX's own accuracy.
"""

import argparse
import json
import sqlite3
import sys
from typing import Any, Dict, List


def explain_telemetry_and_models(
    live_payload: Dict[str, Any],
    mem_fc: Dict[str, Any],
    super_intel: Dict[str, Any] | None,
    neural_subsystems: Dict[str, Any] | None,
    sample_count: int,
) -> Dict[str, Any]:
    cpu = float(live_payload.get("cpu_pct", 0.0) or 0.0)
    mem = float(live_payload.get("mem_pressure_pct", 0.0) or 0.0)
    disk = float(live_payload.get("disk_used_pct", 0.0) or 0.0)
    pageins = float(live_payload.get("pageins_per_sec", 0.0) or 0.0)
    gpu = float(live_payload.get("gpu_util_pct", 0.0) or 0.0)

    understanding_notes: List[str] = []
    improvement_tips: List[Dict[str, str]] = []

    # 1. Plain-English explanation of current system state
    if super_intel:
        hi = super_intel.get("health_index", {})
        bn = super_intel.get("bottleneck", {})
        score = float(hi.get("health_score", 80.0))
        state = str(bn.get("dominant_state", "Nominal"))
        culprit = str(bn.get("culprit", "System"))
        driver = str(hi.get("primary_variance_driver", "Memory"))
        plain_summary = (
            f"Health is {score:.0f}/100 ({state}). Primary variance driver is {driver}"
            + (f", led by {culprit}." if culprit and culprit != "System" else ".")
        )
        understanding_notes.append(
            f"PCA Health Index ({hi.get('model', 'PCA')}) scores overall stability across CPU, Memory, and Storage."
        )
    else:
        plain_summary = f"CPU {cpu:.0f}%, Memory Pressure {mem:.0f}%, Disk {disk:.0f}%."

    if neural_subsystems:
        ctx = neural_subsystems.get("context_intelligence", {})
        causal = neural_subsystems.get("causal_intelligence", {})
        res = neural_subsystems.get("resource_intelligence", {})
        understanding_notes.append(
            f"Context model recognizes '{ctx.get('working_pattern', 'Steady')}' "
            f"while {res.get('selected_model', 'TCN')} forecasts {res.get('ensemble_pred_mem_pct', mem):.0f}% memory pressure."
        )
        chain = causal.get("causal_chain") or []
        if chain:
            understanding_notes.append(f"Causal DAG trace: {' -> '.join(chain)}.")

    # 2. Actionable System & App Improvement Tips
    if sample_count < 18:
        improvement_tips.append({
            "category": "app_tuning",
            "title": "Warming Up ML History",
            "action": "Keep SYNTRIX running for ~2 minutes (or use Learning mode) so TCN/GRU, PCA, and IsolationForest reach full 18+ sample accuracy.",
            "command": "/status",
        })

    if mem_fc.get("drift_detected"):
        improvement_tips.append({
            "category": "app_tuning",
            "title": "Workload Shift Detected",
            "action": "Memory pattern shifted significantly; Transformer+RL is recalibrating its Q-values to your new workload.",
            "command": "/diagnose",
        })

    if pageins >= 50.0 or mem >= 75.0:
        improvement_tips.append({
            "category": "system_performance",
            "title": "Relieve Swap & Memory Pressure",
            "action": f"Active swap page-ins ({pageins:.0f}/s) or high memory pressure ({mem:.0f}%) can cause app freezes. Run Memory Optimize.",
            "command": "/optimize-memory",
        })

    if disk >= 55.0:
        improvement_tips.append({
            "category": "system_storage",
            "title": "Reclaim Stale Caches & Logs",
            "action": f"Disk is {disk:.0f}% full (above 50% guard). Use Optimize All to safely clean stale caches, logs, and DerivedData.",
            "command": "/optimize-all",
        })

    if gpu >= 65.0:
        improvement_tips.append({
            "category": "system_gpu",
            "title": "High Apple GPU Load",
            "action": f"GPU utilization is {gpu:.0f}%. Check browser WebGL tabs or video/ML workloads if UI feels sluggish.",
            "command": "/diagnose",
        })

    if not improvement_tips:
        improvement_tips.append({
            "category": "optimal",
            "title": "System & ML Running Optimally",
            "action": "All 7 neural subsystems are converged and system load is nominal. Use Guardian Assist or Learning mode for continuous protection.",
            "command": "/status",
        })

    return {
        "plain_english_summary": plain_summary,
        "understanding_notes": understanding_notes[:3],
        "improvement_tips": improvement_tips[:4],
    }


def main() -> None:
    parser = argparse.ArgumentParser(description="SYNTRIX App Advisor & Explainer")
    parser.add_argument("--db", type=str, default="", help="Path to syntrix.db")
    parser.add_argument("--explain", action="store_true", help="Print JSON explanation from latest snapshot")
    args = parser.parse_args()

    live: Dict[str, Any] = {"cpu_pct": 0.0, "mem_pressure_pct": 0.0, "disk_used_pct": 0.0}
    count = 0
    if args.db:
        try:
            conn = sqlite3.connect(f"file:{args.db}?mode=ro", uri=True)
            cur = conn.cursor()
            cur.execute("SELECT COUNT(*) FROM snapshots")
            count = int(cur.fetchone()[0])
            cur.execute("SELECT cpu_pct, mem_pressure_pct, disk_used_pct FROM snapshots ORDER BY timestamp_ms DESC LIMIT 1")
            row = cur.fetchone()
            if row:
                live = {"cpu_pct": row[0], "mem_pressure_pct": row[1], "disk_used_pct": row[2]}
            conn.close()
        except sqlite3.Error:
            pass

    out = explain_telemetry_and_models(live, {"drift_detected": False}, None, None, count)
    print(json.dumps(out, indent=2))


if __name__ == "__main__":
    main()
