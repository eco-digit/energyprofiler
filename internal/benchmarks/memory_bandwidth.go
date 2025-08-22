package benchmarks

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/eco-digit/energyprofiler/internal/core"
	"github.com/eco-digit/energyprofiler/internal/hardware"
	"github.com/eco-digit/energyprofiler/internal/results"
	"github.com/eco-digit/energyprofiler/internal/types"
	"github.com/eco-digit/energyprofiler/internal/utils"
)

// MemoryBandwidthBenchmark implements memory bandwidth benchmark using STREAM
type MemoryBandwidthBenchmark struct {
	*core.BaseBenchmark
	ramInfo *hardware.RAMInfo
}

// BandwidthConfig represents a bandwidth test configuration
type BandwidthConfig struct {
	Name        string
	Description string
	Threads     int
	MemoryGB    float64
	LoadLevel   int // Percentage level for reporting
}

// NewMemoryBandwidthBenchmark creates a new memory bandwidth benchmark
func NewMemoryBandwidthBenchmark(config *types.Config) *MemoryBandwidthBenchmark {
	return &MemoryBandwidthBenchmark{
		BaseBenchmark: core.NewBaseBenchmark(config, "memory"),
	}
}

// Name returns the benchmark name
func (b *MemoryBandwidthBenchmark) Name() string {
	return "Memory/Bandwidth"
}

// Validate checks if the benchmark can run
func (b *MemoryBandwidthBenchmark) Validate() error {
	// Check if STREAM binary exists
	streamPath := b.Config.StreamPath
	if streamPath == "" {
		streamPath = "./stream"
	}

	if _, err := os.Stat(streamPath); os.IsNotExist(err) {
		// look for stream in PATH
		if _, err := exec.LookPath("stream"); err != nil {
			return fmt.Errorf("STREAM binary not found at %s. Please compile STREAM from http://www.cs.virginia.edu/stream/", streamPath)
		}
	}

	// Discover RAM hardware
	info, err := hardware.DiscoverRAMHardware()
	if err != nil {
		return fmt.Errorf("RAM discovery failed: %w", err)
	}

	b.ramInfo = info
	log.Printf("Detected RAM: %dGB total, %d channels, %s-%d, %.1f GB/s theoretical peak",
		b.ramInfo.TotalCapacity, b.ramInfo.Channels, b.ramInfo.Type,
		b.ramInfo.Speed, b.ramInfo.TheoreticalBW)

	return nil
}

// Run executes the memory bandwidth benchmark
func (b *MemoryBandwidthBenchmark) Run(ctx context.Context) (*types.ResourceProfile, error) {
	if err := b.Validate(); err != nil {
		return nil, err
	}

	// Get bandwidtsh configurations
	bandwidthConfigs := b.getBandwidthConfigurations()

	log.Printf("Testing %d bandwidth configurations:", len(bandwidthConfigs))
	for _, bc := range bandwidthConfigs {
		log.Printf("  %s: %d threads, %.1fGB memory -> %d%% load level",
			bc.Description, bc.Threads, bc.MemoryGB, bc.LoadLevel)
	}

	// Custom benchmark cycle for bandwitdh testing
	return b.runBandwidthBenchmarkCycles(ctx, bandwidthConfigs)
}

// getBandwidthConfigurations returns predefined bandwidth test configurations
func (b *MemoryBandwidthBenchmark) getBandwidthConfigurations() []BandwidthConfig {
	configs := []BandwidthConfig{
		{
			Name:        "minimal",
			Description: "Minimal load (1 thread)",
			Threads:     1,
			MemoryGB:    1.0,
			LoadLevel:   20,
		},
		{
			Name:        "light",
			Description: "Light load (per-channel)",
			Threads:     b.ramInfo.Channels,
			MemoryGB:    float64(b.ramInfo.TotalCapacity) * 0.1,
			LoadLevel:   40,
		},
		{
			Name:        "moderate",
			Description: "Moderate load (2 per channel)",
			Threads:     b.ramInfo.Channels * 2,
			MemoryGB:    float64(b.ramInfo.TotalCapacity) * 0.3,
			LoadLevel:   60,
		},
		{
			Name:        "heavy",
			Description: "Heavy load (4 per channel)",
			Threads:     b.ramInfo.Channels * 4,
			MemoryGB:    float64(b.ramInfo.TotalCapacity) * 0.6,
			LoadLevel:   80,
		},
		{
			Name:        "maximum",
			Description: "Maximum load (8 per channel)",
			Threads:     b.ramInfo.Channels * 8,
			MemoryGB:    float64(b.ramInfo.TotalCapacity) * 0.8,
			LoadLevel:   100,
		},
	}

	// Ensure minimum memory sizes and reasonable thread counts
	for i := range configs {
		if configs[i].MemoryGB < 1.0 {
			configs[i].MemoryGB = 1.0
		}
		if configs[i].Threads > 128 {
			configs[i].Threads = 128
		}
	}

	return configs
}

