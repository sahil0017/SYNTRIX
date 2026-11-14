#include "syntrix_internal.h"

#include <mach/mach.h>
#include <mach/vm_page_size.h>
#include <string.h>
#include <sys/sysctl.h>

static uint64_t g_prev_pageins = 0;
static uint64_t g_prev_pageouts = 0;
static uint64_t g_prev_mem_mono_ns = 0;
static bool g_mem_has_prev = false;

syntrix_status_t syntrix_memory_collect(syntrix_memory_metrics_t *out) {
    if (!out) {
        return SYNTRIX_ERR_INVALID_ARG;
    }
    memset(out, 0, sizeof(*out));

    uint64_t total_mem = 0;
    size_t len = sizeof(total_mem);
    if (sysctlbyname("hw.memsize", &total_mem, &len, NULL, 0) != 0) {
        return SYNTRIX_ERR_SYS_CALL;
    }

    vm_statistics64_data_t vm_stat;
    mach_msg_type_number_t count = HOST_VM_INFO64_COUNT;
    kern_return_t kr = host_statistics64(
        mach_host_self(),
        HOST_VM_INFO64,
        (host_info64_t)&vm_stat,
        &count
    );
    if (kr != KERN_SUCCESS) {
        return SYNTRIX_ERR_SYS_CALL;
    }

    uint64_t page_size = (uint64_t)vm_kernel_page_size;

    out->total_bytes = total_mem;
    out->active_bytes = (uint64_t)vm_stat.active_count * page_size;
    out->inactive_bytes = (uint64_t)vm_stat.inactive_count * page_size;
    out->wired_bytes = (uint64_t)vm_stat.wire_count * page_size;
    out->compressed_bytes = (uint64_t)vm_stat.compressor_page_count * page_size;
    out->free_bytes = (uint64_t)vm_stat.free_count * page_size;

    uint64_t used = out->active_bytes + out->wired_bytes + out->compressed_bytes;
    if (used > total_mem) {
        used = total_mem;
    }
    out->used_bytes = used;
    out->used_pct = (total_mem > 0) ? ((double)used * 100.0 / (double)total_mem) : 0.0;

    struct xsw_usage swap_usage;
    memset(&swap_usage, 0, sizeof(swap_usage));
    size_t swap_len = sizeof(swap_usage);
    if (sysctlbyname("vm.swapusage", &swap_usage, &swap_len, NULL, 0) == 0) {
        out->swap_total_bytes = swap_usage.xsu_total;
        out->swap_used_bytes = swap_usage.xsu_used;
    }

    int mem_level = -1;
    size_t level_len = sizeof(mem_level);
    if (sysctlbyname("kern.memorystatus_level", &mem_level, &level_len, NULL, 0) == 0 &&
        mem_level >= 0 && mem_level <= 100) {
        /* kern.memorystatus_level is % of memory still available; invert for pressure. */
        out->pressure_pct = (double)(100 - mem_level);
    } else {
        out->pressure_pct = out->used_pct;
    }

    if (out->pressure_pct >= 85.0 ||
        (out->swap_total_bytes > 0 && out->swap_used_bytes > (2ULL * 1024ULL * 1024ULL * 1024ULL))) {
        out->pressure_level = SYNTRIX_PRESSURE_CRITICAL;
    } else if (out->pressure_pct >= 65.0) {
        out->pressure_level = SYNTRIX_PRESSURE_WARN;
    } else {
        out->pressure_level = SYNTRIX_PRESSURE_NORMAL;
    }

    uint64_t now_mono = syntrix_monotonic_ns();
    if (g_mem_has_prev && now_mono > g_prev_mem_mono_ns) {
        double dt_sec = (double)(now_mono - g_prev_mem_mono_ns) / 1000000000.0;
        if (dt_sec > 0.0) {
            if (vm_stat.pageins >= g_prev_pageins) {
                out->pageins_per_sec = (double)(vm_stat.pageins - g_prev_pageins) / dt_sec;
            }
            if (vm_stat.pageouts >= g_prev_pageouts) {
                out->pageouts_per_sec = (double)(vm_stat.pageouts - g_prev_pageouts) / dt_sec;
            }
        }
    }
    g_prev_pageins = vm_stat.pageins;
    g_prev_pageouts = vm_stat.pageouts;
    g_prev_mem_mono_ns = now_mono;
    g_mem_has_prev = true;

    return SYNTRIX_OK;
}
