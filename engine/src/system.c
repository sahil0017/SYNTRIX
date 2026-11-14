#include "syntrix_internal.h"

#include <string.h>
#include <time.h>

static bool g_engine_initialized = false;

static struct {
    uint64_t cpu_err;
    uint64_t mem_err;
    uint64_t disk_err;
    uint64_t net_err;
    uint64_t power_err;
    uint64_t thermal_err;
    uint64_t proc_err;
} g_err_counts;

int64_t syntrix_now_ms(void) {
    struct timespec ts;
    clock_gettime(CLOCK_REALTIME, &ts);
    return (int64_t)ts.tv_sec * 1000LL + (int64_t)(ts.tv_nsec / 1000000LL);
}

uint64_t syntrix_monotonic_ns(void) {
    return clock_gettime_nsec_np(CLOCK_UPTIME_RAW);
}

syntrix_status_t syntrix_engine_init(void) {
    if (g_engine_initialized) {
        return SYNTRIX_OK;
    }
    memset(&g_err_counts, 0, sizeof(g_err_counts));

    syntrix_status_t st = syntrix_cpu_init();
    if (st != SYNTRIX_OK) {
        return st;
    }
    st = syntrix_process_init();
    if (st != SYNTRIX_OK) {
        syntrix_cpu_shutdown();
        return st;
    }
    st = syntrix_network_init();
    if (st != SYNTRIX_OK) {
        syntrix_process_shutdown();
        syntrix_cpu_shutdown();
        return st;
    }
    g_engine_initialized = true;
    return SYNTRIX_OK;
}

void syntrix_engine_shutdown(void) {
    if (!g_engine_initialized) {
        return;
    }
    syntrix_network_shutdown();
    syntrix_process_shutdown();
    syntrix_cpu_shutdown();
    g_engine_initialized = false;
}

static void record_health(
    syntrix_collector_health_t *h,
    syntrix_status_t st,
    uint64_t t0_ns,
    uint64_t *err_counter,
    double ok_confidence
) {
    uint64_t dt_ns = syntrix_monotonic_ns() - t0_ns;
    h->last_duration_us = dt_ns / 1000ULL;
    if (st == SYNTRIX_OK) {
        h->available = true;
        h->confidence = ok_confidence;
    } else {
        (*err_counter)++;
        h->available = false;
        h->confidence = 0.0;
    }
    h->error_count = *err_counter;
}

syntrix_status_t syntrix_collect_snapshot(syntrix_snapshot_t *out, uint32_t max_procs) {
    if (!out) {
        return SYNTRIX_ERR_INVALID_ARG;
    }
    if (!g_engine_initialized) {
        return SYNTRIX_ERR_NOT_INIT;
    }
    if (max_procs > SYNTRIX_MAX_TOP_PROCS) {
        max_procs = SYNTRIX_MAX_TOP_PROCS;
    }

    memset(out, 0, sizeof(*out));
    out->schema_version = SYNTRIX_SCHEMA_VERSION;
    out->timestamp_ms = syntrix_now_ms();
    out->monotonic_ns = syntrix_monotonic_ns();

    uint64_t t0 = syntrix_monotonic_ns();
    syntrix_status_t cpu_st = syntrix_cpu_collect(&out->cpu);
    record_health(&out->cpu_health, cpu_st, t0, &g_err_counts.cpu_err, 1.0);

    t0 = syntrix_monotonic_ns();
    syntrix_status_t mem_st = syntrix_memory_collect(&out->memory);
    record_health(&out->mem_health, mem_st, t0, &g_err_counts.mem_err, 1.0);

    t0 = syntrix_monotonic_ns();
    syntrix_status_t disk_st = syntrix_disk_collect(&out->disk);
    record_health(&out->disk_health, disk_st, t0, &g_err_counts.disk_err, 1.0);

    t0 = syntrix_monotonic_ns();
    syntrix_status_t net_st = syntrix_network_collect(&out->network);
    record_health(&out->net_health, net_st, t0, &g_err_counts.net_err, 1.0);

    t0 = syntrix_monotonic_ns();
    syntrix_status_t pwr_st = syntrix_power_collect(&out->power);
    record_health(&out->power_health, pwr_st, t0, &g_err_counts.power_err, 1.0);

    t0 = syntrix_monotonic_ns();
    syntrix_status_t thm_st = syntrix_thermal_collect(&out->thermal);
    record_health(&out->thermal_health, thm_st, t0, &g_err_counts.thermal_err, 0.85);

    (void)syntrix_gpu_collect(&out->gpu);

    t0 = syntrix_monotonic_ns();
    syntrix_status_t proc_st = syntrix_process_collect(
        out->top_processes,
        max_procs,
        &out->top_process_count,
        &out->process_count_total
    );
    record_health(&out->proc_health, proc_st, t0, &g_err_counts.proc_err, 1.0);

    /* Tolerate partial failures unless core CPU + Memory both failed. */
    if (cpu_st != SYNTRIX_OK && mem_st != SYNTRIX_OK) {
        return SYNTRIX_ERR_SYS_CALL;
    }
    return SYNTRIX_OK;
}