// runBandwidthBenchmarkCycles runs bandwidth-specific benchmark cycles
func (b *MemoryBandwidthBenchmark) runBandwidthBenchmarkCycles(ctx context.Context, configs []BandwidthConfig) (*types.ResourceProfile, error) {
	log.Printf("Starting memory bandwidth benchmark with %d cycles", b.Config.Cycles)

	aggregator := results.NewAggregator()

	for cycle := 1; cycle <= b.Config.Cycles; cycle++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		log.Printf("Running memory bandwidth benchmark cycle %d/%d", cycle, b.Config.Cycles)

		var cycleData []types.CycleData

		// Measure idle baseline
		if cycle == 1 {
			idleData, err := b.measureIdleBandwidth(ctx)
			if err != nil {
				return nil, fmt.Errorf("idle measurement failed: %w", err)
			}
			cycleData = append(cycleData, idleData)
		}

		// Test each bandwidth configuration
		for _, config := range configs {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}

			log.Printf("  Testing: %s (%d%% load level)", config.Description, config.LoadLevel)

			data, err := b.measureBandwidthConfig(ctx, config)
			if err != nil {
				log.Printf("Warning: Failed to measure %s: %v", config.Name, err)
				continue
			}

			data.LoadLevel = config.LoadLevel
			cycleData = append(cycleData, data)

			log.Printf("  %s (%d%% load level): %.1f%% actual bandwidth, %.2f W",
				config.Description, config.LoadLevel, data.MeasuredUtil,
				utils.CalculateAverage(data.PowerValues))

			// Pause between configurations
			time.Sleep(5 * time.Second)
		}

		aggregator.AddCycle(cycleData)

		// Cooldown between cycles
		if cycle < b.Config.Cycles {
			log.Printf("Cooldown period...")
			time.Sleep(b.Config.CooldownDuration)
		}
	}

	//  percentage based aggregation for bandwidth benchmarks
	return aggregator.GetAggregatedProfileWithPercentages(), nil
}

// measureIdleBandwidth measures idle baseline
func (b *MemoryBandwidthBenchmark) measureIdleBandwidth(ctx context.Context) (types.CycleData, error) {
	log.Printf("  Measuring idle baseline (0%% bandwidth)")

	// Wait for system to chill
	time.Sleep(10 * time.Second)

	var powerValues []float64
	var utilizationVals []float64
	measurementEnd := time.Now().Add(b.Config.MeasurementDuration)

	for time.Now().Before(measurementEnd) {
		select {
		case <-ctx.Done():
			return types.CycleData{}, ctx.Err()
		default:
		}

		power, err := b.PowerReader.ReadPower("memory")
		if err != nil {
			log.Printf("Warning: power read failed agi: %v", err)
		} else {
			powerValues = append(powerValues, power)
		}

		utilizationVals = append(utilizationVals, 0.0)

		if b.Config.Verbose {
			log.Printf("Memory Power: %.2f W, Bandwidth: 0.0%%", power)
		}

		time.Sleep(b.Config.MeasurementInterval)
	}

	return types.CycleData{
		LoadLevel:       0,
		MeasuredUtil:    0.0,
		PowerValues:     powerValues,
		UtilizationVals: utilizationVals,
	}, nil
}

