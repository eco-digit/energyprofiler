package utils

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"
)

func CalculateAverage(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	var sum float64
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
	slices.Sort(levels)

	return levels
}

func ParseDuration(durationStr string) time.Duration {
	duration, err := time.ParseDuration(durationStr)
	if err == nil {
		return duration
	}

	durationStrCut, found := strings.CutSuffix(durationStr, "s")
	seconds, err := strconv.Atoi(durationStrCut)
	if err != nil || !found {
		return 30 * time.Second
	}

	return time.Duration(seconds) * time.Second
}

func SortStrings(a, b string) int {
	aLen, bLen := len(a), len(b)

	if aLen != bLen {
		return cmp.Compare(aLen, bLen)
	}

	return cmp.Compare(a, b)
}
