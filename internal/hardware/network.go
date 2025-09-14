package hardware

import (
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v3/net"
)

type NetworkInfo struct {
	Interfaces     []NetworkInterface
	PhysicalNICs   int
	TotalBandwidth float64 // Gbps
	MaxAchievable  float64 // Gbps (calculated from total bandwidth)
}

type NetworkInterface struct {
	Name       string
	Speed      int // Mbps
	Driver     string
	IsPhysical bool
	IsUp       bool
}

func DiscoverNetworkHardware() (*NetworkInfo, error) {
	networkInfo := &NetworkInfo{}

	// Get all network interfaces using gopsutil
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get network interfaces: %w", err)
	}

	var totalBandwidth int
	var physicalInterfaces []NetworkInterface

	for _, iface := range interfaces {
		// Skip loopback and virtual interfaces
		if strings.HasPrefix(iface.Name, "lo") ||
			strings.HasPrefix(iface.Name, "veth") ||
			strings.HasPrefix(iface.Name, "br-") ||
			strings.HasPrefix(iface.Name, "ovs-") ||
			strings.HasPrefix(iface.Name, "tap") ||
			strings.HasPrefix(iface.Name, "genev") ||
			strings.Contains(iface.Name, "docker") {
			continue
		}

		// Check if interface has a hardware address (physical interfaces usually do)
		if iface.HardwareAddr == "" {
			continue
		}

		// Get speed from ethtool (gopsutil doesn't provide speed info)
		speed := getInterfaceSpeed(iface.Name)
		if speed == 0 {
			continue // Skip interfaces without detectable speed
		}

		// Get driver info
		driver := getInterfaceDriver(iface.Name)

		// Check if interface is up
		isUp := false
		for _, flag := range iface.Flags {
			if flag == "up" {
				isUp = true
				break
			}
		}

		netIface := NetworkInterface{
			Name:       iface.Name,
			Speed:      speed,
			Driver:     driver,
			IsPhysical: true,
			IsUp:       isUp,
		}

		physicalInterfaces = append(physicalInterfaces, netIface)
		if isUp {
			totalBandwidth += speed
		}
	}

	networkInfo.Interfaces = physicalInterfaces
	networkInfo.PhysicalNICs = len(physicalInterfaces)
	networkInfo.TotalBandwidth = float64(totalBandwidth) / 1000.0 // Convert to Gbps

	// Calculate achievable bandwidth (typically 80% of theoretical for TCP)
	// This accounts for protocol overhead and real-world conditions
	networkInfo.MaxAchievable = networkInfo.TotalBandwidth * 0.8

	// Log discovered interfaces
	log.Printf("Discovered %d physical network interfaces:", networkInfo.PhysicalNICs)
	for _, iface := range physicalInterfaces {
		status := "down"
		if iface.IsUp {
			status = "up"
		}
		log.Printf("  %s: %d Mbps, driver=%s, status=%s",
			iface.Name, iface.Speed, iface.Driver, status)
	}
	log.Printf("Total bandwidth: %.1f Gbps (up interfaces only)", networkInfo.TotalBandwidth)
	log.Printf("Estimated achievable: %.1f Gbps", networkInfo.MaxAchievable)

	return networkInfo, nil
}

// getInterfaceSpeed gets speed from ethtool since gopsutil doesn't provide it
func getInterfaceSpeed(ifaceName string) int {
	cmd := exec.Command("ethtool", ifaceName)
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Speed:") {
			// Parse lines like "Speed: 25000Mb/s"
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				speedStr := strings.TrimSuffix(parts[1], "Mb/s")
				if speed, err := strconv.Atoi(speedStr); err == nil {
					return speed
				}
			}
		}
	}
	return 0
}

// getInterfaceDriver gets driver info from ethtool
func getInterfaceDriver(ifaceName string) string {
	cmd := exec.Command("ethtool", "-i", ifaceName)
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "driver:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1]
			}
		}
	}
	return "unknown"
}
