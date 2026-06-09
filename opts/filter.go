package opts

import (
	"fmt"
	"strings"
)

// ApplyParamToFilter substitutes {1}, {2}, … placeholders in a WHERE clause with the corresponding positional params.
func ApplyParamToFilter(params []string, filter string) string {
	if len(params) == 0 {
		return filter
	}

	for i := range params {
		filter = strings.ReplaceAll(filter, fmt.Sprintf("{%d}", i+1), params[i])
	}

	return filter
}
