import os
import sqlite3
import tempfile
import unittest

from transformer_rl import ACTIONS, infer_and_update


class TestTransformerRL(unittest.TestCase):
    def test_sqlite_transformer_and_rl_optimization(self):
        with tempfile.TemporaryDirectory() as tmp:
            db_path = os.path.join(tmp, "test_nce.db")
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
            conn.execute(
                """
                CREATE TABLE recommendations (
                    id TEXT PRIMARY KEY,
                    created_at_ms INTEGER NOT NULL,
                    kind TEXT NOT NULL,
                    severity TEXT NOT NULL,
                    title TEXT NOT NULL,
                    detail TEXT NOT NULL,
                    status TEXT NOT NULL,
                    outcome TEXT NOT NULL
                )
                """
            )
            base_ts = 1_700_000_000_000
            for i in range(40):
                cpu = 30.0 + (i % 10) * 5.0
                mem_p = 55.0 + (i % 12) * 2.5
                conn.execute(
                    "INSERT INTO snapshots (timestamp_ms, cpu_pct, mem_used_bytes, mem_total_bytes, mem_pressure_pct, swap_used_bytes, disk_used_pct, battery_pct, top_procs_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
                    (base_ts + i * 2000, cpu, 4_000_000_000, 8_000_000_000, mem_p, 256 * 1024 * 1024, 64.0, 90.0, "[]"),
                )
            conn.execute(
                "INSERT INTO recommendations (id, created_at_ms, kind, severity, title, detail, status, outcome) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
                ("rec-1", base_ts, "memory_pressure", "high", "Trim", "Trim", "approved", "improved"),
            )
            conn.commit()
            conn.close()

            res = infer_and_update(db_path, {"cpu_pct": 78.0, "mem_pressure_pct": 82.0, "disk_used_pct": 64.0})
            self.assertEqual(res["sqlite_rows_used"], 40)
            self.assertGreater(res["rl_episodes"], 0)
            self.assertIn(res["rl_optimal_action"], ACTIONS)
            self.assertGreaterEqual(res["transformer_pred_mem_pct"], 0.0)
            self.assertLessEqual(res["transformer_pred_mem_pct"], 100.0)


if __name__ == "__main__":
    unittest.main()
