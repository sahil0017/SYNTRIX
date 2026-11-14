#ifndef SYNTRIX_ENGINE_H
#define SYNTRIX_ENGINE_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define SYNTRIX_SCHEMA_VERSION 2
#define SYNTRIX_MAX_CORES 128
#define SYNTRIX_MAX_TOP_PROCS 64
#define SYNTRIX_PROC_NAME_LEN 64

typedef enum {
    SYNTRIX_OK = 0,
    SYNTRIX_ERR_INVALID_ARG = -1,
    SYNTRIX_ERR_NOT_INIT = -2,
    SYNTRIX_ERR_SYS_CALL = -3,
    SYNTRIX_ERR_ALLOC = -4
} syntrix_status_t;

typedef enum {
    SYNTRIX_PRESSURE_NORMAL = 0,
    SYNTRIX_PRESSURE_WARN = 1,
    SYNTRIX_PRESSURE_CRITICAL = 2
} syntrix_pressure_level_t;

typedef struct {
    double total_usage_pct;
    double user_pct;
    double system_pct;
    double idle_pct;
    uint32_t core_count;
    double per_core_usage_pct[SYNTRIX_MAX_CORES];
} syntrix_cpu_metrics_t;

typedef struct {
    uint64_t total_bytes;
    uint64_t used_bytes;
    uint64_t active_bytes;
    uint64_t inactive_bytes;
    uint64_t wired_bytes;
    uint64_t compressed_bytes;
    uint64_t free_bytes;
    uint64_t swap_total_bytes;
    uint64_t swap_used_bytes;
    double used_pct;
    double pressure_pct;
    syntrix_pressure_level_t pressure_level;
    double pageins_per_sec;
    double pageouts_per_sec;
} syntrix_memory_metrics_t;

typedef struct {
    uint64_t total_bytes;
    uint64_t free_bytes;
    uint64_t available_bytes;
    double used_pct;
    double read_bytes_per_sec;
    double write_bytes_per_sec;
    double read_iops;
    double write_iops;
} syntrix_disk_metrics_t;

typedef struct {
    uint64_t total_bytes_in;
    uint64_t total_bytes_out;
    double bytes_in_per_sec;
    double bytes_out_per_sec;
} syntrix_network_metrics_t;

typedef struct {
    bool has_battery;
    double battery_pct;
    bool is_charging;
    bool on_ac_power;
} syntrix_power_metrics_t;

typedef struct {
    int32_t thermal_level;       /* 0=nominal, 1=fair, 2=serious, 3=critical */
    double cpu_speed_limit_pct;  /* 100.0 = unthrottled */
    bool is_throttled;
    char state_label[16];
} syntrix_thermal_metrics_t;

typedef struct {
    bool available;
    double device_util_pct;
    double renderer_util_pct;
    double tiler_util_pct;
    uint64_t in_use_memory_bytes;
} syntrix_gpu_metrics_t;

typedef struct {
    uint64_t error_count;
    uint64_t last_duration_us;
    double confidence; /* 0.0 = unavailable, 0.5 = partial/inferred, 1.0 = direct */
    bool available;
} syntrix_collector_health_t;

typedef struct {
    int32_t pid;
    int32_t ppid;
    uint64_t start_time_us;
    char name[SYNTRIX_PROC_NAME_LEN];
    double cpu_pct;
    uint64_t memory_bytes;
    uint64_t virtual_bytes;
    uint32_t threads;
    uint64_t phys_footprint_bytes;
    double disk_read_bps;
    double disk_write_bps;
    double pageins_per_sec;
    double wakeups_per_sec;
} syntrix_process_info_t;

typedef struct {
    uint32_t schema_version;
    int64_t timestamp_ms;
    uint64_t monotonic_ns;
    syntrix_cpu_metrics_t cpu;
    syntrix_memory_metrics_t memory;
    syntrix_disk_metrics_t disk;
    syntrix_network_metrics_t network;
    syntrix_power_metrics_t power;
    syntrix_thermal_metrics_t thermal;
    syntrix_gpu_metrics_t gpu;
    syntrix_collector_health_t cpu_health;
    syntrix_collector_health_t mem_health;
    syntrix_collector_health_t proc_health;
    syntrix_collector_health_t disk_health;
    syntrix_collector_health_t net_health;
    syntrix_collector_health_t power_health;
    syntrix_collector_health_t thermal_health;
    uint32_t process_count_total;
    uint32_t top_process_count;
    syntrix_process_info_t top_processes[SYNTRIX_MAX_TOP_PROCS];
} syntrix_snapshot_t;

/*
 * Initializes internal delta-tracking state for CPU, network, and process
 * sampling. Must be called before syntrix_collect_snapshot().
 */
syntrix_status_t syntrix_engine_init(void);

/*
 * Releases any internal state held by the engine.
 */
void syntrix_engine_shutdown(void);

/*
 * Collects a full system telemetry snapshot into caller-owned `out`.
 * `max_procs` is clamped to [0, SYNTRIX_MAX_TOP_PROCS].
 */
syntrix_status_t syntrix_collect_snapshot(syntrix_snapshot_t *out, uint32_t max_procs);

#ifdef __cplusplus
}
#endif

#endif /* SYNTRIX_ENGINE_H */
