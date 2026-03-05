package types

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/eco-digit/energyprofiler/internal/utils"
)

func (p Profile) MarshalJSON() ([]byte, error) {
	var keys []string

	keyIter := maps.Keys(p)
	for key := range keyIter {
		keys = append(keys, key)
	}

	slices.SortStableFunc(keys, utils.SortStrings)

	var lines []string

	for _, key := range keys {
		lines = append(lines, fmt.Sprintf(`"%s": %f`, key, p[key]))
	}

	return []byte("{" + strings.Join(lines, ",") + "}"), nil
}
