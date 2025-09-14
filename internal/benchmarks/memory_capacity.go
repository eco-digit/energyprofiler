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
	"github.com/shirou/gopsutil/v3/mem"
)

// MemoryCapacityBenchmark implements memory capacity benchmark
type MemoryCapacityBenchmark struct {
	*core.BaseBenchmark
	ramInfo *hardware.RAMInfo
}

// NewMemoryCapacityBenchmark creates a new memory capacity benchmark
func NewMemoryCapacityBenchmark(config *types.Config) *MemoryCapacityBenchmark {
	return &MemoryCapacityBenchmark{
		BaseBenchmark: core.NewBaseBenchmark(config, "memory"),
	}
}

// Name returns the benchmark name
func (b *MemoryCapacityBenchmark) Name() string {
	return "Memory/Capacity"
}

// Validate checks if the benchmark can run
func (b *MemoryCapacityBenchmark) Validate() error {
	// Check if stress-ng is available
	if _, err := exec.LookPath("stress-ng"); err != nil {
		return fmt.Errorf("stress-ng not found: %w", err)
	}

	// Discover RAM hardware (even if wrong, we still log it)
	info, err := hardware.DiscoverRAMHardware()
	if err != nil {
		return fmt.Errorf("RAM discovery failed: %w", err)
	}

	b.ramInfo = info

	// how actual available memory for comparison
	memStat, _ := mem.VirtualMemory()
	actualGB := float64(memStat.Total) / 1024 / 1024 / 1024

	log.Printf("Detected RAM: %dGB reported by dmidecode", b.ramInfo.TotalCapacity)
	log.Printf("Actual RAM: %.1fGB from /proc/meminfo", actualGB)
	log.Printf("RAM details: %d channels, %s-%d, %.1f GB/s theoretical peak",
		b.ramInfo.Channels, b.ramInfo.Type, b.ramInfo.Speed, b.ramInfo.TheoreticalBW)

	return nil
}

// Run executes the memory capacity benchmark
func (b *MemoryCapacityBenchmark) Run(ctx context.Context) (*types.ResourceProfile, error) {
	if err := b.Validate(); err != nil {
		return nil, err
	}

	loadGen := &memoryCapacityLoadGenerator{
		config: b.Config,
	}

	return b.RunBenchmarkCycles(ctx, loadGen, getCurrentMemoryCapacityUtil)
}

// memoryCapacityLoadGenerator implements LoadGenerator for memory capacity
type memoryCapacityLoadGenerator struct {
	config *types.Config
	cmd    *exec.Cmd
}

func (g *memoryCapacityLoadGenerator) Start(loadLevel int) error {
	// Get current memory state for logging
	memStat, _ := mem.VirtualMemory()
	currentUsedGB := float64(memStat.Used) / 1024 / 1024 / 1024
	availableGB := float64(memStat.Available) / 1024 / 1024 / 1024
	totalGB := float64(memStat.Total) / 1024 / 1024 / 1024

	log.Printf("Current memory: %.1fGB used of %.1fGB total (%.1fGB available)",
		currentUsedGB, totalGB, availableGB)

	// Cap at 95% to avoid OOM
	targetPercent := loadLevel
	if targetPercent > 95 {
		targetPercent = 95
		log.Printf("    Capping at 95%% to avoid OOM")
	}

	// For very low percentages, ensure we allocate something meaningful
	if targetPercent < 5 {
		targetPercent = 5
	}

	log.Printf("    Target: %d%% of system memory", targetPercent)

	// get timeout
	timeoutDuration := g.config.StabilizeDuration + g.config.MeasurementDuration + (30 * time.Second)

	// Use stress-ng w
	g.cmd = exec.Command("stress-ng",
		"--vm", "1", // Single VM worker
		"--vm-bytes", fmt.Sprintf("%d%%", targetPercent),
		"--vm-populate",  // Pre-fault all pages
		"--vm-locked",    // Lock in memory
		"--vm-hang", "0", // Keep allocated indefinitely (0 = forever)
		"--timeout", fmt.Sprintf("%.0fs", timeoutDuration.Seconds()),
		"--metrics-brief", // Show summary at end
	)

	log.Printf("Running: stress-ng --vm 1 --vm-bytes %d%% --vm-populate --vm-locked --vm-hang 0", targetPercent)

	return g.cmd.Start()
}

func (g *memoryCapacityLoadGenerator) Stop() error {
	if g.cmd != nil && g.cmd.Process != nil {
		_ = g.cmd.Process.Kill()
		_, _ = g.cmd.Process.Wait()
	}
	return nil
}

// getCurrentMemoryCapacityUtil() gets current memory capacity utilization percentage
func getCurrentMemoryCapacityUtil() (float64, error) {
	memStat, err := mem.VirtualMemory()
	if err != nil {
		return 0, fmt.Errorf("failed to get memory capacity utilization: %w", err)
	}
	return memStat.UsedPercent, nil
}
