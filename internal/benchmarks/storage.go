package benchmarks

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"time"

	"github.com/eco-digit/energyprofiler/internal/core"
	"github.com/eco-digit/energyprofiler/internal/hardware"
	"github.com/eco-digit/energyprofiler/internal/types"
	"github.com/shirou/gopsutil/v3/disk"
)

// StorageBenchmark implements storage/ LOCAL disk benchmark
type StorageBenchmark struct {
	*core.BaseBenchmark
	storageInfo      *hardware.StorageInfo
	stressorType     string
	currentLoadLevel int
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

// getStorageUtilization returns storage utilization
func (b *StorageBenchmark) getStorageUtilization() (float64, error) {
	// Return the target load level as utilization
	return float64(b.currentLoadLevel), nil

	// Option 2: Get actual I/O stats (more complex, commented out for now might add this in the future)
	/*
	   if b.storageInfo.PrimaryDevice != "" {
	       return b.getActualIOUtilization(b.storageInfo.PrimaryDevice)
	   }
	   return float64(b.currentLoadLevel), nil
	*/
}

// getActualIOUtilization attempts to measure actual I/O utilization
// Todo for accurate measurement might needed
func (b *StorageBenchmark) getActualIOUtilization(device string) (float64, error) {
	// Get initial I/O stats
	stats1, err := disk.IOCounters(device)
	if err != nil || len(stats1) == 0 {
		return float64(b.currentLoadLevel), nil // Fallback
	}

	// Wait a short time
	time.Sleep(1 * time.Second)

	// Get second reading
	stats2, err := disk.IOCounters(device)
	if err != nil || len(stats2) == 0 {
		return float64(b.currentLoadLevel), nil // Fallback
	}

	// I/O rate
	stat1 := stats1[device]
	stat2 := stats2[device]

	readRate := float64(stat2.ReadBytes - stat1.ReadBytes)    // bytes/sec
	writeRate := float64(stat2.WriteBytes - stat1.WriteBytes) // bytes/sec
	totalRate := (readRate + writeRate) / 1024 / 1024         // MB/s

	// Calculate utilization as percentage of theoretical max
	utilization := (totalRate / b.storageInfo.TheoreticalBW) * 100.0

	// Cap
	if utilization > 100.0 {
		utilization = 100.0
	}

	// minimum based on load level
	if utilization < float64(b.currentLoadLevel)*0.5 {
		utilization = float64(b.currentLoadLevel) * 0.8
	}

	return utilization, nil
}
