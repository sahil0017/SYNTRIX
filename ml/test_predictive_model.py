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
            self.assertIn("cause", out["abnormal_apps"][0])
            self.assertGreater(out["inference_ms"], 0.0)
            self.assertIn("super_intelligence", out)
            si = out["super_intelligence"]
            self.assertIn("hang_risk_score", si)
            self.assertIn("storage_urgency_score", si)
            self.assertIn("at_risk_processes", si)
            self.assertGreaterEqual(si["hang_risk_score"], 50.0)
            self.assertIn("app_advisor", out)
            self.assertTrue(len(out["app_advisor"]["plain_english_summary"]) > 0)
            self.assertGreaterEqual(len(out["app_advisor"]["improvement_tips"]), 1)

    def test_super_intelligence_nan_and_swap_thrash_resilience(self):
        from super_intelligence import run_super_intelligence

        res = run_super_intelligence(
            [],
            {
                "cpu_pct": float("nan"),
                "mem_pressure_pct": 88.0,
                "disk_used_pct": 62.0,
                "pageins_per_sec": 320.0,
                "pageouts_per_sec": 95.0,
                "top_processes": [
                    {
                        "name": "Google Chrome Helper",
                        "pid": 4242,
                        "cpu_pct": 14.0,
                        "memory_bytes": 3_100_000_000,
                        "pageins": 420.0,
                        "wakeups_per_sec": 45.0,
                    }
                ],
            },
        )
        self.assertEqual(res["bottleneck"]["dominant_state"], "Swap-Thrash Freeze")
        self.assertEqual(res["strategic_directive"]["recommended_directive"], "/optimize-memory")
        self.assertGreater(res["hang_risk_score"], 70.0)
        self.assertEqual(res["at_risk_processes"][0]["cause"], "swap_thrash")

    def test_all_seven_neural_subsystems_and_intent(self):
        from neural_subsystems import classify_user_intent, run_neural_subsystems

        ns = run_neural_subsystems(
            [],
            {
                "cpu_pct": 82.0,
                "mem_pressure_pct": 79.0,
                "disk_used_pct": 62.0,
                "pageins_per_sec": 180.0,
                "gpu_util_pct": 25.0,
                "mode": "ASSIST",
                "top_processes": [
                    {"name": "Chrome", "pid": 500, "cpu_pct": 76.0, "memory_bytes": 2_200_000_000, "pageins": 140}
                ],
            },
        )
        self.assertIn("resource_intelligence", ns)
        self.assertIn(ns["resource_intelligence"]["selected_model"], ("TCN", "GRU"))
        self.assertEqual(ns["behaviour_intelligence"]["model"], "LSTM-Autoencoder-v2")
        self.assertEqual(ns["context_intelligence"]["model"], "ContextEmbed-Seq-v1")
        self.assertEqual(ns["causal_intelligence"]["model"], "CausalDAG-v1")
        self.assertEqual(ns["decision_intelligence"]["policy"], "Constrained-LinUCB-v1")

        intent = classify_user_intent("my chrome browser is lagging and eating ram")
        self.assertIn(intent["intent"], ("diagnose_hang", "optimize_memory"))
        self.assertGreater(intent["confidence"], 0.1)


if __name__ == "__main__":
    unittest.main()
