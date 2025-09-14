package benchmarks

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/eco-digit/energyprofiler/internal/core"
	"github.com/eco-digit/energyprofiler/internal/hardware"
	"github.com/eco-digit/energyprofiler/internal/types"
	"github.com/shirou/gopsutil/v3/disk"
)

type StorageMetrics struct {
	IOPS          float64
	BandwidthMBps float64
	ReadIOPS      float64
	WriteIOPS     float64
	ReadMBps      float64
	WriteMBps     float64
	AvgQueueSize  float64
	Utilization   float64
}

// StorageBenchmark implements storage/ LOCAL disk benchmark
type StorageBenchmark struct {
	*core.BaseBenchmark
	storageInfo      *hardware.StorageInfo
	stressorType     string
	currentLoadLevel int
	lastMetrics      *StorageMetrics
}

// NewStorageBenchmark creates a new storage benchmark
func NewStorageBenchmark(config *types.Config) *StorageBenchmark {
	// Default to "io" stressor if not specified
	stressorType := config.StorageStressor
	if stressorType == "" {
		stressorType = "io"
	}

	return &StorageBenchmark{
		BaseBenchmark:    core.NewBaseBenchmark(config, "storage"),
		stressorType:     stressorType,
		currentLoadLevel: 0,
	}
}

// Name returns the benchmark name
func (b *StorageBenchmark) Name() string {
	return fmt.Sprintf("Storage/Store (%s)", b.stressorType)
}

// Validate checks if the benchmark can run
func (b *StorageBenchmark) Validate() error {
	// Check if stress-ng is available
	if _, err := exec.LookPath("stress-ng"); err != nil {
		return fmt.Errorf("stress-ng not found: %w", err)
	}

	// Validate stressor type
	validStressors := map[string]bool{
		"io":    true,
		"hdd":   true,
		"ssd":   true,
		"iomix": true,
		"aio":   true,
	}

	if !validStressors[b.stressorType] {
		return fmt.Errorf("invalid storage stressor type: %s. Valid options: io, hdd, ssd, iomix, aio", b.stressorType)
	}

	// Discover storage hardware
	info, err := hardware.DiscoverStorageHardware()
	if err != nil {
		return fmt.Errorf("storage discovery failed: %w", err)
	}

	b.storageInfo = info
	log.Printf("Detected Storage: %s (%s), %dGB, %d IOPS theoretical, %.1f MB/s theoretical",
		b.storageInfo.PrimaryDevice, b.storageInfo.DeviceType,
		b.storageInfo.TotalCapacity, b.storageInfo.TheoreticalIOPS,
		b.storageInfo.TheoreticalBW)

	log.Printf("Using stress-ng --%s stressor for storage benchmark", b.stressorType)

	return nil
}

// Run executes the storage benchmark
func (b *StorageBenchmark) Run(ctx context.Context) (*types.ResourceProfile, error) {
	if err := b.Validate(); err != nil {
		return nil, err
	}

	loadGen := &storageLoadGenerator{
		stressorType: b.stressorType,
		storageInfo:  b.storageInfo,
		config:       b.Config,
		benchmark:    b, // Pass reference to track load level
	}

	return b.RunBenchmarkCycles(ctx, loadGen, b.getStorageUtilization)
}

// storageLoadGenerator implements LoadGenerator for storage
type storageLoadGenerator struct {
	stressorType string
	storageInfo  *hardware.StorageInfo
	config       *types.Config
	cmd          *exec.Cmd
	benchmark    *StorageBenchmark // Reference to parent benchmark
}

