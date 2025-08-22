package hardware

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// StorageInfo holds storage hardware information
type StorageInfo struct {
	Devices         []StorageDevice
	PrimaryDevice   string
	TotalCapacity   uint64  // GB
	DeviceType      string  // NVMe, SATA SSD, SATA HDD
	TheoreticalIOPS int     // Estimated max IOPS
	TheoreticalBW   float64 // MB/s
}

// StorageDevice represents a storage device
type StorageDevice struct {
	Name  string
	Size  uint64 // GB
	Type  string // NVMe, SSD, HDD
	Model string
}

// DiscoverStorageHardware discovers storage hardware information
func DiscoverStorageHardware() (*StorageInfo, error) {
	storageInfo := &StorageInfo{}

	// Get storage devices using lsblk
	cmd := exec.Command("lsblk", "-d", "-o", "NAME,SIZE,TYPE,MODEL,ROTA", "-n")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run lsblk: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	var devices []StorageDevice

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[2] == "disk" {
			name := fields[0]
			sizeStr := fields[1]

			model := ""
			if len(fields) >= 4 {
				// Model might have spaces, join remaining fields except last (ROTA)
				if len(fields) >= 5 {
					model = strings.Join(fields[3:len(fields)-1], " ")
				} else {
					model = fields[3]
				}
			}

			// Check rotation (0=SSD, 1=HDD)
			isRotational := false
			if len(fields) >= 5 {
				if fields[len(fields)-1] == "1" {
					isRotational = true
				}
			}

			// Convert size to GB
			size := parseSizeToGB(sizeStr)

			// Determine device type
			deviceType := determineStorageType(name, model, isRotational)

			// Skip loop devices and other non-physical storage
			if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") {
				continue
			}

			devices = append(devices, StorageDevice{
				Name:  name,
				Size:  size,
				Type:  deviceType,
				Model: model,
			})
		}
	}

	storageInfo.Devices = devices

	// Find primary (root filesystem) device or largest device
	primaryDevice := findPrimaryDevice(devices)
	if primaryDevice != nil {
		storageInfo.PrimaryDevice = primaryDevice.Name
		storageInfo.TotalCapacity = primaryDevice.Size
		storageInfo.DeviceType = primaryDevice.Type

		// Estimate performance based on device type
		switch {
		case strings.Contains(primaryDevice.Type, "NVMe"):
			storageInfo.TheoreticalIOPS = 500000 // Modern NVMe can do 500K+ IOPS
			storageInfo.TheoreticalBW = 3500.0   // ~3.5GB/s for PCIe 3.0 NVMe

		case strings.Contains(primaryDevice.Type, "SSD"):
			storageInfo.TheoreticalIOPS = 50000 // SATA SSD ~50K IOPS
			storageInfo.TheoreticalBW = 550.0   // ~550MB/s SATA III limit

		default: // HDD
			storageInfo.TheoreticalIOPS = 200 // HDD ~100-200 IOPS
			storageInfo.TheoreticalBW = 150.0 // ~150MB/s for modern HDD
		}
	}

	return storageInfo, nil
}

// findPrimaryDevice finds the primary storage device
func findPrimaryDevice(devices []StorageDevice) *StorageDevice {
	if len(devices) == 0 {
		return nil
	}

	// Try to find the root filesystem device
	cmd := exec.Command("df", "/")
	output, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		if len(lines) > 1 {
			fields := strings.Fields(lines[1])
			if len(fields) > 0 {
				rootDevice := fields[0]
				// Remove partition number to get base device
				rootDevice = regexp.MustCompile(`\d+$`).ReplaceAllString(rootDevice, "")
				rootDevice = strings.TrimPrefix(rootDevice, "/dev/")

				// Check the list for device
				for i := range devices {
					if devices[i].Name == rootDevice {
						return &devices[i]
					}
				}
			}
		}
	}

	// Fallback: prefer NVMe > SSD > HDD, then largest; weird errors can occur
	var bestDevice *StorageDevice
	bestScore := 0

	for i := range devices {
		device := &devices[i]
		score := 0

		// Type preference
		if strings.Contains(device.Type, "NVMe") {
			score += 1000000
		} else if strings.Contains(device.Type, "SSD") {
			score += 100000
		}

		// Size preference (in GB)
		score += int(device.Size)

		if score > bestScore {
			bestScore = score
			bestDevice = device
		}
	}

	return bestDevice
}

// parseSizeToGB parses size string to GB
func parseSizeToGB(sizeStr string) uint64 {
	// Remove any commas
	sizeStr = strings.ReplaceAll(sizeStr, ",", "")

	re := regexp.MustCompile(`(\d+(?:\.\d+)?)(T|G|M)?`)
	matches := re.FindStringSubmatch(sizeStr)
	if len(matches) >= 2 {
		value, _ := strconv.ParseFloat(matches[1], 64)
		unit := "G"
		if len(matches) >= 3 {
			unit = matches[2]
		}

		switch unit {
		case "T":
			return uint64(value * 1024)
		case "G":
			return uint64(value)
		case "M":
			return uint64(value / 1024)
		default:
			// Assume bytes if no unit
			return uint64(value / 1024 / 1024 / 1024)
		}
	}
	return 0
}

// determineStorageType determines the type of storage device
func determineStorageType(name string, model string, isRotational bool) string {
	// Check device name first
	if strings.HasPrefix(name, "nvme") {
		return "NVMe"
	}

	// Check model string
	modelLower := strings.ToLower(model)
	if strings.Contains(modelLower, "nvme") {
		return "NVMe"
	}

	// Check rotational flag (most reliable for SATA devices)
	if !isRotational {
		if strings.Contains(modelLower, "ssd") || strings.Contains(modelLower, "solid") {
			return "SATA SSD"
		}
		return "SSD"
	}

	//  It's an HDD :D
	return "HDD"
}
