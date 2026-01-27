package utils

import (
	"strconv"
	"strings"
	"time"
)

func CalculateAverage(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sum := 0.0
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}

func RoundToTwoDecimals(val float64) float64 {
	return float64(int(val*100+0.5)) / 100
}

func ParseLoadLevels(loadLevelsStr string) []int {
	var levels []int
	parts := strings.SplitSeq(loadLevelsStr, ",")

	for part := range parts {
		if level, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
			levels = append(levels, level)
		}
	}

	// Sort levels
	for i := 0; i < len(levels); i++ {
		for j := i + 1; j < len(levels); j++ {
			if levels[i] > levels[j] {
				levels[i], levels[j] = levels[j], levels[i]
			}
		}
	}

	return levels
}

func ParseDuration(durationStr string) time.Duration {
	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		if strings.HasSuffix(durationStr, "s") {
			if seconds, err := strconv.Atoi(strings.TrimSuffix(durationStr, "s")); err == nil {
				return time.Duration(seconds) * time.Second
			}
		}
		return 30 * time.Second
	}
	return duration
}
