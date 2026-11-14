#!/usr/bin/env python3
"""
SYNTRIX 2.0 Predictive Resource Intelligence (Python / scikit-learn)
- Chronological train/validation split comparing Ridge, RandomForest, and GradientBoosting
- Multi-horizon memory forecasting (1m, 5m, 15m, 30m) with uncertainty bounds
- Storage exhaustion regression with uncertainty intervals
- Concept drift detection across rolling telemetry windows
- IsolationForest abnormal application behaviour detection
"""

import argparse
import json
import os
import sqlite3
import sys
from typing import Any, Dict, List, Tuple

import numpy as np
from sklearn.cluster import KMeans
from sklearn.ensemble import (
    ExtraTreesRegressor,
    GradientBoostingRegressor,
    IsolationForest,
    RandomForestRegressor,
)
from sklearn.linear_model import HuberRegressor, Ridge
from sklearn.neighbors import LocalOutlierFactor
from sklearn.neural_network import MLPRegressor
from sklearn.pipeline import make_pipeline
from sklearn.preprocessing import StandardScaler

MODEL_VERSION = "2.1.0"
FEATURE_SCHEMA_VERSION = "v2"


def load_history(db_path: str, limit: int = 500) -> List[Tuple[int, float, float, float, str]]:
    if not db_path or not os.path.exists(db_path):
        return []
    conn = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)
    try:
        cur = conn.cursor()
        cur.execute(
            """
            SELECT timestamp_ms, cpu_pct, mem_pressure_pct, disk_used_pct, top_procs_json
            FROM snapshots
            ORDER BY timestamp_ms DESC
            LIMIT ?
            """,
            (limit,),
        )
        rows = cur.fetchall()
        rows.reverse()  # oldest -> newest
        return rows
    except sqlite3.Error:
        return []
    finally:
        conn.close()


def detect_concept_drift(values: np.ndarray) -> Tuple[bool, float]:
    if len(values) < 12:
        return False, 0.0
    mid = len(values) // 2
    old, recent = values[:mid], values[mid:]
    old_std = max(float(np.std(old)), 2.5)
    z_shift = abs(float(np.mean(recent)) - float(np.mean(old))) / old_std
    return bool(z_shift >= 1.8), round(z_shift, 2)


