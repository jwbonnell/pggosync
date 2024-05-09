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
		{"city:444", "public|city|444"},
		{"other.country", "other|country|"},
		{"other.country:453", "other|country|453"},
		{"other.country:453,677", "other|country|453,677"},
	}

	for _, test := range tests {
		schema, table, params, err := ParseTableArg(test.input)
		assert.NoError(t, err)
		assert.Equal(t, test.expected, fmt.Sprintf("%s|%s|%s", schema, table, strings.Join(params, ",")))
	}
}
