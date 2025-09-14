package core

import (
	"context"
	"fmt"
	"github.com/eco-digit/energyprofiler/internal/types"
	"log"
	"time"

	"github.com/eco-digit/energyprofiler/internal/power"
	"github.com/eco-digit/energyprofiler/internal/results"
	"github.com/eco-digit/energyprofiler/internal/utils"
)

type BaseBenchmark struct {
	Config       *types.Config
	PowerReader  PowerReader
	Aggregator   *results.Aggregator
	ResourceType string
}

func NewBaseBenchmark(config *types.Config, resourceType string) *BaseBenchmark {
	return &BaseBenchmark{
		Config:       config,
		PowerReader:  power.NewBMCReader(config.UseSystemPower, config.Verbose),
		Aggregator:   results.NewAggregator(),
		ResourceType: resourceType,
	}
}

func (b *BaseBenchmark) RunBenchmarkCycles(
	ctx context.Context,
	loadGen LoadGenerator,
	utilizationGetter UtilizationGetter,
) (*types.ResourceProfile, error) {

	log.Printf("Starting %s core with %d cycles", b.ResourceType, b.Config.Cycles)

	for cycle := 1; cycle <= b.Config.Cycles; cycle++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		log.Printf("Running %s core cycle %d/%d", b.ResourceType, cycle, b.Config.Cycles)

		cycleData, err := b.runSingleCycle(ctx, loadGen, utilizationGetter, cycle == 1)
		if err != nil {
			return nil, fmt.Errorf("cycle %d failed: %w", cycle, err)
		}

		b.Aggregator.AddCycle(cycleData)

		if cycle < b.Config.Cycles {
			log.Printf("Cooldown period...")
			time.Sleep(b.Config.CooldownDuration)
		}
	}

	return b.Aggregator.GetAggregatedProfile(), nil
}

// runSingleCycle() test for each cycle multiple load levels
func (b *BaseBenchmark) runSingleCycle(
	ctx context.Context,
	loadGen LoadGenerator,
	utilizationGetter UtilizationGetter,
	includeIdle bool,
) ([]types.CycleData, error) {

	var cycleData []types.CycleData

	if includeIdle {
		idleData, err := b.measureIdle(ctx, utilizationGetter)
		if err != nil {
			return nil, fmt.Errorf("idle measurement failed: %w", err)
		}
		cycleData = append(cycleData, idleData)
	}

	//Test each load level
	for _, loadLevel := range b.Config.LoadLevels {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		log.Printf("  Testing %s load level: %d%%", b.ResourceType, loadLevel)

		if err := loadGen.Start(loadLevel); err != nil {
			return nil, fmt.Errorf("failed to start load at %d%%: %w", loadLevel, err)
		}

		time.Sleep(b.Config.StabilizeDuration)

		data, err := b.collectMeasurements(ctx, utilizationGetter)
		if err != nil {
			// Stop load
			loadGen.Stop()
			return nil, fmt.Errorf("measurement collection failed: %w", err)
		}

		data.LoadLevel = loadLevel

		if err := loadGen.Stop(); err != nil {
			log.Printf("Warning: failed to stop load generator: %v", err)
		}

		cycleData = append(cycleData, data)

		log.Printf("  Target: %d%%, Measured: %.1f%%, Avg Power: %.2f W",
			loadLevel, data.MeasuredUtil, utils.CalculateAverage(data.PowerValues))

		time.Sleep(5 * time.Second)
	}

	return cycleData, nil
}

func (b *BaseBenchmark) measureIdle(ctx context.Context, utilizationGetter UtilizationGetter) (types.CycleData, error) {
	log.Printf("  Measuring idle baseline (0%% load)")

	time.Sleep(10 * time.Second)

	data, err := b.collectMeasurements(ctx, utilizationGetter)
	if err != nil {
		return types.CycleData{}, err
	}

	data.LoadLevel = 0
	log.Printf("  Idle baseline: %.1f%% util, Avg Power: %.2f W",
		data.MeasuredUtil, utils.CalculateAverage(data.PowerValues))

	return data, nil
}

// collectMeasurements() collect power readings for each DBR
func (b *BaseBenchmark) collectMeasurements(ctx context.Context, utilizationGetter UtilizationGetter) (types.CycleData, error) {
	var powerValues []float64
	var utilizationVals []float64

	measurementEnd := time.Now().Add(b.Config.MeasurementDuration)

	// Collect samples for set measurement duration
	for time.Now().Before(measurementEnd) {
		select {
		case <-ctx.Done():
			return types.CycleData{}, ctx.Err()
		default:
		}
		// Read power from source (BMC)
		power, err := b.PowerReader.ReadPower(b.ResourceType)
		if err != nil {
			log.Printf("Warning: power read failed: %v", err)
		} else {
			powerValues = append(powerValues, power)
		}

		// Read utilization (CPU%, Memory%, etc.)
		util, err := utilizationGetter()
		if err != nil {
			log.Printf("Warning: utilization read failed: %v", err)
		} else {
			utilizationVals = append(utilizationVals, util)
		}

		if b.Config.Verbose {
			log.Printf("    %s Power: %.2f W, Utilization: %.1f%%",
				b.ResourceType, power, util)
		}

		time.Sleep(b.Config.MeasurementInterval)
	}

	return types.CycleData{
		MeasuredUtil:    utils.CalculateAverage(utilizationVals),
		PowerValues:     powerValues,
		UtilizationVals: utilizationVals,
	}, nil
}
