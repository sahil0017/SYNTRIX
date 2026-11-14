#include "syntrix_internal.h"

#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/IOKitLib.h>
#include <string.h>

static double cf_dict_num_double(CFDictionaryRef dict, CFStringRef key) {
    if (!dict || !key) {
        return -1.0;
    }
    CFTypeRef val = CFDictionaryGetValue(dict, key);
    if (!val || CFGetTypeID(val) != CFNumberGetTypeID()) {
        return -1.0;
    }
    double out = 0.0;
    if (!CFNumberGetValue((CFNumberRef)val, kCFNumberDoubleType, &out)) {
        return -1.0;
    }
    return out;
}

static uint64_t cf_dict_num_u64(CFDictionaryRef dict, CFStringRef key) {
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

syntrix_status_t syntrix_gpu_collect(syntrix_gpu_metrics_t *out) {
    if (!out) {
        return SYNTRIX_ERR_INVALID_ARG;
    }
    memset(out, 0, sizeof(*out));

    CFMutableDictionaryRef match = IOServiceMatching("IOAccelerator");
    if (!match) {
        return SYNTRIX_OK;
    }
    io_iterator_t iter = 0;
    if (IOServiceGetMatchingServices(0, match, &iter) != KERN_SUCCESS) {
        return SYNTRIX_OK;
    }

    io_registry_entry_t entry;
    while ((entry = IOIteratorNext(iter)) != 0) {
        CFTypeRef perfRef = IORegistryEntryCreateCFProperty(
            entry, CFSTR("PerformanceStatistics"), kCFAllocatorDefault, 0);
        if (perfRef && CFGetTypeID(perfRef) == CFDictionaryGetTypeID()) {
            CFDictionaryRef perf = (CFDictionaryRef)perfRef;
            double dev = cf_dict_num_double(perf, CFSTR("Device Utilization %"));
            double ren = cf_dict_num_double(perf, CFSTR("Renderer Utilization %"));
            double til = cf_dict_num_double(perf, CFSTR("Tiler Utilization %"));
            uint64_t mem = cf_dict_num_u64(perf, CFSTR("In use system memory"));
            if (mem == 0) {
                mem = cf_dict_num_u64(perf, CFSTR("Alloc system memory"));
            }
            if (dev >= 0.0 || ren >= 0.0 || til >= 0.0) {
                out->available = true;
                if (dev >= 0.0 && dev > out->device_util_pct) {
                    out->device_util_pct = dev;
                }
                if (ren >= 0.0 && ren > out->renderer_util_pct) {
                    out->renderer_util_pct = ren;
                }
                if (til >= 0.0 && til > out->tiler_util_pct) {
                    out->tiler_util_pct = til;
                }
            }
            if (mem > out->in_use_memory_bytes) {
                out->in_use_memory_bytes = mem;
            }
        }
        if (perfRef) {
            CFRelease(perfRef);
        }
        IOObjectRelease(entry);
    }
    IOObjectRelease(iter);
    return SYNTRIX_OK;
}
