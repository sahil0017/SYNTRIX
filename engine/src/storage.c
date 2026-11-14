#include "syntrix_internal.h"

#include <string.h>
#include <sys/mount.h>

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
    return SYNTRIX_OK;
}
