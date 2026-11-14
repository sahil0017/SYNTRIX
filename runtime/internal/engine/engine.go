package engine

/*
#cgo CFLAGS: -I${SRCDIR}/../../../engine/include
#cgo LDFLAGS: ${SRCDIR}/../../../engine/build/libsyntrix_engine.a -framework CoreFoundation -framework IOKit
#include "syntrix_engine.h"
*/
import "C"
import (
	"fmt"
	"sync"
)

type PressureLevel string

const (
	PressureNormal   PressureLevel = "normal"
	PressureWarn     PressureLevel = "warn"
	PressureCritical PressureLevel = "critical"
)

type CPUMetrics struct {
	TotalUsagePct float64   `json:"total_usage_pct"`
	UserPct       float64   `json:"user_pct"`
	SystemPct     float64   `json:"system_pct"`
	IdlePct       float64   `json:"idle_pct"`
	CoreCount     int       `json:"core_count"`
	PerCorePct    []float64 `json:"per_core_pct"`
}

type MemoryMetrics struct {
	TotalBytes      uint64        `json:"total_bytes"`
	UsedBytes       uint64        `json:"used_bytes"`
	ActiveBytes     uint64        `json:"active_bytes"`
	InactiveBytes   uint64        `json:"inactive_bytes"`
	WiredBytes      uint64        `json:"wired_bytes"`
	CompressedBytes uint64        `json:"compressed_bytes"`
	FreeBytes       uint64        `json:"free_bytes"`
	SwapTotalBytes  uint64        `json:"swap_total_bytes"`
	SwapUsedBytes   uint64        `json:"swap_used_bytes"`
	UsedPct         float64       `json:"used_pct"`
	PressurePct     float64       `json:"pressure_pct"`
	PressureLevel   PressureLevel `json:"pressure_level"`
}

type DiskMetrics struct {
	TotalBytes     uint64  `json:"total_bytes"`
	FreeBytes      uint64  `json:"free_bytes"`
	AvailableBytes uint64  `json:"available_bytes"`
	UsedPct        float64 `json:"used_pct"`
}

type NetworkMetrics struct {
	TotalBytesIn  uint64  `json:"total_bytes_in"`
	TotalBytesOut uint64  `json:"total_bytes_out"`
	BytesInPerSec float64 `json:"bytes_in_per_sec"`
	BytesOutPerSec float64 `json:"bytes_out_per_sec"`
}

type PowerMetrics struct {
	HasBattery bool    `json:"has_battery"`
	BatteryPct float64 `json:"battery_pct"`
	IsCharging bool    `json:"is_charging"`
	OnACPower  bool    `json:"on_ac_power"`
}

type ThermalMetrics struct {
	ThermalLevel     int32   `json:"thermal_level"`
	CPUSpeedLimitPct float64 `json:"cpu_speed_limit_pct"`
	IsThrottled      bool    `json:"is_throttled"`
	StateLabel       string  `json:"state_label"`
}

type CollectorHealth struct {
	ErrorCount     uint64  `json:"error_count"`
	LastDurationUs uint64  `json:"last_duration_us"`
	Confidence     float64 `json:"confidence"`
	Available      bool    `json:"available"`
}

type ProcessInfo struct {
	PID          int32   `json:"pid"`
	PPID         int32   `json:"ppid"`
	StartTimeUs  uint64  `json:"start_time_us"`
	Name         string  `json:"name"`
	CPUPct       float64 `json:"cpu_pct"`
	MemoryBytes  uint64  `json:"memory_bytes"`
	VirtualBytes uint64  `json:"virtual_bytes"`
	Threads      uint32  `json:"threads"`
}

type Snapshot struct {
	SchemaVersion     uint32          `json:"schema_version"`
	TimestampMs       int64           `json:"timestamp_ms"`
	MonotonicNs       uint64          `json:"monotonic_ns"`
	CPU               CPUMetrics      `json:"cpu"`
	Memory            MemoryMetrics   `json:"memory"`
	Disk              DiskMetrics     `json:"disk"`
	Network           NetworkMetrics  `json:"network"`
	Power             PowerMetrics    `json:"power"`
	Thermal           ThermalMetrics  `json:"thermal"`
	CPUHealth         CollectorHealth `json:"cpu_health"`
	MemHealth         CollectorHealth `json:"mem_health"`
	ProcHealth        CollectorHealth `json:"proc_health"`
	ProcessCountTotal uint32          `json:"process_count_total"`
	TopProcesses      []ProcessInfo   `json:"top_processes"`
}

type Collector struct {
	mu          sync.Mutex
	initialized bool
}

func NewCollector() (*Collector, error) {
	c := &Collector{}
	if err := c.Init(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Collector) Init() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.initialized {
		return nil
	}
	if st := C.syntrix_engine_init(); st != C.SYNTRIX_OK {
		return fmt.Errorf("syntrix_engine_init failed with status %d", int(st))
	}
	c.initialized = true
	return nil
}

func (c *Collector) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.initialized {
		C.syntrix_engine_shutdown()
		c.initialized = false
	}
}

