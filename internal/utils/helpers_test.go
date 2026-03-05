package utils_test

import (
	"slices"
	"testing"

	"github.com/eco-digit/energyprofiler/internal/utils"
)

func BenchmarkSortString(b *testing.B) {
	// setup
	unsortedStrings := []string{"123", "54", "0", "6532", "3", "33", "222", "1000"}

	for b.Loop() {
		slices.SortStableFunc(unsortedStrings, utils.SortStrings)
	}

	// end
}
