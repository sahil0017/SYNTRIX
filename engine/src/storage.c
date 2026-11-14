#include "syntrix_internal.h"

#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/IOKitLib.h>
#include <string.h>
#include <sys/mount.h>

static uint64_t g_prev_read_bytes = 0;
static uint64_t g_prev_write_bytes = 0;
static uint64_t g_prev_read_ops = 0;
static uint64_t g_prev_write_ops = 0;
static uint64_t g_prev_disk_mono_ns = 0;
static bool g_disk_io_has_prev = false;

static uint64_t cf_dict_u64(CFDictionaryRef dict, CFStringRef key) {
    if (!dict || !key) {
        return 0;
    }
    CFTypeRef val = CFDictionaryGetValue(dict, key);
    if (!val || CFGetTypeID(val) != CFNumberGetTypeID()) {
        return 0;
    }
    uint64_t out = 0;
    CFNumberGetValue((CFNumberRef)val, kCFNumberSInt64Type, &out);
    return out;
}

static void collect_iokit_block_stats(
    uint64_t *out_read_bytes,
    uint64_t *out_write_bytes,
    uint64_t *out_read_ops,
    uint64_t *out_write_ops
) {
    *out_read_bytes = 0;
    *out_write_bytes = 0;
    *out_read_ops = 0;
    *out_write_ops = 0;

    CFMutableDictionaryRef match = IOServiceMatching("IOBlockStorageDriver");
    if (!match) {
        return;
    }
    io_iterator_t iter = 0;
    if (IOServiceGetMatchingServices(0, match, &iter) != KERN_SUCCESS) {
        return;
    }

    io_registry_entry_t entry;
    while ((entry = IOIteratorNext(iter)) != 0) {
        CFTypeRef statsRef = IORegistryEntryCreateCFProperty(
            entry, CFSTR("Statistics"), kCFAllocatorDefault, 0);
        if (statsRef && CFGetTypeID(statsRef) == CFDictionaryGetTypeID()) {
            CFDictionaryRef stats = (CFDictionaryRef)statsRef;
            *out_read_bytes += cf_dict_u64(stats, CFSTR("Bytes (Read)"));
            *out_write_bytes += cf_dict_u64(stats, CFSTR("Bytes (Write)"));
            *out_read_ops += cf_dict_u64(stats, CFSTR("Operations (Read)"));
            *out_write_ops += cf_dict_u64(stats, CFSTR("Operations (Write)"));
        }
        if (statsRef) {
            CFRelease(statsRef);
        }
        IOObjectRelease(entry);
    }
    IOObjectRelease(iter);
}

syntrix_status_t syntrix_disk_collect(syntrix_disk_metrics_t *out) {
    if (!out) {
        return SYNTRIX_ERR_INVALID_ARG;
    }
    memset(out, 0, sizeof(*out));

    struct statfs st;
    if (statfs("/", &st) != 0) {
        return SYNTRIX_ERR_SYS_CALL;
    }

    uint64_t bsize = (uint64_t)st.f_bsize;
    out->total_bytes = (uint64_t)st.f_blocks * bsize;
    out->free_bytes = (uint64_t)st.f_bfree * bsize;
    out->available_bytes = (uint64_t)st.f_bavail * bsize;
    if (out->total_bytes > 0) {
        uint64_t used = (out->total_bytes > out->available_bytes)
                            ? (out->total_bytes - out->available_bytes)
                            : 0;
        out->used_pct = (double)used * 100.0 / (double)out->total_bytes;
    }

    uint64_t r_bytes = 0, w_bytes = 0, r_ops = 0, w_ops = 0;
    collect_iokit_block_stats(&r_bytes, &w_bytes, &r_ops, &w_ops);
    uint64_t now_mono = syntrix_monotonic_ns();
    if (g_disk_io_has_prev && now_mono > g_prev_disk_mono_ns) {
        double dt_sec = (double)(now_mono - g_prev_disk_mono_ns) / 1000000000.0;
        if (dt_sec > 0.0) {
            if (r_bytes >= g_prev_read_bytes) {
                out->read_bytes_per_sec = (double)(r_bytes - g_prev_read_bytes) / dt_sec;
            }
            if (w_bytes >= g_prev_write_bytes) {
                out->write_bytes_per_sec = (double)(w_bytes - g_prev_write_bytes) / dt_sec;
            }
            if (r_ops >= g_prev_read_ops) {
                out->read_iops = (double)(r_ops - g_prev_read_ops) / dt_sec;
            }
            if (w_ops >= g_prev_write_ops) {
                out->write_iops = (double)(w_ops - g_prev_write_ops) / dt_sec;
            }
        }
    }
    g_prev_read_bytes = r_bytes;
    g_prev_write_bytes = w_bytes;
    g_prev_read_ops = r_ops;
    g_prev_write_ops = w_ops;
    g_prev_disk_mono_ns = now_mono;
    g_disk_io_has_prev = true;

    return SYNTRIX_OK;
}
