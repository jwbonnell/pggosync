package opts

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"strings"
	"testing"
)

func TestParseGroupArg(t *testing.T) {
	var tests = []struct {
		input    string
		expected string
	}{
		{"base", "base|"},
		{"city:234", "city|234"},
		{"country:453,677", "country|453,677"},
	}

	for _, test := range tests {
		groupID, params, err := ParseGroupArg(test.input)
		assert.NoError(t, err)
		assert.Equal(t, test.expected, fmt.Sprintf("%s|%s", groupID, strings.Join(params, ",")))
	}
}

func TestParseTableArg(t *testing.T) {
	var tests = []struct {
		input    string
		expected string
	}{
		{"city", "public|city|"},
		{"public.city:234", "public|city|234"},
		{"city:WHERE test = 'asdf'", "public|city|WHERE test = 'asdf'"},
		{"other.country", "other|country|"},
		{"other.country:\"WHERE test = 'zxcv'\"", "other|country|WHERE test = 'zxcv'"},
	}

	for _, test := range tests {
		schema, table, filter, err := ParseTableArg(test.input)
		assert.NoError(t, err)
		assert.Equal(t, test.expected, fmt.Sprintf("%s|%s|%s", schema, table, filter))
	}
}

func TestParseFullTableName(t *testing.T) {
	var tests = []struct {
		input    string
		expected string
	}{
		{"city", "public|city"},
		{"public.city", "public|city"},
		{"city", "public|city"},
		{"other.country", "other|country"},
	}

	for _, test := range tests {
		schema, table, err := ParseFullTableName(test.input)
		assert.NoError(t, err)
		assert.Equal(t, test.expected, fmt.Sprintf("%s|%s", schema, table))
	}
}
