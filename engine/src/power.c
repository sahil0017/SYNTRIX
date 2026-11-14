#include "syntrix_internal.h"

#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/ps/IOPSKeys.h>
#include <IOKit/ps/IOPowerSources.h>
#include <string.h>

syntrix_status_t syntrix_power_collect(syntrix_power_metrics_t *out) {
    if (!out) {
        return SYNTRIX_ERR_INVALID_ARG;
    }
    memset(out, 0, sizeof(*out));

    CFTypeRef blob = IOPSCopyPowerSourcesInfo();
    if (!blob) {
        out->on_ac_power = true;
        return SYNTRIX_OK;
    }

    CFArrayRef sources = IOPSCopyPowerSourcesList(blob);
    if (!sources || CFArrayGetCount(sources) == 0) {
        if (sources) {
            CFRelease(sources);
        }
        CFRelease(blob);
        out->on_ac_power = true;
        return SYNTRIX_OK;
    }

    CFIndex count = CFArrayGetCount(sources);
    for (CFIndex i = 0; i < count; i++) {
        CFDictionaryRef desc = IOPSGetPowerSourceDescription(blob, CFArrayGetValueAtIndex(sources, i));
        if (!desc) {
            continue;
        }
        CFNumberRef cur_cap = CFDictionaryGetValue(desc, CFSTR(kIOPSCurrentCapacityKey));
        CFNumberRef max_cap = CFDictionaryGetValue(desc, CFSTR(kIOPSMaxCapacityKey));
        if (cur_cap && max_cap) {
            int cur = 0;
            int max = 0;
            if (CFNumberGetValue(cur_cap, kCFNumberIntType, &cur) &&
                CFNumberGetValue(max_cap, kCFNumberIntType, &max) && max > 0) {
                out->has_battery = true;
                out->battery_pct = (double)cur * 100.0 / (double)max;
            }
        }
        CFBooleanRef charging = CFDictionaryGetValue(desc, CFSTR(kIOPSIsChargingKey));
        if (charging) {
            out->is_charging = CFBooleanGetValue(charging);
        }
        CFStringRef state = CFDictionaryGetValue(desc, CFSTR(kIOPSPowerSourceStateKey));
        if (state && CFStringCompare(state, CFSTR(kIOPSACPowerValue), 0) == kCFCompareEqualTo) {
            out->on_ac_power = true;
        }
    }

    CFRelease(sources);
    CFRelease(blob);
    return SYNTRIX_OK;
}
