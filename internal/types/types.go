package types

import "time"

type Config struct {
	Resource            string
	Cycles              int
	LoadLevels          []int
	StabilizeDuration   time.Duration
	MeasurementDuration time.Duration
	MeasurementInterval time.Duration
	CooldownDuration    time.Duration
	DataSource          string
	OutputFile          string
	UseSystemPower      bool
	Verbose             bool

	StreamPath      string
	StreamWorkdir   string
	StorageStressor string

	NetworkServer string
	NetworkPort   int
}

type CycleData struct {
	LoadLevel       int       `json:"load_level"`
	MeasuredUtil    float64   `json:"measured_util"`
	PowerValues     []float64 `json:"power_values"`
	UtilizationVals []float64 `json:"util_values"`
}

type ResourceProfile struct {
	Profile map[string]float64 `json:"profile"`
	Avg     float64            `json:"avg"`
}

type BenchmarkResults struct {
	PowerProfile PowerProfile `json:"power_profile"`
}

type PowerProfile struct {
	Compute  *ResourceProfile `json:"compute,omitempty"`
	Memorize *ResourceProfile `json:"memorize,omitempty"`
	Store    *ResourceProfile `json:"store,omitempty"`
	Transfer *ResourceProfile `json:"transfer,omitempty"`
	TotalAvg float64          `json:"total_avg"`
}
