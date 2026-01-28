package results

import (
	"fmt"
	"testing"

	"github.com/eco-digit/energyprofiler/internal/types"
	"github.com/eco-digit/energyprofiler/internal/utils"
)

func BenchmarkSortAlgo(b *testing.B) {
	a := NewAggregator()

	a.AddCycle([]types.CycleData{
		{
			LoadLevel:       40,
			MeasuredUtil:    40.1,
			PowerValues:     []float64{32.45435, 23.1212, 13.22222},
			UtilizationVals: []float64{40.1, 40.2, 40.0},
		},
	})
	a.AddCycle([]types.CycleData{
		{
			LoadLevel:       30,
			MeasuredUtil:    30.1,
			PowerValues:     []float64{32.45435, 23.1212, 13.22222},
			UtilizationVals: []float64{30.1, 30.2, 30.0},
		},
	})
	a.AddCycle([]types.CycleData{
		{
			LoadLevel:       10,
			MeasuredUtil:    10.1,
			PowerValues:     []float64{32.45435, 23.1212, 13.22222},
			UtilizationVals: []float64{10.1, 10.2, 10.0},
		},
	})
	a.AddCycle([]types.CycleData{
		{
			LoadLevel:       20,
			MeasuredUtil:    20.1,
			PowerValues:     []float64{32.45435, 23.1212, 13.22222},
			UtilizationVals: []float64{20.1, 20.2, 19.9},
		},
	})
	a.AddCycle([]types.CycleData{
		{
			LoadLevel:       5,
			MeasuredUtil:    5.1,
			PowerValues:     []float64{32.45435, 23.1212, 13.22222},
			UtilizationVals: []float64{5.1, 5.2, 5.0},
		},
	})

	utilizationLevels := make(map[string][]float64)

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

	for b.Loop() {
		a.getSortedKeys(utilizationLevels)
	}
}
