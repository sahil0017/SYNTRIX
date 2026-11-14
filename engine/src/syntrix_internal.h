#ifndef SYNTRIX_INTERNAL_H
#define SYNTRIX_INTERNAL_H

#include "syntrix_engine.h"

syntrix_status_t syntrix_cpu_init(void);
void syntrix_cpu_shutdown(void);
syntrix_status_t syntrix_cpu_collect(syntrix_cpu_metrics_t *out);

syntrix_status_t syntrix_memory_collect(syntrix_memory_metrics_t *out);

syntrix_status_t syntrix_process_init(void);
void syntrix_process_shutdown(void);
syntrix_status_t syntrix_process_collect(
    syntrix_process_info_t *out_procs,
    uint32_t max_procs,
    uint32_t *out_count,
    uint32_t *out_total_count
);

syntrix_status_t syntrix_disk_collect(syntrix_disk_metrics_t *out);

syntrix_status_t syntrix_network_init(void);
void syntrix_network_shutdown(void);
syntrix_status_t syntrix_network_collect(syntrix_network_metrics_t *out);

syntrix_status_t syntrix_power_collect(syntrix_power_metrics_t *out);
syntrix_status_t syntrix_thermal_collect(syntrix_thermal_metrics_t *out);
syntrix_status_t syntrix_gpu_collect(syntrix_gpu_metrics_t *out);

int64_t syntrix_now_ms(void);
uint64_t syntrix_monotonic_ns(void);

#endif /* SYNTRIX_INTERNAL_H */
