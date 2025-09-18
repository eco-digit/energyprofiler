package benchmarks

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/shirou/gopsutil/v3/net"
	"log"
	"math"
	"os/exec"
	"strings"
	"time"

	"github.com/eco-digit/energyprofiler/internal/core"
	"github.com/eco-digit/energyprofiler/internal/hardware"
	"github.com/eco-digit/energyprofiler/internal/types"
)

type networkStats struct {
	bytesReceived uint64
	bytesSent     uint64
	timestamp     time.Time
}

type NetworkBenchmark struct {
	*core.BaseBenchmark
	networkInfo      *hardware.NetworkInfo
	serverAddr       string
	serverPort       int
	currentBitrate   float64 // Gbps
	currentLoadLevel int
	lastStats        *networkStats
	testMode         string
}

func NewNetworkBenchmark(config *types.Config) *NetworkBenchmark {
	return &NetworkBenchmark{
		BaseBenchmark: core.NewBaseBenchmark(config, "network"),
		serverAddr:    config.NetworkServer,
		serverPort:    config.NetworkPort,
		testMode:      config.NetworkTestMode,
	}
}

func (b *NetworkBenchmark) Name() string {
	switch b.testMode {
	case "receive":
		return "Network/Transfer (Receive)"
	case "bidirectional":
		return "Network/Transfer (Bidirectional)"
	default:
		return "Network/Transfer (Send)"
	}
}

func (b *NetworkBenchmark) Validate() error {
	// Check iperf3
	if _, err := exec.LookPath("iperf3"); err != nil {
		return fmt.Errorf("iperf3 not found: %w", err)
	}

	// Validate server address
	if b.serverAddr == "" {
		return fmt.Errorf("network server address required (--network-server)")
	}

	// Set default port if not specified
	if b.serverPort == 0 {
		b.serverPort = 5201
	}

	// Discover network hardware
	info, err := hardware.DiscoverNetworkHardware()
	if err != nil {
		return fmt.Errorf("network discovery failed: %w", err)
	}

	b.networkInfo = info

	// Validate we have at least one physical NIC
	if b.networkInfo.PhysicalNICs == 0 {
		return fmt.Errorf("no physical network interfaces found")
	}

	// Quick connectivity test
	log.Printf("Testing connectivity to iperf3 server at %s:%d...", b.serverAddr, b.serverPort)
	cmd := exec.Command("iperf3", "-c", b.serverAddr, "-p", fmt.Sprintf("%d", b.serverPort), "-t", "1", "-J")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("cannot connect to iperf3 server at %s:%d: %w", b.serverAddr, b.serverPort, err)
	}

	// Parse JSON output to get actual achieved bandwidth
	var result iperf3Result
	if err := json.Unmarshal(output, &result); err == nil && result.End.SumReceived.BitsPerSecond > 0 {
		achievedGbps := result.End.SumReceived.BitsPerSecond / 1e9
		log.Printf("Connectivity test successful, achieved %.1f Gbps", achievedGbps)

		// Update max achievable if test shows we can do better than estimated
		if achievedGbps > b.networkInfo.MaxAchievable {
			b.networkInfo.MaxAchievable = achievedGbps
			log.Printf("Updated max achievable to %.1f Gbps based on test", achievedGbps)
		}
	}

	return nil
}

func (b *NetworkBenchmark) Run(ctx context.Context) (*types.ResourceProfile, error) {
	if err := b.Validate(); err != nil {
		return nil, err
	}

	loadGen := &networkLoadGenerator{
		serverAddr:   b.serverAddr,
		serverPort:   b.serverPort,
		maxBandwidth: b.networkInfo.MaxAchievable,
		config:       b.Config,
		benchmark:    b,
	}

	return b.RunBenchmarkCycles(ctx, loadGen, b.getNetworkUtilization)
}

type networkLoadGenerator struct {
	serverAddr   string
	serverPort   int
	maxBandwidth float64
	config       *types.Config
	cmd          *exec.Cmd
	benchmark    *NetworkBenchmark
}

