package opts

import (
	"fmt"
	"regexp"
	"strings"
)

// placeholderRe matches positional filter placeholders like {1}, {2}, …
var placeholderRe = regexp.MustCompile(`\{\d+\}`)

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

// UnresolvedPlaceholders returns any {N} placeholders still present in a filter after substitution,
// so callers can reject a group invoked without the params its filters require instead of running
// syntactically broken SQL.
func UnresolvedPlaceholders(filter string) []string {
	return placeholderRe.FindAllString(filter, -1)
}
