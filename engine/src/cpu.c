#include "syntrix_internal.h"

#include <mach/mach.h>
#include <mach/processor_info.h>
#include <mach/vm_map.h>
#include <stdlib.h>
#include <string.h>

typedef struct {
    uint64_t user;
    uint64_t sys;
    uint64_t idle;
    uint64_t nice;
} core_ticks_t;

static core_ticks_t g_prev_ticks[SYNTRIX_MAX_CORES];
static uint32_t g_prev_core_count = 0;
static bool g_cpu_initialized = false;

syntrix_status_t syntrix_cpu_init(void) {
    memset(g_prev_ticks, 0, sizeof(g_prev_ticks));
    g_prev_core_count = 0;
    g_cpu_initialized = true;
    /* Prime initial ticks so the first snapshot has a valid baseline if possible. */
    syntrix_cpu_metrics_t dummy;
    memset(&dummy, 0, sizeof(dummy));
    syntrix_cpu_collect(&dummy);
    return SYNTRIX_OK;
}

void syntrix_cpu_shutdown(void) {
    g_cpu_initialized = false;
    g_prev_core_count = 0;
}

syntrix_status_t syntrix_cpu_collect(syntrix_cpu_metrics_t *out) {
    if (!out) {
        return SYNTRIX_ERR_INVALID_ARG;
    }
    if (!g_cpu_initialized) {
        return SYNTRIX_ERR_NOT_INIT;
    }

    natural_t num_cpus = 0;
    processor_info_array_t cpu_info = NULL;
    mach_msg_type_number_t num_cpu_info = 0;

    kern_return_t kr = host_processor_info(
        mach_host_self(),
        PROCESSOR_CPU_LOAD_INFO,
        &num_cpus,
        &cpu_info,
        &num_cpu_info
    );
    if (kr != KERN_SUCCESS || cpu_info == NULL) {
        return SYNTRIX_ERR_SYS_CALL;
    }

    uint32_t count = num_cpus;
    if (count > SYNTRIX_MAX_CORES) {
        count = SYNTRIX_MAX_CORES;
    }
    out->core_count = count;

    uint64_t sum_user = 0;
    uint64_t sum_sys = 0;
    uint64_t sum_idle = 0;
    uint64_t sum_total = 0;

    processor_cpu_load_info_t load = (processor_cpu_load_info_t)cpu_info;
    for (uint32_t i = 0; i < count; i++) {
        uint64_t u = load[i].cpu_ticks[CPU_STATE_USER];
        uint64_t s = load[i].cpu_ticks[CPU_STATE_SYSTEM];
        uint64_t id = load[i].cpu_ticks[CPU_STATE_IDLE];
        uint64_t n = load[i].cpu_ticks[CPU_STATE_NICE];

        if (i < g_prev_core_count) {
            uint64_t du = (u >= g_prev_ticks[i].user) ? (u - g_prev_ticks[i].user) : 0;
            uint64_t ds = (s >= g_prev_ticks[i].sys) ? (s - g_prev_ticks[i].sys) : 0;
            uint64_t did = (id >= g_prev_ticks[i].idle) ? (id - g_prev_ticks[i].idle) : 0;
            uint64_t dn = (n >= g_prev_ticks[i].nice) ? (n - g_prev_ticks[i].nice) : 0;

            uint64_t dtotal = du + ds + did + dn;
            if (dtotal > 0) {
                double busy = (double)(du + ds + dn) * 100.0 / (double)dtotal;
                out->per_core_usage_pct[i] = busy;
            } else {
                out->per_core_usage_pct[i] = 0.0;
            }
            sum_user += du + dn;
            sum_sys += ds;
            sum_idle += did;
            sum_total += dtotal;
        } else {
            out->per_core_usage_pct[i] = 0.0;
        }

        g_prev_ticks[i].user = u;
        g_prev_ticks[i].sys = s;
        g_prev_ticks[i].idle = id;
        g_prev_ticks[i].nice = n;
    }

    g_prev_core_count = count;

    vm_deallocate(mach_task_self(), (vm_address_t)cpu_info,
                  (vm_size_t)num_cpu_info * sizeof(integer_t));

    if (sum_total > 0) {
        out->user_pct = (double)sum_user * 100.0 / (double)sum_total;
        out->system_pct = (double)sum_sys * 100.0 / (double)sum_total;
        out->idle_pct = (double)sum_idle * 100.0 / (double)sum_total;
        out->total_usage_pct = out->user_pct + out->system_pct;
    } else {
        out->user_pct = 0.0;
        out->system_pct = 0.0;
        out->idle_pct = 100.0;
        out->total_usage_pct = 0.0;
    }

    return SYNTRIX_OK;
}
