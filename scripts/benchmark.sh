#!/usr/bin/env bash
set -euo pipefail

BIN="/Users/sahilkanungo/Desktop/SYNTRIX/runtime/build/syntrixd"
SOCK="/tmp/syntrixd_bench.sock"
DB="/tmp/syntrix_bench.db"

if [[ ! -x "$BIN" ]]; then
  echo "ERROR: $BIN not found. Run 'make runtime' first." >&2
  exit 1
fi

rm -f "$SOCK" "$DB"
"$BIN" -sock "$SOCK" -db "$DB" -interval 1s -mode MONITOR >/tmp/syntrix_bench.log 2>&1 &
PID=$!
trap 'kill "$PID" 2>/dev/null || true; wait "$PID" 2>/dev/null || true; rm -f "$SOCK" "$DB"' EXIT

sleep 2

echo "Running SYNTRIX 8-second idle overhead benchmark (PID=$PID)..."
MAX_CPU="0.0"
MAX_RSS_KB=0

for _ in {1..8}; do
  LINE=$(ps -p "$PID" -o %cpu=,rss= || echo "0.0 0")
  CPU=$(echo "$LINE" | awk '{print $1}')
  RSS_KB=$(echo "$LINE" | awk '{print $2}')
  MAX_CPU=$(awk -v a="$MAX_CPU" -v b="$CPU" 'BEGIN { print (b > a ? b : a) }')
  if (( RSS_KB > MAX_RSS_KB )); then
    MAX_RSS_KB=$RSS_KB
  fi
  sleep 1
done

MAX_RSS_MB=$(awk -v kb="$MAX_RSS_KB" 'BEGIN { printf "%.2f", kb / 1024.0 }')
echo "Measured peak idle CPU: ${MAX_CPU}% (target < 1.0%)"
echo "Measured peak RSS memory: ${MAX_RSS_MB} MB (target < 50.0 MB)"

CPU_OK=$(awk -v c="$MAX_CPU" 'BEGIN { print (c < 1.0 ? 1 : 0) }')
MEM_OK=$(awk -v m="$MAX_RSS_MB" 'BEGIN { print (m < 50.0 ? 1 : 0) }')

if [[ "$CPU_OK" -eq 1 && "$MEM_OK" -eq 1 ]]; then
  echo "BENCHMARK: PASS"
  exit 0
else
  echo "BENCHMARK: WARN (outside target)"
  exit 1
fi
