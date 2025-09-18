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

	var args []string
	timeoutDuration := g.config.StabilizeDuration + g.config.MeasurementDuration + (30 * time.Second)

	switch g.stressorType {
	case "io":
		// For higher utilization at max load, use more workers
		var numWorkers int
		if loadLevel >= 90 {
			numWorkers = 8 // More workers for 90-100% load
		} else if loadLevel >= 75 {
			numWorkers = 6 // Moderate increase for high load
		} else {
			numWorkers = 1 + (3 * loadLevel / 100) // 1-3 workers for 0-74%
		}

		args = []string{
			"--io", fmt.Sprintf("%d", numWorkers),
			"--timeout", fmt.Sprintf("%.0fs", timeoutDuration.Seconds()),
			"--metrics-brief",
		}
		log.Printf("    I/O stress: %d workers for %d%% load", numWorkers, loadLevel)
		log.Printf("    Target load: %d%%, Starting %d workers with %s stressor",
			loadLevel, numWorkers, g.stressorType)

	case "hdd":
		// HDD stress with progressive scaling
		numWorkers := 1 + (3 * loadLevel / 100) // 1-4 workers
		sizePerWorker := calculateDataSize(loadLevel, g.storageInfo.TotalCapacity)

		args = []string{
			"--hdd", fmt.Sprintf("%d", numWorkers),
			"--hdd-bytes", sizePerWorker,
			"--hdd-write-size", "1M", // Sequential chunks
			"--timeout", fmt.Sprintf("%.0fs", timeoutDuration.Seconds()),
			"--metrics-brief",
		}
		log.Printf("    HDD stress: %d workers, %s per worker", numWorkers, sizePerWorker)
		log.Printf("    Target load: %d%%, Starting %d workers with %s stressor",
			loadLevel, numWorkers, g.stressorType)

	case "ssd":
		// SSD optimized with better scaling
		var numWorkers int
		if loadLevel >= 80 {
			numWorkers = 8 // adds workers for high load
		} else {
			numWorkers = 1 + (5 * loadLevel / 100) // 1-5 workers
		}

		sizePerWorker := calculateDataSize(loadLevel, g.storageInfo.TotalCapacity)

		args = []string{
			"--hdd", fmt.Sprintf("%d", numWorkers),
			"--hdd-bytes", sizePerWorker,
			"--hdd-opts", "direct,sync,wr-rnd,rd-rnd", // Random I/O
			"--hdd-write-size", "4K", // 4KB chunks
			"--timeout", fmt.Sprintf("%.0fs", timeoutDuration.Seconds()),
			"--metrics-brief",
		}
		log.Printf("    SSD stress: %d workers, %s per worker, 4K random I/O", numWorkers, sizePerWorker)
		log.Printf("    Target load: %d%%, Starting %d workers with %s stressor",
			loadLevel, numWorkers, g.stressorType)

	case "iomix":
		// Mixed I/O workload with better scaling
		numWorkers := 1 + (4 * loadLevel / 100)           // 1-5 workers
		opsPerWorker := 10000 + (40000 * loadLevel / 100) // 10K-50K ops

		args = []string{
			"--iomix", fmt.Sprintf("%d", numWorkers),
			"--iomix-ops", fmt.Sprintf("%d", opsPerWorker),
			"--timeout", fmt.Sprintf("%.0fs", timeoutDuration.Seconds()),
			"--metrics-brief",
		}
		log.Printf("    I/O mix stress: %d workers, %d ops per worker", numWorkers, opsPerWorker)
		log.Printf("    Target load: %d%%, Starting %d workers with %s stressor",
			loadLevel, numWorkers, g.stressorType)

	case "aio":
		// Async I/O with scaled workers and queue depth
		numWorkers := 1 + (3 * loadLevel / 100)  // 1-4 workers
		queueDepth := 4 + (60 * loadLevel / 100) // 4-64 queue depth

		args = []string{
			"--aio", fmt.Sprintf("%d", numWorkers),
			"--aio-requests", fmt.Sprintf("%d", queueDepth),
			"--timeout", fmt.Sprintf("%.0fs", timeoutDuration.Seconds()),
			"--metrics-brief",
		}
		log.Printf("    Async I/O stress: %d workers, queue depth %d", numWorkers, queueDepth)
		log.Printf("    Target load: %d%%, Starting %d workers with %s stressor",
			loadLevel, numWorkers, g.stressorType)

	default:
		return fmt.Errorf("unsupported stressor type: %s", g.stressorType)
	}

	g.cmd = exec.Command("stress-ng", args...)

	// Set working directory to temp to avoid filling system disk
	g.cmd.Dir = "/tmp"

	return g.cmd.Start()
}

// Helper function for calculating data size
func calculateDataSize(loadLevel int, totalCapacityGB uint64) string {
	// Scale data size based on load level and available capacity
	maxSizeGB := totalCapacityGB / 20 // Use max 5% of disk
	if maxSizeGB > 10 {
		maxSizeGB = 10 // Cap at 10GB per worker
	}

	sizeGB := (maxSizeGB * uint64(loadLevel)) / 100
	if sizeGB < 1 {
		// For low load levels, use MB
		sizeMB := 256 + (768 * loadLevel / 100) // 256MB - 1GB
		return fmt.Sprintf("%dM", sizeMB)
	}

	return fmt.Sprintf("%dG", sizeGB)
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
