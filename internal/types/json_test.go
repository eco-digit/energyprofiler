package types_test

import (
	"encoding/json"
	"testing"

	"github.com/eco-digit/energyprofiler/internal/types"
)

func TestMarshalling(t *testing.T) {
	var br types.BenchmarkResults

	br.PowerProfile.Compute = &types.ResourceProfile{
		Profile: types.Profile{
			"100": 132.34,
			"12":  123.10,
			"28":  132.34,
			"15":  118.4,
			"5":   118.4,
			"14":  111.23,
		},
	}

	b, err := json.MarshalIndent(br, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("%s", b)
}
