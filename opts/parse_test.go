package opts

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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
		{"city:test = 'asdf'", "public|city|test = 'asdf'"},
		{"other.country", "other|country|"},
		{"other.country:\"test = 'zxcv'\"", "other|country|test = 'zxcv'"},
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

func TestParseTableArgWithScrub(t *testing.T) {
	var tests = []struct {
		input      string
		schema     string
		table      string
		filter     string
		scrubCount int
		firstScrub string
	}{
		{"public.users", "public", "users", "", 0, ""},
		{"public.users:active=true", "public", "users", "active=true", 0, ""},
		{"public.users:active=true:email=hash", "public", "users", "active=true", 1, "email=hash"},
		{"public.users::email=hash,ssn=redact", "public", "users", "", 2, "email=hash"},
		{"users::email=hash", "public", "users", "", 1, "email=hash"},
		{"public.users::email=hash,ssn=redact,salary=random_int", "public", "users", "", 3, "email=hash"},
		{"public.users::email=static:redacted@example.com", "public", "users", "", 1, "email=static:redacted@example.com"},
		{"public.users:active=true:name=partial:5", "public", "users", "active=true", 1, "name=partial:5"},
	}

	for _, test := range tests {
		result, err := ParseTableArgWithScrub(test.input)
		assert.NoError(t, err)
		assert.Equal(t, test.schema, result.Schema)
		assert.Equal(t, test.table, result.Table)
		assert.Equal(t, test.filter, result.Filter)
		assert.Equal(t, test.scrubCount, len(result.ScrubRules))
		if test.scrubCount > 0 {
			assert.Equal(t, test.firstScrub, fmt.Sprintf("%s=%s", result.ScrubRules[0].Column, result.ScrubRules[0].Rule))
		}
	}
}

func TestParseTableArgWithScrub_Invalid(t *testing.T) {
	invalid := []string{
		"",
		"public.users::email=unknown",
		"public.users::=hash",
		"public.users::email=",
		"public.users::email=partial:abc",
	}

	for _, input := range invalid {
		_, err := ParseTableArgWithScrub(input)
		assert.Error(t, err, "expected error for input %q", input)
	}
}

func TestSplitTableArg(t *testing.T) {
	var tests = []struct {
		input  string
		expect []string
	}{
		{"public.users", []string{"public.users"}},
		{"public.users:filter", []string{"public.users", "filter"}},
		{"public.users:filter:email=hash", []string{"public.users", "filter", "email=hash"}},
		{`public.users:"country = 'US'":email=hash`, []string{"public.users", `"country = 'US'"`, "email=hash"}},
		{"public.users::email=hash,ssn=redact", []string{"public.users", "", "email=hash,ssn=redact"}},
		{"public.users::email=static:foo", []string{"public.users", "", "email=static:foo"}},
	}

	for _, tt := range tests {
		got := splitTableArg(tt.input)
		assert.Equal(t, tt.expect, got)
	}
}