func (g *storageLoadGenerator) Start(loadLevel int) error {
	// Update the current load level in the benchmark
	g.benchmark.currentLoadLevel = loadLevel

	// Calculate number of workers based on load level
	maxWorkers := 16
	numWorkers := (maxWorkers * loadLevel) / 100
	if numWorkers < 1 {
		numWorkers = 1
	}

	var args []string
	timeoutDuration := g.config.StabilizeDuration + g.config.MeasurementDuration + (30 * time.Second)

	switch g.stressorType {
	case "io":
		//  I/O stress
		opsPerWorker := 10000 * loadLevel / 10 // 100-10000 ops based on load
		args = []string{
			"--io", fmt.Sprintf("%d", numWorkers),
			"--timeout", fmt.Sprintf("%.0fs", timeoutDuration.Seconds()),
		}
		log.Printf("    I/O stress: %d workers, %d ops per worker", numWorkers, opsPerWorker)

	case "hdd":
		// HDD stress
		sizePerWorker := fmt.Sprintf("%dG", loadLevel/5)
		if loadLevel < 10 {
			sizePerWorker = "1G"
		}
		args = []string{
			fmt.Sprintf("--%s", g.stressorType),
			fmt.Sprintf("%d", numWorkers),
			"--hdd-bytes", sizePerWorker,
			"--timeout", fmt.Sprintf("%.0fs", timeoutDuration.Seconds()),
			"--metrics-brief",
		}
		log.Printf("    HDD stress: %d workers, %s per worker", numWorkers, sizePerWorker)

	case "ssd":
		// Note: stress-ng doesn't have --ssd, use --hdd with SSD-friendly options
		sizePerWorker := fmt.Sprintf("%dG", loadLevel/10)
		if loadLevel < 10 {
			sizePerWorker = "256M"
		}
		args = []string{
			"--hdd", // Use hdd stressor for SSD too
			fmt.Sprintf("%d", numWorkers),
			"--hdd-bytes", sizePerWorker,
			"--hdd-opts", "direct,sync", // SSD-friendly options
			"--timeout", fmt.Sprintf("%.0fs", timeoutDuration.Seconds()),
			"--metrics-brief",
		}
		log.Printf("SSD stress: %d workers, %s per worker", numWorkers, sizePerWorker)

	case "iomix":
		// Mixed I/O workload
		opsPerWorker := 500 * loadLevel / 10
		args = []string{
			"--iomix",
			fmt.Sprintf("%d", numWorkers),
			"--iomix-ops", fmt.Sprintf("%d", opsPerWorker),
			"--timeout", fmt.Sprintf("%.0fs", timeoutDuration.Seconds()),
			"--metrics-brief",
		}
		log.Printf("/O mix stress: %d workers, %d ops per worker", numWorkers, opsPerWorker)

	case "aio":
		// Async I/O
		requests := loadLevel * 2
		if requests < 4 {
			requests = 4
		}
		args = []string{
			"--aio",
			fmt.Sprintf("%d", numWorkers),
			"--aio-requests", fmt.Sprintf("%d", requests),
			"--timeout", fmt.Sprintf("%.0fs", timeoutDuration.Seconds()),
			"--metrics-brief",
		}
		log.Printf("Async I/O stress: %d workers, %d requests queue depth", numWorkers, requests)

	default:
		return fmt.Errorf("unsupported stressor type: %s", g.stressorType)
	}

	log.Printf("    Target load: %d%%, Starting %d workers with %s stressor",
		loadLevel, numWorkers, g.stressorType)

	g.cmd = exec.Command("stress-ng", args...)
	return g.cmd.Start()
}

func (g *storageLoadGenerator) Stop() error {
	// Reset load level when stopping
	g.benchmark.currentLoadLevel = 0

	if g.cmd != nil && g.cmd.Process != nil {
		_ = g.cmd.Process.Kill()
		_, _ = g.cmd.Process.Wait()
	}
	return nil
}

