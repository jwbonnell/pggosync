package sync

import (
	"testing"

	"github.com/jwbonnell/pggosync/config"
	"github.com/jwbonnell/pggosync/db"
	"github.com/stretchr/testify/assert"
)

func TestGetSelectColumns(t *testing.T) {
	cols := []db.Column{
		{Schema: "public", Table: "users", Name: "id"},
		{Schema: "public", Table: "users", Name: "email"},
		{Schema: "public", Table: "users", Name: `"order"`},
	}
	task := Task{
		Table:         db.Table{Schema: "public", Name: "users"},
		SourceColumns: cols,
		DestColumns:   cols,
	}

	// No scrub rules: columns pass through untouched.
	assert.Equal(t, []string{"id", "email", `"order"`}, task.GetSelectColumns())

	// Scrub rules reference the bare column name; expressions must use the quoted name.
	task.ScrubRules = []config.ScrubRule{
		{Column: "email", Rule: "hash"},
		{Column: "order", Rule: "redact"},
	}
	assert.Equal(t, []string{
		"id",
		"MD5(email::text) AS email",
		`'***REDACTED***' AS "order"`,
	}, task.GetSelectColumns())

	// Unknown rule leaves the column unscrubbed rather than emitting broken SQL.
	task.ScrubRules = []config.ScrubRule{{Column: "email", Rule: "bogus"}}
	assert.Equal(t, []string{"id", "email", `"order"`}, task.GetSelectColumns())
}

func TestValidateScrubColumns(t *testing.T) {
	cols := []db.Column{
		{Schema: "public", Table: "users", Name: "id"},
		{Schema: "public", Table: "users", Name: "email"},
	}
	task := Task{
		Table:         db.Table{Schema: "public", Name: "users"},
		SourceColumns: cols,
		DestColumns:   cols,
		DestPK:        []db.PrimaryKey{{Schema: "public", Table: "users", Column: "id"}},
	}

	task.ScrubRules = []config.ScrubRule{{Column: "email", Rule: "hash"}}
	assert.NoError(t, validateScrubColumns([]Task{task}))

	task.ScrubRules = []config.ScrubRule{{Column: "emial", Rule: "hash"}}
	assert.ErrorContains(t, validateScrubColumns([]Task{task}), "does not exist")

	task.ScrubRules = []config.ScrubRule{{Column: "id", Rule: "random_int"}}
	assert.ErrorContains(t, validateScrubColumns([]Task{task}), "primary key")
}
