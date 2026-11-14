#include "syntrix_internal.h"

#include <libproc.h>
#include <mach/mach_time.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/proc_info.h>

#define MAX_TRACKED_PIDS 2048

typedef struct {
    int32_t pid;
    uint64_t start_time_us;
    uint64_t total_ns;
    uint64_t mono_ns;
} proc_cpu_sample_t;

static proc_cpu_sample_t g_prev_procs[MAX_TRACKED_PIDS];
static uint32_t g_prev_proc_count = 0;
static mach_timebase_info_data_t g_timebase;
static bool g_proc_initialized = false;

static uint64_t mach_ticks_to_ns(uint64_t ticks) {
    if (g_timebase.denom == 0) {
        return ticks;
    }
    return (ticks * g_timebase.numer) / g_timebase.denom;
}

static int find_prev_proc(int32_t pid, uint64_t start_time_us, uint64_t *out_ns, uint64_t *out_mono_ns) {
    for (uint32_t i = 0; i < g_prev_proc_count; i++) {
        if (g_prev_procs[i].pid == pid && g_prev_procs[i].start_time_us == start_time_us) {
            *out_ns = g_prev_procs[i].total_ns;
            *out_mono_ns = g_prev_procs[i].mono_ns;
            return 1;
        }
    }
    return 0;
}

static int compare_procs(const void *a, const void *b) {
    const syntrix_process_info_t *pa = (const syntrix_process_info_t *)a;
    const syntrix_process_info_t *pb = (const syntrix_process_info_t *)b;

    if (pb->cpu_pct > pa->cpu_pct + 0.05) {
        return 1;
    }
    if (pa->cpu_pct > pb->cpu_pct + 0.05) {
        return -1;
    }
    if (pb->memory_bytes > pa->memory_bytes) {
        return 1;
    }
    if (pa->memory_bytes > pb->memory_bytes) {
        return -1;
    }
    return 0;
}

syntrix_status_t syntrix_process_init(void) {
    memset(g_prev_procs, 0, sizeof(g_prev_procs));
    g_prev_proc_count = 0;
    mach_timebase_info(&g_timebase);
    g_proc_initialized = true;
    return SYNTRIX_OK;
}

void syntrix_process_shutdown(void) {
    g_proc_initialized = false;
    g_prev_proc_count = 0;
}

syntrix_status_t syntrix_process_collect(
    syntrix_process_info_t *out_procs,
    uint32_t max_procs,
    uint32_t *out_count,
    uint32_t *out_total_count
) {
    if (!out_procs || !out_count || !out_total_count) {
        return SYNTRIX_ERR_INVALID_ARG;
    }
    if (!g_proc_initialized) {
        return SYNTRIX_ERR_NOT_INIT;
    }

    int bytes_needed = proc_listpids(PROC_ALL_PIDS, 0, NULL, 0);
    if (bytes_needed <= 0) {
        return SYNTRIX_ERR_SYS_CALL;
    }

    uint32_t capacity = (uint32_t)(bytes_needed / (int)sizeof(pid_t)) + 64;
    if (capacity > MAX_TRACKED_PIDS) {
        capacity = MAX_TRACKED_PIDS;
    }

    pid_t *pids = (pid_t *)calloc(capacity, sizeof(pid_t));
    syntrix_process_info_t *all_procs =
        (syntrix_process_info_t *)calloc(capacity, sizeof(syntrix_process_info_t));
    proc_cpu_sample_t *next_samples =
        (proc_cpu_sample_t *)calloc(capacity, sizeof(proc_cpu_sample_t));

    if (!pids || !all_procs || !next_samples) {
        free(pids);
        free(all_procs);
        free(next_samples);
        return SYNTRIX_ERR_ALLOC;
    }

    int bytes_filled = proc_listpids(PROC_ALL_PIDS, 0, pids, (int)(capacity * sizeof(pid_t)));
    if (bytes_filled <= 0) {
        free(pids);
        free(all_procs);
        free(next_samples);
        return SYNTRIX_ERR_SYS_CALL;
    }

    uint32_t pid_count = (uint32_t)(bytes_filled / (int)sizeof(pid_t));
    uint64_t now_mono_ns = syntrix_monotonic_ns();
    uint32_t collected = 0;
    uint32_t next_sample_count = 0;

    for (uint32_t i = 0; i < pid_count; i++) {
        pid_t pid = pids[i];
        if (pid <= 0) {
            continue;
        }

        struct proc_taskinfo task_info;
        int ret = proc_pidinfo(pid, PROC_PIDTASKINFO, 0, &task_info, sizeof(task_info));
        if (ret != (int)sizeof(task_info)) {
            continue;
        }

        syntrix_process_info_t info;
        memset(&info, 0, sizeof(info));
        info.pid = (int32_t)pid;
        info.memory_bytes = task_info.pti_resident_size;
        info.virtual_bytes = task_info.pti_virtual_size;
        info.threads = (uint32_t)task_info.pti_threadnum;

        struct proc_bsdinfo bsd_info;
        if (proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, &bsd_info, sizeof(bsd_info)) ==
            (int)sizeof(bsd_info)) {
            info.ppid = (int32_t)bsd_info.pbi_ppid;
            info.start_time_us = (uint64_t)bsd_info.pbi_start_tvsec * 1000000ULL +
                                 (uint64_t)bsd_info.pbi_start_tvusec;
            if (bsd_info.pbi_name[0] != '\0') {
                snprintf(info.name, sizeof(info.name), "%s", bsd_info.pbi_name);
            } else {
                snprintf(info.name, sizeof(info.name), "%s", bsd_info.pbi_comm);
            }
        }
        if (info.name[0] == '\0') {
            proc_name(pid, info.name, sizeof(info.name));
        }
        if (info.name[0] == '\0') {
            snprintf(info.name, sizeof(info.name), "pid:%d", pid);
        }

        uint64_t total_ns = mach_ticks_to_ns(task_info.pti_total_user + task_info.pti_total_system);
        uint64_t prev_ns = 0;
        uint64_t prev_mono = 0;
        if (find_prev_proc(pid, info.start_time_us, &prev_ns, &prev_mono) &&
            now_mono_ns > prev_mono && total_ns >= prev_ns) {
            double dt_ns = (double)(now_mono_ns - prev_mono);
            info.cpu_pct = ((double)(total_ns - prev_ns) / dt_ns) * 100.0;
        } else {
            info.cpu_pct = 0.0;
        }

        if (next_sample_count < capacity) {
            next_samples[next_sample_count].pid = pid;
            next_samples[next_sample_count].start_time_us = info.start_time_us;
            next_samples[next_sample_count].total_ns = total_ns;
            next_samples[next_sample_count].mono_ns = now_mono_ns;
            next_sample_count++;
        }

        all_procs[collected++] = info;
    }

    if (next_sample_count > MAX_TRACKED_PIDS) {
        next_sample_count = MAX_TRACKED_PIDS;
    }
    memcpy(g_prev_procs, next_samples, next_sample_count * sizeof(proc_cpu_sample_t));
    g_prev_proc_count = next_sample_count;

    qsort(all_procs, collected, sizeof(syntrix_process_info_t), compare_procs);

    uint32_t to_copy = collected;
    if (to_copy > max_procs) {
        to_copy = max_procs;
    }
    if (to_copy > 0) {
        memcpy(out_procs, all_procs, to_copy * sizeof(syntrix_process_info_t));
    }

    *out_count = to_copy;
    *out_total_count = collected;

    free(pids);
    free(all_procs);
    free(next_samples);
    return SYNTRIX_OK;
}