func (g *networkLoadGenerator) Start(loadLevel int) error {
	// Update current load level
	g.benchmark.currentLoadLevel = loadLevel

	// Calculate target bandwidth based on test mode
	var targetGbps float64
	if g.benchmark.testMode == "bidirectional" {
		// For bidirectional, target is per direction
		targetGbps = (g.maxBandwidth * float64(loadLevel)) / 100.0
	} else {
		// For unidirectional, target is total
		targetGbps = (g.maxBandwidth * float64(loadLevel)) / 100.0
	}

	g.benchmark.currentBitrate = targetGbps

	// Build iperf3 command
	timeoutDuration := g.config.StabilizeDuration + g.config.MeasurementDuration + (30 * time.Second)

	args := []string{
		"-c", g.serverAddr,
		"-p", fmt.Sprintf("%d", g.serverPort),
		"-t", fmt.Sprintf("%.0f", timeoutDuration.Seconds()),
	}

	// Add test mode specific flags
	switch g.benchmark.testMode {
	case "receive":
		args = append(args, "-R") // Reverse mode

	case "bidirectional":
		args = append(args, "--bidir") // Bidirectional mode
	}

	// Set bandwidth limit
	if loadLevel < 100 {
		if g.benchmark.testMode == "bidirectional" {
			// For bidirectional, set bandwidth per stream
			args = append(args, "-b", fmt.Sprintf("%.1fG", targetGbps))
			log.Printf("    Target load: %d%% (%.1f Gbps each direction)", loadLevel, targetGbps)
		} else {
			args = append(args, "-b", fmt.Sprintf("%.1fG", targetGbps))
			log.Printf("    Target load: %d%% (%.1f Gbps %s)", loadLevel, targetGbps, g.benchmark.testMode)
		}
	} else {
		log.Printf("    Target load: %d%% (unlimited %s)", loadLevel, g.benchmark.testMode)
	}

	g.cmd = exec.Command("iperf3", args...)

	if err := g.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start iperf3: %w", err)
	}

	return nil
}

func (g *networkLoadGenerator) Stop() error {
	// reset state
	g.benchmark.currentBitrate = 0
	g.benchmark.currentLoadLevel = 0
	g.benchmark.lastStats = nil

	if g.cmd != nil && g.cmd.Process != nil {
		// Kill the process
		if err := g.cmd.Process.Kill(); err != nil {
			// Process might have already exited
			if !strings.Contains(err.Error(), "process already finished") {
				log.Printf("Warning: failed to kill iperf3: %v", err)
			}
		}
		// clean up
		_, _ = g.cmd.Process.Wait()
	}
	return nil
}

func (b *NetworkBenchmark) getNetworkUtilization() (float64, error) {
	// Get current network stats for all physical interfaces
	ioCounters, err := net.IOCounters(true)
	if err != nil {
		return float64(b.currentLoadLevel), err
	}

	var totalBytesRx, totalBytesTx uint64
	currentTime := time.Now()

	// Sum up bytes for all physical interfaces
	for _, iface := range b.networkInfo.Interfaces {
		for _, counter := range ioCounters {
			if counter.Name == iface.Name {
				totalBytesRx += counter.BytesRecv
				totalBytesTx += counter.BytesSent
				break
			}
		}
	}

	// Calculate throughput if we have previous stats
	if b.lastStats != nil {
		deltaTime := currentTime.Sub(b.lastStats.timestamp).Seconds()
		if deltaTime > 0 {
			// Calculate bytes transferred since last measurement
			deltaBytesRx := totalBytesRx - b.lastStats.bytesReceived
			deltaBytesTx := totalBytesTx - b.lastStats.bytesSent

			// Convert to bits per second, then to Gbps
			rxGbps := float64(deltaBytesRx) * 8 / deltaTime / 1e9
			txGbps := float64(deltaBytesTx) * 8 / deltaTime / 1e9

			// Calculate utilization based on test mode
			var utilization float64
			var utilizationMode string

			switch b.testMode {
			case "bidirectional":
				// For bidirectional, use MAX of TX/RX as percentage of per-NIC capacity
				perNicCapacity := b.networkInfo.TotalBandwidth / float64(b.networkInfo.PhysicalNICs)
				utilization = math.Max(rxGbps, txGbps) / perNicCapacity * 100.0
				utilizationMode = "MAX(TX,RX)"

			default:
				// For unidirectional (send/receive), use the dominant direction
				perNicCapacity := b.networkInfo.TotalBandwidth / float64(b.networkInfo.PhysicalNICs)
				if b.testMode == "receive" {
					utilization = rxGbps / perNicCapacity * 100.0
					utilizationMode = "RX"
				} else {
					utilization = txGbps / perNicCapacity * 100.0
					utilizationMode = "TX"
				}
			}

			if b.Config.Verbose {
				log.Printf("    Network throughput: RX %.1f Gbps, TX %.1f Gbps",
					rxGbps, txGbps)
				log.Printf("    Utilization: %.1f%% (%s mode, using %s)",
					utilization, b.testMode, utilizationMode)
			}

			// Update stats for next measurement
			b.lastStats = &networkStats{
				bytesReceived: totalBytesRx,
				bytesSent:     totalBytesTx,
				timestamp:     currentTime,
			}

			// Cap at 100%
			if utilization > 100.0 {
				utilization = 100.0
			}

			return utilization, nil
		}
	}

	// First measurement - just store baseline
	b.lastStats = &networkStats{
		bytesReceived: totalBytesRx,
		bytesSent:     totalBytesTx,
		timestamp:     currentTime,
	}

	return float64(b.currentLoadLevel), nil
}

// iperf3Result structures for parsing JSON output
type iperf3Result struct {
	End struct {
		SumReceived struct {
			BitsPerSecond float64 `json:"bits_per_second"`
		} `json:"sum_received"`
	} `json:"end"`
}