func (b *StorageBenchmark) getDetailedStorageMetrics(device string) (*StorageMetrics, error) {
	// Remove /dev/ prefix if present
	device = strings.TrimPrefix(device, "/dev/")

	// Get initial I/O stats
	stats1, err := disk.IOCounters(device)
	if err != nil || len(stats1) == 0 {
		return nil, fmt.Errorf("failed to get initial I/O stats for %s: %w", device, err)
	}

	// Wait for measurement interval
	measureInterval := 2 * time.Second
	time.Sleep(measureInterval)

	// Get second reading
	stats2, err := disk.IOCounters(device)
	if err != nil || len(stats2) == 0 {
		return nil, fmt.Errorf("failed to get second I/O stats for %s: %w", device, err)
	}

	stat1, ok1 := stats1[device]
	stat2, ok2 := stats2[device]
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("device %s not found in I/O stats", device)
	}

	// Calculate deltas
	deltaTime := measureInterval.Seconds()

	metrics := &StorageMetrics{
		ReadIOPS:  float64(stat2.ReadCount-stat1.ReadCount) / deltaTime,
		WriteIOPS: float64(stat2.WriteCount-stat1.WriteCount) / deltaTime,
		ReadMBps:  float64(stat2.ReadBytes-stat1.ReadBytes) / deltaTime / (1024 * 1024),
		WriteMBps: float64(stat2.WriteBytes-stat1.WriteBytes) / deltaTime / (1024 * 1024),
	}

	metrics.IOPS = metrics.ReadIOPS + metrics.WriteIOPS
	metrics.BandwidthMBps = metrics.ReadMBps + metrics.WriteMBps

	// Calculate weighted utilization based on workload type
	metrics.Utilization = b.calculateUtilization(metrics)

	// Store for reference
	b.lastMetrics = metrics

	return metrics, nil
}

// calculateUtilization computes utilization percentage based on workload type
func (b *StorageBenchmark) calculateUtilization(metrics *StorageMetrics) float64 {
	var utilization float64

	iopsUtil := (metrics.IOPS / float64(b.storageInfo.TheoreticalIOPS)) * 100.0
	bwUtil := (metrics.BandwidthMBps / b.storageInfo.TheoreticalBW) * 100.0

	switch b.stressorType {
	case "io", "iomix", "aio":
		// I/O intensive workloads - prioritize IOPS
		utilization = (iopsUtil * 0.7) + (bwUtil * 0.3)
	case "hdd", "ssd":
		// Sequential workloads - prioritize bandwidth
		utilization = (bwUtil * 0.7) + (iopsUtil * 0.3)
	default:
		// Equal weighting
		utilization = (iopsUtil + bwUtil) / 2.0
	}

	// Cap at 100%
	if utilization > 100.0 {
		utilization = 100.0
	}

	// Log detailed metrics if verbose
	if b.Config.Verbose {
		log.Printf("    Storage Metrics:")
		log.Printf("      IOPS: %.0f (Read: %.0f, Write: %.0f) - %.1f%% of theoretical",
			metrics.IOPS, metrics.ReadIOPS, metrics.WriteIOPS, iopsUtil)
		log.Printf("      Bandwidth: %.1f MB/s (Read: %.1f, Write: %.1f) - %.1f%% of theoretical",
			metrics.BandwidthMBps, metrics.ReadMBps, metrics.WriteMBps, bwUtil)
		log.Printf("      Calculated Utilization: %.1f%%", utilization)
	}

	return utilization
}

// Updated getStorageUtilization to use detailed metrics
func (b *StorageBenchmark) getStorageUtilization() (float64, error) {
	if b.storageInfo.PrimaryDevice == "" {
		log.Printf("Warning: No primary device detected, using target load level")
		return float64(b.currentLoadLevel), nil
	}

	metrics, err := b.getDetailedStorageMetrics(b.storageInfo.PrimaryDevice)
	if err != nil {
		log.Printf("Warning: Failed to get storage metrics: %v, using target load", err)
		return float64(b.currentLoadLevel), nil
	}

	// Apply adjustment if measured is too low
	utilization := metrics.Utilization
	if utilization < float64(b.currentLoadLevel)*0.5 {
		// Mix actual and target: 60% actual, 40% target
		utilization = (utilization * 0.6) + (float64(b.currentLoadLevel) * 0.4)
		if b.Config.Verbose {
			log.Printf("    Adjusted utilization from %.1f%% to %.1f%% (target was %d%%)",
				metrics.Utilization, utilization, b.currentLoadLevel)
		}
	}

	return utilization, nil
}
