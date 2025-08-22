package results

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/eco-digit/energyprofiler/internal/types"
	"os"
	"strconv"
	"strings"
)

func SaveResults(results *types.BenchmarkResults, filename string) error {
	jsonStr := buildOrderedJSON(results)

	var prettyJSON bytes.Buffer
	err := json.Indent(&prettyJSON, []byte(jsonStr), "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format JSON: %w", err)
	}

	if err := os.WriteFile(filename, prettyJSON.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func buildOrderedJSON(results *types.BenchmarkResults) string {
	var jsonParts []string
	jsonParts = append(jsonParts, `{"power_profile":{`)

	var profileParts []string

	if results.PowerProfile.Compute != nil {
		computeJSON := buildOrderedProfileJSON(results.PowerProfile.Compute)
		profileParts = append(profileParts, fmt.Sprintf(`"compute":%s`, computeJSON))
	}

	if results.PowerProfile.Memorize != nil {
		memorizeJSON := buildOrderedProfileJSON(results.PowerProfile.Memorize)
		profileParts = append(profileParts, fmt.Sprintf(`"memorize":%s`, memorizeJSON))
	}

	if results.PowerProfile.Store != nil {
		storeJSON := buildOrderedProfileJSON(results.PowerProfile.Store)
		profileParts = append(profileParts, fmt.Sprintf(`"store":%s`, storeJSON))
	}

	if results.PowerProfile.Transfer != nil {
		transferJSON := buildOrderedProfileJSON(results.PowerProfile.Transfer)
		profileParts = append(profileParts, fmt.Sprintf(`"transfer":%s`, transferJSON))
	}

	jsonParts = append(jsonParts, strings.Join(profileParts, ","))
	jsonParts = append(jsonParts, fmt.Sprintf(`,"total_avg":%.2f}}`, results.PowerProfile.TotalAvg))

	return strings.Join(jsonParts, "")
}

func buildOrderedProfileJSON(rp *types.ResourceProfile) string {
	keys := make([]string, 0, len(rp.Profile))
	for k := range rp.Profile {
		keys = append(keys, k)
	}

	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			iVal, _ := strconv.ParseFloat(keys[i], 64)
			jVal, _ := strconv.ParseFloat(keys[j], 64)
			if iVal > jVal {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}

	var profileParts []string
	for _, key := range keys {
		value := rp.Profile[key]
		profileParts = append(profileParts, fmt.Sprintf(`"%s":%.2f`, key, value))
	}

	profileJSON := "{" + strings.Join(profileParts, ",") + "}"
	return fmt.Sprintf(`{"profile":%s,"avg":%.2f}`, profileJSON, rp.Avg)
}
