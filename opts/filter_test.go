package opts

import (
	"github.com/stretchr/testify/assert"
	"strings"
	"testing"
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
