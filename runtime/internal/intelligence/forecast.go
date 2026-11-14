package intelligence

import (
	"fmt"

	"github.com/syntrix/runtime/internal/store"
)

type StorageForecast struct {
	CurrentUsedPct  float64 `json:"current_used_pct"`
	GrowthPctPerDay float64 `json:"growth_pct_per_day"`
	DaysUntil90Pct  int     `json:"days_until_90_pct"`
	Summary         string  `json:"summary"`
}

func ForecastStorageGrowth(st *store.Store, fallbackUsedPct float64) StorageForecast {
	if st == nil {
		return StorageForecast{
			CurrentUsedPct: fallbackUsedPct,
			DaysUntil90Pct: -1,
			Summary:        fmt.Sprintf("Disk %.1f%% · Insufficient history", fallbackUsedPct),
		}
	}
	records, err := st.RecentSnapshots(300)
	if err != nil || len(records) < 3 {
		cur := fallbackUsedPct
		if len(records) > 0 {
			cur = records[0].DiskUsedPct
		}
		return StorageForecast{
			CurrentUsedPct: cur,
			DaysUntil90Pct: -1,
			Summary:        fmt.Sprintf("Disk %.1f%% · Collecting baseline", cur),
		}
	}

	// records are DESC; iterate oldest -> newest
	n := float64(len(records))
	t0 := float64(records[len(records)-1].TimestampMs)
	const msPerDay = 86400000.0

	var sumX, sumY, sumXY, sumXX float64
	for i := len(records) - 1; i >= 0; i-- {
		x := (float64(records[i].TimestampMs) - t0) / msPerDay
		y := records[i].DiskUsedPct
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}

	denom := n*sumXX - sumX*sumX
	slope := 0.0
	if denom > 1e-12 {
		slope = (n*sumXY - sumX*sumY) / denom
	}

	cur := records[0].DiskUsedPct
	days90 := -1
	if slope > 0.01 && cur < 90.0 {
		days90 = int((90.0 - cur) / slope)
	}

	summary := fmt.Sprintf("Disk %.1f%% · Stable", cur)
	if slope > 0.01 {
		if days90 >= 0 {
			summary = fmt.Sprintf("Disk %.1f%% · +%.2f%%/day (~%dd to 90%%)", cur, slope, days90)
		} else {
			summary = fmt.Sprintf("Disk %.1f%% · +%.2f%%/day", cur, slope)
		}
	}

	return StorageForecast{
		CurrentUsedPct:  cur,
		GrowthPctPerDay: slope,
		DaysUntil90Pct:  days90,
		Summary:         summary,
	}
}
