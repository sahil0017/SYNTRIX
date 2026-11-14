#include "syntrix_internal.h"

#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/pwr_mgt/IOPMLib.h>
#include <stdio.h>
#include <string.h>
#include <sys/sysctl.h>

syntrix_status_t syntrix_thermal_collect(syntrix_thermal_metrics_t *out) {
    if (!out) {
        return SYNTRIX_ERR_INVALID_ARG;
    }
    memset(out, 0, sizeof(*out));
    out->cpu_speed_limit_pct = 100.0;
    out->thermal_level = 0;
    snprintf(out->state_label, sizeof(out->state_label), "nominal");

    CFDictionaryRef power_status = NULL;
    if (IOPMCopyCPUPowerStatus(&power_status) == kIOReturnSuccess && power_status != NULL) {
        CFNumberRef speed_ref = CFDictionaryGetValue(power_status, CFSTR("Processor Speed"));
        if (speed_ref) {
            int speed = 100;
            if (CFNumberGetValue(speed_ref, kCFNumberIntType, &speed)) {
                out->cpu_speed_limit_pct = (double)speed;
                if (speed < 100) {
                    out->is_throttled = true;
                }
            }
        }
        CFRelease(power_status);
    }

    int xcpm_level = -1;
    size_t len = sizeof(xcpm_level);
    if (sysctlbyname("machdep.xcpm.cpu_thermal_level", &xcpm_level, &len, NULL, 0) == 0 && xcpm_level >= 0) {
        if (xcpm_level >= 75 || out->cpu_speed_limit_pct < 60.0) {
            out->thermal_level = 3;
            snprintf(out->state_label, sizeof(out->state_label), "critical");
        } else if (xcpm_level >= 40 || out->cpu_speed_limit_pct < 85.0) {
            out->thermal_level = 2;
            snprintf(out->state_label, sizeof(out->state_label), "serious");
        } else if (xcpm_level >= 15 || out->cpu_speed_limit_pct < 100.0) {
            out->thermal_level = 1;
            snprintf(out->state_label, sizeof(out->state_label), "fair");
        }
    } else if (out->cpu_speed_limit_pct < 60.0) {
        out->thermal_level = 3;
        snprintf(out->state_label, sizeof(out->state_label), "critical");
    } else if (out->cpu_speed_limit_pct < 85.0) {
        out->thermal_level = 2;
        snprintf(out->state_label, sizeof(out->state_label), "serious");
    } else if (out->cpu_speed_limit_pct < 100.0) {
        out->thermal_level = 1;
        snprintf(out->state_label, sizeof(out->state_label), "fair");
    }

    return SYNTRIX_OK;
}