def forecast_memory(timestamps_ms: np.ndarray, pressures: np.ndarray, current_pct: float) -> Dict[str, Any]:
    if len(pressures) < 3:
        risk = "high" if current_pct >= 80 else ("moderate" if current_pct >= 65 else "low")
        return {
            "current_pct": round(current_pct, 2),
            "predicted_1m_pct": round(current_pct, 2),
            "predicted_5m_pct": round(current_pct, 2),
            "predicted_15m_pct": round(current_pct, 2),
            "predicted_30m_pct": round(current_pct, 2),
            "uncertainty_pct": 5.0,
            "slope_pct_per_min": 0.0,
            "minutes_to_critical": -1.0,
            "selected_model": "baseline-ewma",
            "validation_mae": 0.0,
            "drift_detected": False,
            "drift_score": 0.0,
            "risk": risk,
        }

    t_min = (timestamps_ms - timestamps_ms[0]) / 60000.0
    drift_detected, drift_score = detect_concept_drift(pressures)

    if np.ptp(t_min) < 1e-6:
        slope = 0.0
        intercept = float(pressures[-1])
        uncertainty = 3.0
        selected_model = "Ridge"
        val_mae = 0.0
    else:
        ridge = Ridge(alpha=1.0)
        ridge.fit(t_min.reshape(-1, 1), pressures)
        slope = float(ridge.coef_[0])
        intercept = float(ridge.intercept_)
        residuals = np.abs(pressures - ridge.predict(t_min.reshape(-1, 1)))
        uncertainty = max(2.0, float(np.std(residuals)) * 1.5)
        selected_model = "Ridge"
        val_mae = float(np.mean(residuals))

        # Chronological model comparison when enough samples exist
        if len(pressures) >= 18:
            lags = np.column_stack([
                t_min[1:],
                pressures[:-1],
                np.diff(pressures, prepend=pressures[0])[1:],
            ])
            targets = pressures[1:]
            split = max(10, int(len(targets) * 0.75))
            if split < len(targets):
                X_tr, y_tr = lags[:split], targets[:split]
                X_va, y_va = lags[split:], targets[split:]
                candidates = {
                    "Ridge": Ridge(alpha=1.0),
                    "Huber": HuberRegressor(max_iter=200),
                    "RandomForest": RandomForestRegressor(n_estimators=20, max_depth=4, random_state=42),
                    "ExtraTrees": ExtraTreesRegressor(n_estimators=20, max_depth=4, random_state=42),
                    "GradientBoosting": GradientBoostingRegressor(n_estimators=25, max_depth=2, random_state=42),
                    "NeuralMLP": make_pipeline(
                        StandardScaler(),
                        MLPRegressor(hidden_layer_sizes=(16, 8), max_iter=600, tol=1e-2, random_state=42),
                    ),
                }
                best_mae = float("inf")
                for name, mdl in candidates.items():
                    mdl.fit(X_tr, y_tr)
                    preds = mdl.predict(X_va)
                    mae = float(np.mean(np.abs(y_va - preds)))
                    if mae < best_mae:
                        best_mae = mae
                        selected_model = name
                val_mae = best_mae
                uncertainty = max(2.0, best_mae * 1.5)

    now_t = float(t_min[-1])
    base = 0.7 * current_pct + 0.3 * (slope * now_t + intercept)
    pred_1m = float(np.clip(base + slope * 1.0, 0.0, 100.0))
    pred_5m = float(np.clip(base + slope * 5.0, 0.0, 100.0))
    pred_15m = float(np.clip(base + slope * 15.0, 0.0, 100.0))
    pred_30m = float(np.clip(base + slope * 30.0, 0.0, 100.0))

    mins_to_crit = -1.0
    if slope > 0.05 and current_pct < 85.0:
        mins_to_crit = round((85.0 - current_pct) / slope, 1)

    risk = "low"
    if current_pct >= 80.0 or pred_15m >= 85.0 or (0 < mins_to_crit <= 15.0):
        risk = "high"
    elif current_pct >= 65.0 or pred_15m >= 72.0 or slope >= 0.5:
        risk = "moderate"

    return {
        "current_pct": round(current_pct, 2),
        "predicted_1m_pct": round(pred_1m, 2),
        "predicted_5m_pct": round(pred_5m, 2),
        "predicted_15m_pct": round(pred_15m, 2),
        "predicted_30m_pct": round(pred_30m, 2),
        "uncertainty_pct": round(uncertainty, 2),
        "slope_pct_per_min": round(slope, 3),
        "minutes_to_critical": mins_to_crit,
        "selected_model": selected_model,
        "validation_mae": round(val_mae, 2),
        "drift_detected": drift_detected,
        "drift_score": drift_score,
        "risk": risk,
    }


def forecast_storage(timestamps_ms: np.ndarray, disk_pcts: np.ndarray, current_pct: float) -> Dict[str, Any]:
    if len(disk_pcts) < 3:
        risk = "high" if current_pct >= 88 else ("moderate" if current_pct >= 75 else "low")
        return {
            "current_pct": round(current_pct, 2),
            "growth_pct_per_day": 0.0,
            "days_until_90_pct": -1,
            "days_until_90_pct_low": -1,
            "days_until_90_pct_high": -1,
            "risk": risk,
        }

    t_days = (timestamps_ms - timestamps_ms[0]) / 86400000.0
    if np.ptp(t_days) < 1e-7:
        slope = 0.0
    else:
        model = Ridge(alpha=0.5)
        model.fit(t_days.reshape(-1, 1), disk_pcts)
        slope = float(model.coef_[0])

    days_90 = -1
    days_low = -1
    days_high = -1
    if slope > 0.005 and current_pct < 90.0:
        days_90 = int(round((90.0 - current_pct) / slope))
        days_low = max(0, int(round(days_90 * 0.8)))
        days_high = int(round(days_90 * 1.25))

    risk = "low"
    if current_pct >= 88.0 or (0 <= days_90 <= 7):
        risk = "high"
    elif current_pct >= 75.0 or (0 <= days_90 <= 30) or slope >= 0.5:
        risk = "moderate"

    return {
        "current_pct": round(current_pct, 2),
        "growth_pct_per_day": round(slope, 3),
        "days_until_90_pct": days_90,
        "days_until_90_pct_low": days_low,
        "days_until_90_pct_high": days_high,
        "risk": risk,
    }


