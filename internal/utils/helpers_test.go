package utils_test

import (
	"slices"
	"testing"

	"github.com/eco-digit/energyprofiler/internal/utils"
)

var (
	unsortedStrings = []string{"123", "54", "1.7", "1", "0", "6532", "3", "33", "222", "1000", "1.4"}
)

func BenchmarkSortString(b *testing.B) {
	for b.Loop() {
		slices.SortStableFunc(unsortedStrings, utils.SortStrings)
	}
}

func TestSortString(t *testing.T) {
	slices.SortStableFunc(unsortedStrings, utils.SortStrings)

	t.Logf("%+v", unsortedStrings)
}
