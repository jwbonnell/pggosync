package opts

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestProcessExcludedArgs(t *testing.T) {
	tables, err := ProcessExcludedArgs([]string{
		"users",
		"clients",
		"auth.tokens",
	})

	require.Nil(t, err, "ProcessExcludedArgs should not return an error")
	assert.Len(t, tables, 3, "ProcessExcludedArgs should return 3 tables")
	assert.Equal(t, "public.users", tables[0].FullName(), "ProcessExcludedArgs should properly handle excluded args with no schema")
	assert.Equal(t, "public.clients", tables[1].FullName(), "ProcessExcludedArgs should properly handle excluded args with no schema")
	assert.Equal(t, "auth.tokens", tables[2].FullName(), "ProcessExcludedArgs should properly handle excluded args with a schema")
}

func TestProcessExcludedArgs_Invalid(t *testing.T) {
	_, err := ProcessExcludedArgs([]string{
		"users",
		"clients",
		"auth.tokens.something",
	})

	require.NotNil(t, err, "ProcessExcludedArgs should return an error when an invalid schema/table is passed in")
}