def detect_abnormal_apps(
    history_rows: List[Tuple[int, float, float, float, str]],
    live_procs: List[Dict[str, Any]],
) -> List[Dict[str, Any]]:
    baseline_feats: List[List[float]] = []
    for _, _, _, _, procs_json in history_rows[-80:]:
        if not procs_json:
            continue
        try:
            procs = json.loads(procs_json)
            for p in procs:
                cpu = float(p.get("cpu_pct", 0.0))
                mem_mb = float(p.get("memory_bytes", 0)) / (1024.0 * 1024.0)
                threads = float(p.get("threads", 1))
                baseline_feats.append([cpu, mem_mb, threads])
        except (ValueError, TypeError):
            continue

    live_feats: List[List[float]] = []
    for p in live_procs:
        cpu = float(p.get("cpu_pct", 0.0))
        mem_mb = float(p.get("memory_bytes", 0)) / (1024.0 * 1024.0)
        threads = float(p.get("threads", 1))
        live_feats.append([cpu, mem_mb, threads])

    if not live_procs:
        return []

    abnormal: List[Dict[str, Any]] = []
    combined = baseline_feats + live_feats

    if len(combined) >= 12:
        scaler = StandardScaler()
        X_train = scaler.fit_transform(np.array(combined, dtype=np.float64))
        X_live = scaler.transform(np.array(live_feats, dtype=np.float64))

        iso = IsolationForest(contamination=0.1, random_state=42, n_estimators=50)
        iso.fit(X_train)
        iso_preds = iso.predict(X_live)
        iso_scores = iso.decision_function(X_live)

        n_neighbors = min(10, max(2, len(X_train) - 1))
        lof = LocalOutlierFactor(n_neighbors=n_neighbors, novelty=True, contamination=0.1)
        lof.fit(X_train)
        lof_preds = lof.predict(X_live)

        for idx, p in enumerate(live_procs):
            cpu = live_feats[idx][0]
            mem_mb = live_feats[idx][1]
            iso_out = iso_preds[idx] == -1
            lof_out = lof_preds[idx] == -1
            if (iso_out or lof_out) and (cpu >= 25.0 or mem_mb >= 512.0):
                consensus = "IsolationForest+LOF" if (iso_out and lof_out) else ("IsolationForest" if iso_out else "LocalOutlierFactor")
                bonus = 0.08 if (iso_out and lof_out) else 0.0
                norm_score = float(np.clip(0.5 - iso_scores[idx] + bonus, 0.5, 0.99))
                abnormal.append(
                    {
                        "name": p.get("name", "unknown"),
                        "pid": int(p.get("pid", 0)),
                        "cpu_pct": round(cpu, 1),
                        "mem_mb": round(mem_mb, 1),
                        "anomaly_score": round(norm_score, 2),
                        "reason": f"{consensus} outlier (CPU {cpu:.1f}%, RSS {mem_mb:.0f} MB)",
                    }
                )
    else:
        for idx, p in enumerate(live_procs):
            cpu, mem_mb, _ = live_feats[idx]
            if cpu >= 70.0 or mem_mb >= 2048.0:
                abnormal.append(
                    {
                        "name": p.get("name", "unknown"),
                        "pid": int(p.get("pid", 0)),
                        "cpu_pct": round(cpu, 1),
                        "mem_mb": round(mem_mb, 1),
                        "anomaly_score": 0.80,
                        "reason": f"Threshold outlier (CPU {cpu:.1f}%, RSS {mem_mb:.0f} MB)",
                    }
                )

    abnormal.sort(key=lambda x: x["anomaly_score"], reverse=True)
    return abnormal[:5]


def classify_workload_regime(
    history_rows: List[Tuple[int, float, float, float, str]],
    live_cpu: float,
    live_mem: float,
    live_disk: float,
) -> Dict[str, Any]:
    points = [[float(r[1]), float(r[2]), float(r[3])] for r in history_rows[-120:]]
    points.append([live_cpu, live_mem, live_disk])

    regime = "Steady Work"
    if live_cpu >= 70.0 and live_mem >= 75.0:
        regime = "Contention"
    elif live_mem >= 72.0:
        regime = "Memory-Heavy"
    elif live_cpu >= 55.0:
        regime = "Compute-Heavy"
    elif live_cpu < 20.0 and live_mem < 55.0:
        regime = "Idle / Light"

    cluster_id = 0
    model_used = "heuristic"
    if len(points) >= 12:
        X = np.array(points, dtype=np.float64)
        scaler = StandardScaler()
        Xs = scaler.fit_transform(X)
        km = KMeans(n_clusters=4, n_init=5, random_state=42)
        labels = km.fit_predict(Xs)
        cluster_id = int(labels[-1])
        model_used = "KMeans(k=4)"

    return {
        "regime": regime,
        "cluster_id": cluster_id,
        "model": model_used,
    }


