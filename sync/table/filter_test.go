package table

import (
	"github.com/jwbonnell/pggosync/db"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestFilterTables(t *testing.T) {
	tables := []db.Table{
		{
			Schema: "public",
			Name:   "users",
		},
		{
			Schema: "public",
			Name:   "clients",
		},
		{
			Schema: "auth",
			Name:   "tokens",
		},
		{
			Schema: "auth",
			Name:   "requests",
		},
	}

	excludedTables := []db.Table{
		{
			Schema: "auth",
			Name:   "tokens",
		},
	}

	filtered := FilterTables(tables, excludedTables)

	assert.Len(t, filtered, 3)
	assert.Len(t, tables, 4)
	assert.Len(t, excludedTables, 1)
	assert.Equal(t, "public.users", filtered[0].FullName())
	assert.Equal(t, "auth.requests", filtered[2].FullName())
}
