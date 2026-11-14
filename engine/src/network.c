#include "syntrix_internal.h"

#include <ifaddrs.h>
#include <net/if.h>
#include <net/if_dl.h>
#include <string.h>
#include <sys/socket.h>

static bool g_net_initialized = false;
static uint64_t g_prev_net_in = 0;
static uint64_t g_prev_net_out = 0;
static uint64_t g_prev_net_mono_ns = 0;

static void read_network_totals(uint64_t *out_in, uint64_t *out_out) {
    *out_in = 0;
    *out_out = 0;

    struct ifaddrs *ifap = NULL;
    if (getifaddrs(&ifap) != 0 || !ifap) {
        return;
    }

    for (struct ifaddrs *ifa = ifap; ifa != NULL; ifa = ifa->ifa_next) {
        if (!ifa->ifa_addr || ifa->ifa_addr->sa_family != AF_LINK) {
            continue;
        }
        if ((ifa->ifa_flags & IFF_LOOPBACK) != 0) {
            continue;
        }
        if ((ifa->ifa_flags & IFF_UP) == 0) {
            continue;
        }
        if (ifa->ifa_data != NULL) {
            const struct if_data *ifd = (const struct if_data *)ifa->ifa_data;
            *out_in += (uint64_t)ifd->ifi_ibytes;
            *out_out += (uint64_t)ifd->ifi_obytes;
        }
    }

    freeifaddrs(ifap);
}

syntrix_status_t syntrix_network_init(void) {
    read_network_totals(&g_prev_net_in, &g_prev_net_out);
    g_prev_net_mono_ns = syntrix_monotonic_ns();
    g_net_initialized = true;
    return SYNTRIX_OK;
}

void syntrix_network_shutdown(void) {
    g_net_initialized = false;
    g_prev_net_in = 0;
    g_prev_net_out = 0;
    g_prev_net_mono_ns = 0;
}

syntrix_status_t syntrix_network_collect(syntrix_network_metrics_t *out) {
    if (!out) {
        return SYNTRIX_ERR_INVALID_ARG;
    }
    memset(out, 0, sizeof(*out));
    if (!g_net_initialized) {
        return SYNTRIX_ERR_NOT_INIT;
    }

    uint64_t cur_in = 0;
    uint64_t cur_out = 0;
    read_network_totals(&cur_in, &cur_out);
    uint64_t now_ns = syntrix_monotonic_ns();

    out->total_bytes_in = cur_in;
    out->total_bytes_out = cur_out;

    if (now_ns > g_prev_net_mono_ns && g_prev_net_mono_ns > 0) {
        double dt_sec = (double)(now_ns - g_prev_net_mono_ns) / 1e9;
        if (dt_sec > 0.0) {
            if (cur_in >= g_prev_net_in) {
                out->bytes_in_per_sec = (double)(cur_in - g_prev_net_in) / dt_sec;
            }
            if (cur_out >= g_prev_net_out) {
                out->bytes_out_per_sec = (double)(cur_out - g_prev_net_out) / dt_sec;
            }
        }
    }

    g_prev_net_in = cur_in;
    g_prev_net_out = cur_out;
    g_prev_net_mono_ns = now_ns;
    return SYNTRIX_OK;
}