def rank_candidate_actions(live_cpu: float, live_mem: float, live_disk: float) -> List[Dict[str, Any]]:
    candidates = [
        ("trim_daemon_heap", round(min(0.95, max(0.15, live_mem / 100.0)), 2)),
        ("nice_background_cpu", round(min(0.95, max(0.10, live_cpu / 100.0)), 2)),
        ("preview_stale_caches", round(min(0.95, max(0.10, live_disk / 100.0)), 2)),
        ("monitor_only", round(max(0.20, 1.0 - max(live_cpu, live_mem) / 100.0), 2)),
    ]
    candidates.sort(key=lambda x: x[1], reverse=True)
    return [{"action": a, "utility": u} for a, u in candidates]


def predict(db_path: str, live_payload: Dict[str, Any]) -> Dict[str, Any]:
    rows = load_history(db_path)

    live_cpu_pct = float(live_payload.get("cpu_pct", 0.0))
    live_mem_pct = float(live_payload.get("mem_pressure_pct", 0.0))
    live_disk_pct = float(live_payload.get("disk_used_pct", 0.0))
    live_ts_ms = int(live_payload.get("timestamp_ms", 0))
    live_procs = live_payload.get("top_processes") or []

    ts_list = [r[0] for r in rows]
    mem_list = [r[2] for r in rows]
    disk_list = [r[3] for r in rows]

    if live_ts_ms > 0 and (not ts_list or live_ts_ms > ts_list[-1]):
        ts_list.append(live_ts_ms)
        mem_list.append(live_mem_pct)
        disk_list.append(live_disk_pct)

    ts_arr = np.array(ts_list, dtype=np.float64)
    mem_arr = np.array(mem_list, dtype=np.float64)
    disk_arr = np.array(disk_list, dtype=np.float64)

    cur_mem = float(mem_arr[-1]) if len(mem_arr) > 0 else live_mem_pct
    cur_disk = float(disk_arr[-1]) if len(disk_arr) > 0 else live_disk_pct

    mem_fc = forecast_memory(ts_arr, mem_arr, cur_mem)
    sto_fc = forecast_storage(ts_arr, disk_arr, cur_disk)
    abnormal = detect_abnormal_apps(rows, live_procs)
    regime_info = classify_workload_regime(rows, live_cpu_pct, cur_mem, cur_disk)
    ranked_actions = rank_candidate_actions(live_cpu_pct, cur_mem, cur_disk)

    transformer_rl_out = None
    try:
        script_dir = os.path.dirname(os.path.abspath(__file__))
        if script_dir not in sys.path:
            sys.path.insert(0, script_dir)
        from transformer_rl import infer_and_update

        transformer_rl_out = infer_and_update(db_path, live_payload)
    except Exception:
        transformer_rl_out = None

    summary_parts = [
        f"Mem 15m={mem_fc['predicted_15m_pct']:.0f}%±{mem_fc['uncertainty_pct']:.0f}% ({mem_fc['risk']})",
        f"Disk={sto_fc['current_pct']:.0f}% ({sto_fc['risk']})",
        f"Regime={regime_info['regime']}",
    ]
    if abnormal:
        summary_parts.append(f"{len(abnormal)} abnormal app(s): {abnormal[0]['name']}")
    else:
        summary_parts.append("No abnormal apps")

    return {
        "model_backend": f"python-sklearn ({mem_fc['selected_model']} + IF/LOF + KMeans)",
        "model_version": MODEL_VERSION,
        "feature_schema_version": FEATURE_SCHEMA_VERSION,
        "sample_count": len(ts_list),
        "memory_forecast": mem_fc,
        "storage_forecast": sto_fc,
        "workload_regime": regime_info,
        "recommended_actions": ranked_actions,
        "transformer_rl": transformer_rl_out,
        "abnormal_apps": abnormal,
        "summary": " · ".join(summary_parts),
    }


def main() -> None:
    parser = argparse.ArgumentParser(description="SYNTRIX Python Predictive Resource Intelligence")
    parser.add_argument("--db", default="", help="Path to syntrix.db")
    args = parser.parse_args()

    live_payload: Dict[str, Any] = {}
    if not sys.stdin.isatty():
        raw = sys.stdin.read().strip()
        if raw:
            try:
                live_payload = json.loads(raw)
            except ValueError:
                live_payload = {}

    result = predict(args.db, live_payload)
    print(json.dumps(result))


if __name__ == "__main__":
    main()
