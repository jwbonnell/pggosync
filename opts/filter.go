package opts

import (
	"fmt"
	"strings"
)

func ApplyParamToFilter(params []string, filter string) string {
	if len(params) == 0 {
		return filter
	}

	for i := range params {
		filter = strings.ReplaceAll(filter, fmt.Sprintf("{%d}", i+1), params[i])
	}

	return filter
}
