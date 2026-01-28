package results

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/eco-digit/energyprofiler/internal/types"
)

func SaveResults(results *types.BenchmarkResults, filename string) error {
	b, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format JSON: %w", err)
	}

	if err := os.WriteFile(filename, b, os.ModePerm); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}