// measureBandwidthConfig measures power for a specific bandwitdh configuration
func (b *MemoryBandwidthBenchmark) measureBandwidthConfig(ctx context.Context, config BandwidthConfig) (types.CycleData, error) {
	// Start continuous STREAM process
	streamCmd, err := b.startContinuousSTREAM(config)
	if err != nil {
		return types.CycleData{}, fmt.Errorf("failed to start STREAM: %w", err)
	}
	defer func() {
		if streamCmd != nil && streamCmd.Process != nil {
			_ = streamCmd.Process.Kill()
			_, _ = streamCmd.Process.Wait()
		}
	}()

	// Wait for STREAM to stabilize
	time.Sleep(b.Config.StabilizeDuration)

	// Sample STREAM bandwidth once to get utilization estimate
	bandwidthUtil, err := b.sampleSTREAMBandwidth(config)
	if err != nil {
		log.Printf("Warning: Failed to sample STREAM bandwidth: %v", err)
		bandwidthUtil = float64(config.LoadLevel) // Fallback to target percentage
	}

	// Collect power measurements
	var powerValues []float64
	var utilizationVals []float64
	measurementEnd := time.Now().Add(b.Config.MeasurementDuration)

	for time.Now().Before(measurementEnd) {
		select {
		case <-ctx.Done():
			return types.CycleData{}, ctx.Err()
		default:
		}

		power, err := b.PowerReader.ReadPower("memory")
		if err != nil {
			log.Printf("Warning: power read failed: %v", err)
		} else {
			powerValues = append(powerValues, power)
		}

		utilizationVals = append(utilizationVals, bandwidthUtil)

		if b.Config.Verbose {
			log.Printf("    Memory Power: %.2f W, Bandwidth: %.1f%%", power, bandwidthUtil)
		}

		time.Sleep(b.Config.MeasurementInterval)
	}

	return types.CycleData{
		MeasuredUtil:    bandwidthUtil,
		PowerValues:     powerValues,
		UtilizationVals: utilizationVals,
	}, nil
}

// startContinuousSTREAM starts STREAM as a continuous background process (gave better result)
func (b *MemoryBandwidthBenchmark) startContinuousSTREAM(config BandwidthConfig) (*exec.Cmd, error) {
	path := b.Config.StreamPath
	if path == "" {
		path = "./stream"
	}

	// Calculate array size based on memory configuration
	arraySize := int64(config.MemoryGB * 1024 * 1024 * 1024 / 3 / 8)

	cmd := exec.Command(path)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("OMP_NUM_THREADS=%d", config.Threads),
		fmt.Sprintf("STREAM_ARRAY_SIZE=%d", arraySize),
		fmt.Sprintf("STREAM_NTIMES=999999"), // Run many iterations
	)
	if b.Config.StreamWorkdir != "" {
		cmd.Dir = b.Config.StreamWorkdir
	}

	// Start the process but don't wait for it to finish
	err := cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start continuous STREAM: %w", err)
	}

	return cmd, nil
}

// sampleSTREAMBandwidth runs a quick STREAM sample to estimate bandwidth utilization
func (b *MemoryBandwidthBenchmark) sampleSTREAMBandwidth(config BandwidthConfig) (float64, error) {
	path := b.Config.StreamPath
	if path == "" {
		path = "./stream"
	}

	// Get array size
	arraySize := int64(config.MemoryGB * 1024 * 1024 * 1024 / 3 / 8)

	// Run a quick STREAM sample
	cmd := exec.Command(path)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("OMP_NUM_THREADS=%d", config.Threads),
		fmt.Sprintf("STREAM_ARRAY_SIZE=%d", arraySize),
		fmt.Sprintf("STREAM_NTIMES=1"),
	)
	if b.Config.StreamWorkdir != "" {
		cmd.Dir = b.Config.StreamWorkdir
	}

	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to run STREAM sample: %w", err)
	}

	// Parse STREAM output for Triad bandwidth
	re := regexp.MustCompile(`(?m)^Triad:\s+([\d.]+)\b`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) < 2 {
		// alternative parsing

		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "Triad") && strings.Contains(line, "Rate") {
				fields := strings.Fields(line)
				for _, field := range fields {
					if val, err := strconv.ParseFloat(field, 64); err == nil && val > 0 {
						mbps := val
						gbps := mbps / 1024.0
						utilizationPercent := (gbps / b.ramInfo.TheoreticalBW) * 100.0
						if utilizationPercent > 100.0 {
							utilizationPercent = 100.0
						}
						return utilizationPercent, nil
					}
				}
			}
		}
		return 0, fmt.Errorf("Triad bandwidth not found in STREAM output")
	}

	mbps, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse STREAM Triad value: %w", err)
	}

	// Convert to percentage of theoretical peak
	gbps := mbps / 1024.0
	utilizationPercent := (gbps / b.ramInfo.TheoreticalBW) * 100.0

	if utilizationPercent > 100.0 {
		utilizationPercent = 100.0
	}
	if utilizationPercent < 0 {
		utilizationPercent = 0
	}

	return utilizationPercent, nil
}