func (c *Collector) Collect(maxProcs uint32) (Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.initialized {
		return Snapshot{}, fmt.Errorf("collector not initialized")
	}

	var cSnap C.syntrix_snapshot_t
	if st := C.syntrix_collect_snapshot(&cSnap, C.uint32_t(maxProcs)); st != C.SYNTRIX_OK {
		return Snapshot{}, fmt.Errorf("syntrix_collect_snapshot failed with status %d", int(st))
	}

	coreCount := int(cSnap.cpu.core_count)
	if coreCount > C.SYNTRIX_MAX_CORES {
		coreCount = C.SYNTRIX_MAX_CORES
	}
	perCore := make([]float64, coreCount)
	for i := 0; i < coreCount; i++ {
		perCore[i] = float64(cSnap.cpu.per_core_usage_pct[i])
	}

	pressureLevel := PressureNormal
	switch cSnap.memory.pressure_level {
	case C.SYNTRIX_PRESSURE_WARN:
		pressureLevel = PressureWarn
	case C.SYNTRIX_PRESSURE_CRITICAL:
		pressureLevel = PressureCritical
	}

	topCount := int(cSnap.top_process_count)
	if topCount > C.SYNTRIX_MAX_TOP_PROCS {
		topCount = C.SYNTRIX_MAX_TOP_PROCS
	}
	procs := make([]ProcessInfo, topCount)
	for i := 0; i < topCount; i++ {
		p := cSnap.top_processes[i]
		procs[i] = ProcessInfo{
			PID:          int32(p.pid),
			PPID:         int32(p.ppid),
			StartTimeUs:  uint64(p.start_time_us),
			Name:         C.GoString(&p.name[0]),
			CPUPct:       float64(p.cpu_pct),
			MemoryBytes:  uint64(p.memory_bytes),
			VirtualBytes: uint64(p.virtual_bytes),
			Threads:      uint32(p.threads),
		}
	}

	toHealth := func(h C.syntrix_collector_health_t) CollectorHealth {
		return CollectorHealth{
			ErrorCount:     uint64(h.error_count),
			LastDurationUs: uint64(h.last_duration_us),
			Confidence:     float64(h.confidence),
			Available:      bool(h.available),
		}
	}

	return Snapshot{
		SchemaVersion: uint32(cSnap.schema_version),
		TimestampMs:   int64(cSnap.timestamp_ms),
		MonotonicNs:   uint64(cSnap.monotonic_ns),
		CPU: CPUMetrics{
			TotalUsagePct: float64(cSnap.cpu.total_usage_pct),
			UserPct:       float64(cSnap.cpu.user_pct),
			SystemPct:     float64(cSnap.cpu.system_pct),
			IdlePct:       float64(cSnap.cpu.idle_pct),
			CoreCount:     coreCount,
			PerCorePct:    perCore,
		},
		Memory: MemoryMetrics{
			TotalBytes:      uint64(cSnap.memory.total_bytes),
			UsedBytes:       uint64(cSnap.memory.used_bytes),
			ActiveBytes:     uint64(cSnap.memory.active_bytes),
			InactiveBytes:   uint64(cSnap.memory.inactive_bytes),
			WiredBytes:      uint64(cSnap.memory.wired_bytes),
			CompressedBytes: uint64(cSnap.memory.compressed_bytes),
			FreeBytes:       uint64(cSnap.memory.free_bytes),
			SwapTotalBytes:  uint64(cSnap.memory.swap_total_bytes),
			SwapUsedBytes:   uint64(cSnap.memory.swap_used_bytes),
			UsedPct:         float64(cSnap.memory.used_pct),
			PressurePct:     float64(cSnap.memory.pressure_pct),
			PressureLevel:   pressureLevel,
		},
		Disk: DiskMetrics{
			TotalBytes:     uint64(cSnap.disk.total_bytes),
			FreeBytes:      uint64(cSnap.disk.free_bytes),
			AvailableBytes: uint64(cSnap.disk.available_bytes),
			UsedPct:        float64(cSnap.disk.used_pct),
		},
		Network: NetworkMetrics{
			TotalBytesIn:   uint64(cSnap.network.total_bytes_in),
			TotalBytesOut:  uint64(cSnap.network.total_bytes_out),
			BytesInPerSec:  float64(cSnap.network.bytes_in_per_sec),
			BytesOutPerSec: float64(cSnap.network.bytes_out_per_sec),
		},
		Power: PowerMetrics{
			HasBattery: bool(cSnap.power.has_battery),
			BatteryPct: float64(cSnap.power.battery_pct),
			IsCharging: bool(cSnap.power.is_charging),
			OnACPower:  bool(cSnap.power.on_ac_power),
		},
		Thermal: ThermalMetrics{
			ThermalLevel:     int32(cSnap.thermal.thermal_level),
			CPUSpeedLimitPct: float64(cSnap.thermal.cpu_speed_limit_pct),
			IsThrottled:      bool(cSnap.thermal.is_throttled),
			StateLabel:       C.GoString(&cSnap.thermal.state_label[0]),
		},
		CPUHealth:         toHealth(cSnap.cpu_health),
		MemHealth:         toHealth(cSnap.mem_health),
		ProcHealth:        toHealth(cSnap.proc_health),
		ProcessCountTotal: uint32(cSnap.process_count_total),
		TopProcesses:      procs,
	}, nil
}
