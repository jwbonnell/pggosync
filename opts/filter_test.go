package opts

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplyParamToFilter(t *testing.T) {
	var tests = []struct {
		input    string
		filter   string
		expected string
	}{
		{"433", "WHERE country_id = {1}", "WHERE country_id = 433"},
		{"433,123", "WHERE country_id = {1} AND something = {2}", "WHERE country_id = 433 AND something = 123"},
		{"burrito", "WHERE country_id = '{1}'", "WHERE country_id = 'burrito'"},
	}

	for _, test := range tests {
		parts := strings.Split(test.input, ",")
		result := ApplyParamToFilter(parts, test.filter)
		assert.Equal(t, test.expected, result)
	}
}

// TestApplyParamToFilterNoParams guards H7: with no params, placeholders are left intact rather
// than substituted with an empty string.
func TestApplyParamToFilterNoParams(t *testing.T) {
	assert.Equal(t, "WHERE id = {1}", ApplyParamToFilter(nil, "WHERE id = {1}"))
	assert.Equal(t, "WHERE 1=1", ApplyParamToFilter(nil, "WHERE 1=1"))
}

func TestUnresolvedPlaceholders(t *testing.T) {
	assert.Equal(t, []string{"{1}"}, UnresolvedPlaceholders("WHERE id = {1}"))
	assert.Equal(t, []string{"{1}", "{2}"}, UnresolvedPlaceholders("a = {1} AND b = {2}"))
	assert.Empty(t, UnresolvedPlaceholders("WHERE id = 5"))
}
