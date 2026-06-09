package table

import (
	"testing"

	"github.com/jwbonnell/pggosync/db"
	"github.com/stretchr/testify/assert"
)

var allTables = []db.Table{
	{Schema: "public", Name: "users"},
	{Schema: "public", Name: "clients"},
	{Schema: "auth", Name: "tokens"},
	{Schema: "auth", Name: "requests"},
}

func TestFilterTables_OneExclusion(t *testing.T) {
	excluded := []db.Table{{Schema: "auth", Name: "tokens"}}
	filtered := FilterTables(allTables, excluded)

	assert.Len(t, filtered, 3)
	assert.Equal(t, "public.users", filtered[0].FullName())
	assert.Equal(t, "auth.requests", filtered[2].FullName())
}

func TestFilterTables_NoExclusions(t *testing.T) {
	filtered := FilterTables(allTables, nil)
	assert.Len(t, filtered, 4)
}

func TestFilterTables_MultipleExclusions(t *testing.T) {
	excluded := []db.Table{
		{Schema: "auth", Name: "tokens"},
		{Schema: "auth", Name: "requests"},
	}
	filtered := FilterTables(allTables, excluded)

	assert.Len(t, filtered, 2)
	assert.Equal(t, "public.users", filtered[0].FullName())
	assert.Equal(t, "public.clients", filtered[1].FullName())
}
