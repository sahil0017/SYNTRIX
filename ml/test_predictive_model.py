import json
import os
import sqlite3
import tempfile
import unittest

from predictive_model import FEATURE_SCHEMA_VERSION, MODEL_VERSION, predict


class TestPredictiveModel(unittest.TestCase):
    def test_chronological_split_multi_horizon_and_isolation_forest(self):
        with tempfile.TemporaryDirectory() as tmp:
            db_path = os.path.join(tmp, "test.db")
            conn = sqlite3.connect(db_path)
            conn.execute(
                """
                CREATE TABLE snapshots (
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
                )
                """
            )

            base_ts = 1_700_000_000_000
            normal_procs = json.dumps(
                [
                    {"name": "Finder", "pid": 100, "cpu_pct": 1.2, "memory_bytes": 120 * 1024 * 1024, "threads": 4},
                    {"name": "WindowServer", "pid": 101, "cpu_pct": 8.0, "memory_bytes": 240 * 1024 * 1024, "threads": 6},
                ]
            )
            for i in range(22):
                ts = base_ts + i * 60_000
                mem_p = 40.0 + i * 1.4
                disk_p = 60.0 + i * 0.2
                conn.execute(
                    "INSERT INTO snapshots (timestamp_ms, cpu_pct, mem_used_bytes, mem_total_bytes, mem_pressure_pct, swap_used_bytes, disk_used_pct, battery_pct, top_procs_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
                    (ts, 15.0, 4_000_000_000, 8_000_000_000, mem_p, 0, disk_p, 90.0, normal_procs),
                )
            conn.commit()
            conn.close()

            live = {
                "timestamp_ms": base_ts + 22 * 60_000,
                "mem_pressure_pct": 71.0,
                "disk_used_pct": 64.4,
                "top_processes": [
                    {"name": "Finder", "pid": 100, "cpu_pct": 1.5, "memory_bytes": 120 * 1024 * 1024, "threads": 4},
                    {"name": "RunawayCompiler", "pid": 9999, "cpu_pct": 94.0, "memory_bytes": 2500 * 1024 * 1024, "threads": 24},
                ],
            }
            out = predict(db_path, live)
            self.assertEqual(out["model_version"], MODEL_VERSION)
            self.assertEqual(out["feature_schema_version"], FEATURE_SCHEMA_VERSION)
            self.assertEqual(out["sample_count"], 23)
            self.assertIn("predicted_1m_pct", out["memory_forecast"])
            self.assertIn("predicted_30m_pct", out["memory_forecast"])
            self.assertGreater(out["memory_forecast"]["uncertainty_pct"], 0.0)
            self.assertIn(
                out["memory_forecast"]["selected_model"],
                ("Ridge", "Huber", "RandomForest", "ExtraTrees", "GradientBoosting", "NeuralMLP"),
            )
            self.assertTrue(out["memory_forecast"]["drift_detected"])
            self.assertIn("workload_regime", out)
            self.assertEqual(out["workload_regime"]["model"], "KMeans(k=4)")
            self.assertGreaterEqual(len(out["recommended_actions"]), 1)
            self.assertGreaterEqual(len(out["abnormal_apps"]), 1)
            self.assertEqual(out["abnormal_apps"][0]["name"], "RunawayCompiler")


if __name__ == "__main__":
    unittest.main()
