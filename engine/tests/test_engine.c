#include "syntrix_engine.h"

#include <assert.h>
#include <stdio.h>
#include <string.h>
#include <unistd.h>

int main(void) {
    printf("Running SYNTRIX C17 engine tests...\n");

    syntrix_snapshot_t snap;
    memset(&snap, 0, sizeof(snap));

    assert(syntrix_collect_snapshot(&snap, 10) == SYNTRIX_ERR_NOT_INIT);
    assert(syntrix_collect_snapshot(NULL, 10) == SYNTRIX_ERR_INVALID_ARG);

    assert(syntrix_engine_init() == SYNTRIX_OK);
    /* Warm-up snapshot for delta-based metrics */
    assert(syntrix_collect_snapshot(&snap, 10) == SYNTRIX_OK);
    usleep(200000);

    assert(syntrix_collect_snapshot(&snap, 10) == SYNTRIX_OK);

    assert(snap.schema_version == SYNTRIX_SCHEMA_VERSION);
    assert(snap.timestamp_ms > 0);
    assert(snap.monotonic_ns > 0);
    assert(snap.cpu_health.available && snap.mem_health.available && snap.proc_health.available);
    assert(snap.cpu.core_count > 0 && snap.cpu.core_count <= SYNTRIX_MAX_CORES);
    assert(snap.cpu.total_usage_pct >= 0.0 && snap.cpu.total_usage_pct <= 100.0);
    assert(snap.memory.total_bytes > 0);
    assert(snap.memory.used_bytes > 0 && snap.memory.used_bytes <= snap.memory.total_bytes);
    assert(snap.disk.total_bytes > 0);
    assert(snap.thermal.cpu_speed_limit_pct > 0.0 && snap.thermal.state_label[0] != '\0');
    assert(snap.process_count_total > 0);
    assert(snap.top_process_count > 0 && snap.top_process_count <= 10);

    printf("  CPU: %.2f%% (%u cores)\n", snap.cpu.total_usage_pct, snap.cpu.core_count);
    printf("  MEM: %.2f GB / %.2f GB (%.1f%%, pressure=%.1f%%)\n",
           (double)snap.memory.used_bytes / (1024.0 * 1024.0 * 1024.0),
           (double)snap.memory.total_bytes / (1024.0 * 1024.0 * 1024.0),
           snap.memory.used_pct,
           snap.memory.pressure_pct);
    printf("  THERMAL: %s (speed_limit=%.0f%%, throttled=%d)\n",
           snap.thermal.state_label, snap.thermal.cpu_speed_limit_pct, snap.thermal.is_throttled ? 1 : 0);
    printf("  DISK: %.1f%% used\n", snap.disk.used_pct);
    printf("  PROCS: %u total, top=%s (pid=%d, mem=%.1f MB)\n",
           snap.process_count_total,
           snap.top_processes[0].name,
           snap.top_processes[0].pid,
           (double)snap.top_processes[0].memory_bytes / (1024.0 * 1024.0));

    syntrix_engine_shutdown();
    printf("All SYNTRIX C17 engine tests passed!\n");
    return 0;
}
