package benchmarks

import (
	"context"
	"fmt"
	"github.com/eco-digit/energyprofiler/internal/core"
	"log"
	"os/exec"
	"strconv"
	"time"

	"github.com/eco-digit/energyprofiler/internal/hardware"
	"github.com/eco-digit/energyprofiler/internal/types"
	"github.com/shirou/gopsutil/v3/cpu"
)

type CPUBenchmark struct {
	*core.BaseBenchmark
	cpuInfo *hardware.CPUInfo
}

func NewCPUBenchmark(config *types.Config) *CPUBenchmark {
	return &CPUBenchmark{
		BaseBenchmark: core.NewBaseBenchmark(config, "cpu"),
	}
}

func (b *CPUBenchmark) Name() string {
	return "CPU/Compute"
}

func (b *CPUBenchmark) Validate() error {
	if _, err := exec.LookPath("stress-ng"); err != nil {
		return fmt.Errorf("stress-ng not found: %w", err)
	}

	info, err := hardware.DiscoverCPUHardware()
	if err != nil {
		return fmt.Errorf("CPU discovery failed: %w", err)
	}

	b.cpuInfo = info
	log.Printf("Detected CPU: %d cores, %d threads, %s",
		b.cpuInfo.Cores, b.cpuInfo.Threads, b.cpuInfo.Architecture)

	return nil
}

func (b *CPUBenchmark) Run(ctx context.Context) (*types.ResourceProfile, error) {
	if err := b.Validate(); err != nil {
		return nil, err
	}

	loadGen := &cpuLoadGenerator{
		threads: b.cpuInfo.Threads,
		config:  b.Config,
	}

	return b.RunBenchmarkCycles(ctx, loadGen, getCurrentCPUUtilization)
}

type cpuLoadGenerator struct {
	threads int
	config  *types.Config
	cmd     *exec.Cmd
}

func (g *cpuLoadGenerator) Start(loadLevel int) error {
	timeoutDuration := g.config.StabilizeDuration + g.config.MeasurementDuration + (30 * time.Second)

	g.cmd = exec.Command("stress-ng",
		"--cpu", strconv.Itoa(g.threads),
		"--cpu-load", strconv.Itoa(loadLevel),
		"--timeout", fmt.Sprintf("%.0fs", timeoutDuration.Seconds()),
	)

	return g.cmd.Start()
}

func (g *cpuLoadGenerator) Stop() error {
	if g.cmd != nil && g.cmd.Process != nil {
		_ = g.cmd.Process.Kill()
		_, _ = g.cmd.Process.Wait()
	}
	return nil
}

func getCurrentCPUUtilization() (float64, error) {
	percentages, err := cpu.Percent(time.Second, false)
	if err != nil {
		return 0, fmt.Errorf("failed to get CPU utilization: %w", err)
	}
	if len(percentages) == 0 {
		return 0, fmt.Errorf("no CPU utilization data")
	}
	return percentages[0], nil
}
