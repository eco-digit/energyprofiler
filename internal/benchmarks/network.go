package benchmarks

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/eco-digit/energyprofiler/internal/core"
	"github.com/eco-digit/energyprofiler/internal/hardware"
	"github.com/eco-digit/energyprofiler/internal/types"
)

type NetworkBenchmark struct {
	*core.BaseBenchmark
	networkInfo      *hardware.NetworkInfo
	serverAddr       string
	serverPort       int
	currentBitrate   float64 // Gbps
	currentLoadLevel int
}

func NewNetworkBenchmark(config *types.Config) *NetworkBenchmark {
	return &NetworkBenchmark{
		BaseBenchmark: core.NewBaseBenchmark(config, "network"),
		serverAddr:    config.NetworkServer,
		serverPort:    config.NetworkPort,
	}
}

func (b *NetworkBenchmark) Name() string {
	return "Network/Transfer"
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

	// Calculate target bandwidth
	targetGbps := (g.maxBandwidth * float64(loadLevel)) / 100.0
	g.benchmark.currentBitrate = targetGbps

	// Build iperf3 command
	timeoutDuration := g.config.StabilizeDuration + g.config.MeasurementDuration + (30 * time.Second)

	args := []string{
		"-c", g.serverAddr,
		"-p", fmt.Sprintf("%d", g.serverPort),
		"-t", fmt.Sprintf("%.0f", timeoutDuration.Seconds()),
	}

	// less than 100% load, set bandwidth limit
	if loadLevel < 100 {
		args = append(args, "-b", fmt.Sprintf("%.1fG", targetGbps))
		log.Printf("    Target load: %d%% (limited to %.1f Gbps)", loadLevel, targetGbps)
	} else {
		log.Printf("    Target load: %d%% (unlimited, expecting ~%.1f Gbps)", loadLevel, g.maxBandwidth)
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
	return float64(b.currentLoadLevel), nil
}

// iperf3Result structures for parsing JSON output (minimal needed fields)
type iperf3Result struct {
	End struct {
		SumReceived struct {
			BitsPerSecond float64 `json:"bits_per_second"`
		} `json:"sum_received"`
	} `json:"end"`
}
