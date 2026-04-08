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

	const dot = byte(46)

	switch {
	// comparing decimal numbers (a, b < 10)
	case aLen > 2 && a[1] == dot && bLen > 2 && b[1] == dot:
		a = strings.ReplaceAll(a, ".", "")
		b = strings.ReplaceAll(b, ".", "")

		return cmp.Compare(a, b)

	// a is decimal and b is single digit
	case aLen > 2 && a[1] == dot && bLen == 1 && cmp.Compare(string(a[0]), b) == 0:
		return 1

	// b is decimal and a is single digit
	case bLen > 2 && b[1] == dot && aLen == 1 && cmp.Compare(a, string(b[0])) == 0:
		return -1

	// a is decimal and b is multiple digits
	case aLen > 2 && a[1] == dot:
		return cmp.Compare(string(a[0]), b)

	// b is decimal and a is multiple digits
	case bLen > 2 && b[1] == dot:
		return cmp.Compare(a, string(b[0]))

	// a and b are multiple non decimal digits of different length
	case aLen != bLen:
		return cmp.Compare(aLen, bLen)

	// a and b are multiple non decimal digits of same length
	default:
		return cmp.Compare(a, b)
	}
}
