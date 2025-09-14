package hardware

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v3/mem"
)

// DiscoverRAMHardware discovers RAM hardware information
func DiscoverRAMHardware() (*RAMInfo, error) {
	ramInfo := &RAMInfo{}

	// FIRST: Get actual memory from /proc/meminfo as ground truth
	memStat, err := mem.VirtualMemory()
	if err == nil {
		actualGB := memStat.Total / 1024 / 1024 / 1024
		ramInfo.TotalCapacity = actualGB
		log.Printf("Actual memory from /proc/meminfo: %dGB", actualGB)
	}

	// Try dmidecode for additional details (speed, type, channels)
	if err := discoverDetailsWithDmidecode(ramInfo); err != nil {
		log.Printf("dmidecode failed for details, using defaults: %v", err)
		discoverWithFallback(ramInfo)
	}

	// Calculate theoretical bandwidth
	if ramInfo.Speed > 0 && ramInfo.Channels > 0 {
		bandwidthPerChannel := float64(ramInfo.Speed) * 64.0 / 8.0 / 1000.0 // GB/s
		ramInfo.TheoreticalBW = bandwidthPerChannel * float64(ramInfo.Channels)
	}

	return ramInfo, nil
}

func discoverDetailsWithDmidecode(ramInfo *RAMInfo) error {
	cmd := exec.Command("dmidecode", "--type", "memory")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to run dmidecode: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	channelSet := make(map[string]bool)
	dimmCount := 0
	populatedSlots := 0

	// Parse DIMM information
	inMemoryDevice := false
	var currentSize uint64

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Track Memory Device section
		if strings.Contains(line, "Memory Device") {
			inMemoryDevice = true
			currentSize = 0
			continue
		}

		if !inMemoryDevice {
			continue
		}

		// Extract size for this DIMM
		if strings.Contains(line, "Size:") {
			if strings.Contains(line, "No Module") || strings.Contains(line, "Not Installed") || strings.Contains(line, "Unknown") {
				inMemoryDevice = false
				continue
			}

			sizeRe := regexp.MustCompile(`Size:\s*(\d+)\s*(MB|GB)`)
			matches := sizeRe.FindStringSubmatch(line)
			if len(matches) >= 3 {
				size, _ := strconv.ParseUint(matches[1], 10, 64)
				unit := strings.ToUpper(matches[2])
				if unit == "MB" {
					currentSize = size / 1024 // Convert MB to GB
				} else if unit == "GB" {
					currentSize = size
				}

				if currentSize > 0 {
					populatedSlots++
					dimmCount++
				}
			}
		}

		// Extract locator to count channels (only for populated DIMMs)
		if currentSize > 0 && strings.Contains(line, "Locator:") && !strings.Contains(line, "Bank") {
			// Try to extract channel identifier
			if strings.Contains(line, "DIMM") {
				// Look for patterns like "DIMM_A1", "DIMM_B1", etc.
				locatorRe := regexp.MustCompile(`DIMM[\s_-]*([A-H])\d`)
				matches := locatorRe.FindStringSubmatch(line)
				if len(matches) >= 2 {
					channel := matches[1]
					channelSet[channel] = true
				}
			}
		}

		// Extract speed (only once)
		if ramInfo.Speed == 0 && strings.Contains(line, "Speed:") && strings.Contains(line, "MT/s") {
			speedRe := regexp.MustCompile(`(\d+)\s*MT/s`)
			matches := speedRe.FindStringSubmatch(line)
			if len(matches) >= 2 {
				ramInfo.Speed, _ = strconv.Atoi(matches[1])
			}
		}

		// Extract type (only once)
		if ramInfo.Type == "" && strings.Contains(line, "Type:") && strings.Contains(line, "DDR") {
			typeRe := regexp.MustCompile(`Type:\s*(DDR\d+)`)
			matches := typeRe.FindStringSubmatch(line)
			if len(matches) >= 2 {
				ramInfo.Type = matches[1]
			}
		}
	}

	// Set channels based on what we found
	if len(channelSet) > 0 {
		ramInfo.Channels = len(channelSet)
		log.Printf("Detected %d memory channels from DIMM locators", ramInfo.Channels)
	} else if populatedSlots > 0 {
		// Estimate channels from populated slots
		if populatedSlots == 8 || populatedSlots == 16 {
			ramInfo.Channels = 8 // Likely EPYC with 8 channels
		} else if populatedSlots == 4 {
			ramInfo.Channels = 4
		} else if populatedSlots == 2 {
			ramInfo.Channels = 2
		}
		log.Printf("Estimated %d channels from %d populated DIMM slots", ramInfo.Channels, populatedSlots)
	}

	log.Printf("dmidecode: Found %d populated DIMM slots", populatedSlots)

	return nil
}

func discoverWithFallback(ramInfo *RAMInfo) {
	// We already have total capacity from /proc/meminfo

	// Default assumptions for channels based on CPU
	if ramInfo.Channels == 0 {
		cpuInfo := detectCPUType()
		if strings.Contains(cpuInfo, "EPYC") {
			ramInfo.Channels = 8
			log.Printf("Detected AMD EPYC, assuming 8 memory channels")
		} else if strings.Contains(cpuInfo, "Xeon") {
			if strings.Contains(cpuInfo, "Platinum") || strings.Contains(cpuInfo, "Gold") {
				ramInfo.Channels = 8
			} else {
				ramInfo.Channels = 6
			}
			log.Printf("Detected Intel Xeon, assuming %d memory channels", ramInfo.Channels)
		} else {
			ramInfo.Channels = 4 // Conservative default
			log.Printf("Unknown CPU, assuming 4 memory channels")
		}
	}

	// Default memory speed
	if ramInfo.Speed == 0 {
		ramInfo.Speed = 3200
		ramInfo.Type = "DDR4"
		log.Printf("Assuming DDR4-3200 as default since we have this on our AMDs")
	}
}

// should be removed as we already got this information from cpu
func detectCPUType() string {
	cmd := exec.Command("lscpu")
	output, err := cmd.Output()
	if err != nil {
		cpuInfo, _ := os.ReadFile("/proc/cpuinfo")
		return string(cpuInfo)
	}
	return string(output)
}
