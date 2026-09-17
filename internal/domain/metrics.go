package domain

import "time"

// MetricSample is one recorded resource sample; InstanceID nil = the host
// (F-1.3). CPU is percent of one core for an instance (matches
// InstanceStatus.CPUPercent) or percent across all cores for the host
// (matches HostMetrics.CPUPercent); Mem is resident bytes for an instance or
// used bytes for the host. Hourly marks a row produced by Downsample rather
// than a raw 60s sample.
type MetricSample struct {
	InstanceID *string
	At         time.Time
	CPU        float64
	Mem        int64
	Players    int
	DiskFree   int64
	Hourly     bool
}

// MetricSeries is the API shape returned by GET .../metrics: parallel arrays
// of equal length, ascending by time. Never nil, even when empty.
type MetricSeries struct {
	TS       []string  `json:"ts"`
	CPU      []float64 `json:"cpu"`
	Mem      []int64   `json:"mem"`
	Players  []int     `json:"players"`
	DiskFree []int64   `json:"disk_free"`
}
