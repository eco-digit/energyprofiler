package hardware

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func DiscoverCPUHardware() (*CPUInfo, error) {
	cmd := exec.Command("lscpu")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("faileed to run lscpu: %w", err)
	}

	cpuInfo := &CPUInfo{}
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		if strings.Contains(line, "CPU(s):") && !strings.Contains(line, "NUMA") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				cpuInfo.Threads, _ = strconv.Atoi(parts[1])
			}
		}
		if strings.Contains(line, "Core(s) per socket:") {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				cpuInfo.Cores, _ = strconv.Atoi(parts[3])
			}
		}
		if strings.Contains(line, "Architecture:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				cpuInfo.Architecture = parts[1]
			}
		}
	}

	return cpuInfo, nil
}
