package test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eco-digit/energyprofiler/internal/utils"
)

const (
	iterations = 1 << 10
)

func BenchmarkParseLoadLevels(b *testing.B) {
	var loadLevels string
	for i := range iterations {
		loadLevels += fmt.Sprintf("%d,", i)
	}

	loadLevels, _ = strings.CutSuffix(loadLevels, ",")

	for b.Loop() {
		utils.ParseLoadLevels(loadLevels)
	}
}
