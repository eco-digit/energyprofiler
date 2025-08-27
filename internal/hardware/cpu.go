package hardware

import (
	"fmt"
	"github.com/shirou/gopsutil/v3/cpu"
	"runtime"
)

func DiscoverCPUHardware() (*CPUInfo, error) {
	cpuInfo := &CPUInfo{
		Architecture: runtime.GOARCH,
	}

	// Get logical (thread) count
	threads, err := cpu.Counts(true)
	if err != nil {
		return nil, fmt.Errorf("failed to get CPU thread count: %w", err)
	}
	cpuInfo.Threads = threads

	// Get physical core count
	cores, err := cpu.Counts(false)
	if err != nil {
		return nil, fmt.Errorf("failed to get CPU core count: %w", err)
	}
	cpuInfo.Cores = cores

	return cpuInfo, nil
}
