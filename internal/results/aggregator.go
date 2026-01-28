package results

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/eco-digit/energyprofiler/internal/types"
	"github.com/eco-digit/energyprofiler/internal/utils"
)

type Aggregator struct {
	cycles [][]types.CycleData
}

func NewAggregator() *Aggregator {
	return &Aggregator{
		cycles: make([][]types.CycleData, 0),
	}
}

func (a *Aggregator) AddCycle(cycle []types.CycleData) {
	a.cycles = append(a.cycles, cycle)
}

// GetAggregatedProfile() returns measurement data after all cycles complete
func (a *Aggregator) GetAggregatedProfile() *types.ResourceProfile {
	if len(a.cycles) == 0 {
		return &types.ResourceProfile{
			Profile: make(map[string]float64),
			Avg:     0,
		}
	}

	utilizationLevels := make(map[string][]float64)

	// Group power values by utilization level across cycles
	for _, cycle := range a.cycles {
		for _, data := range cycle {
			var utilKey string
			if data.MeasuredUtil < 5.0 {
				utilKey = fmt.Sprintf("%.1f", data.MeasuredUtil)
			} else {
				utilKey = fmt.Sprintf("%d", int(data.MeasuredUtil+0.5))
			}

			if len(data.PowerValues) > 0 {
				cycleAvg := utils.CalculateAverage(data.PowerValues)
				utilizationLevels[utilKey] = append(utilizationLevels[utilKey], cycleAvg)
			}
		}
	}

	// Average across cycles for each utilization level
	profile := make(map[string]float64)
	var allAvgs []float64

	keys := a.getSortedKeys(utilizationLevels)

	for _, key := range keys {
		powerVals := utilizationLevels[key]
		if len(powerVals) > 0 {
			avgPower := utils.CalculateAverage(powerVals)
			profile[key] = utils.RoundToTwoDecimals(avgPower)
			allAvgs = append(allAvgs, avgPower)
		}
	}

	return &types.ResourceProfile{
		Profile: profile,
		Avg:     utils.RoundToTwoDecimals(utils.CalculateAverage(allAvgs)),
	}
}

func (a *Aggregator) getSortedKeys(utilizationLevels map[string][]float64) []string {
	keys := make([]string, 0, len(utilizationLevels))
	for k := range utilizationLevels {
		keys = append(keys, k)
	}

	slices.SortStableFunc(keys, func(a, b string) int {
		aLen, bLen := len(a), len(b)

		if aLen != bLen {
			return cmp.Compare(aLen, bLen)
		}

		return cmp.Compare(a, b)
	})

	return keys
}

func (a *Aggregator) GetAggregatedProfileWithPercentages() *types.ResourceProfile {
	if len(a.cycles) == 0 {
		return &types.ResourceProfile{
			Profile: make(map[string]float64),
			Avg:     0,
		}
	}

	// Process in percentage order to ensure consistent output
	// For bandwidth benchmarks, we use fixed percentage levels: 0, 20, 40, 60, 80, 100
	percentageLevels := []int{0, 20, 40, 60, 80, 100}
	profile := make(map[string]float64)
	var allAvgs []float64

	// Process each percentage level in order
	for i, targetPercentage := range percentageLevels {
		configIndex := i

		for _, cycle := range a.cycles {
			if configIndex < len(cycle) {
				data := cycle[configIndex]
				if len(data.PowerValues) > 0 {
					cycleAvg := utils.CalculateAverage(data.PowerValues)
					key := fmt.Sprintf("%d", targetPercentage)
					profile[key] = utils.RoundToTwoDecimals(cycleAvg)
					allAvgs = append(allAvgs, cycleAvg)
					break // Only process first cycle for each config
				}
			}
		}
	}

	return &types.ResourceProfile{
		Profile: profile,
		Avg:     utils.RoundToTwoDecimals(utils.CalculateAverage(allAvgs)),
	}
}
