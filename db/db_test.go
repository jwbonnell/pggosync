package db

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildUrl(t *testing.T) {
	host := "somehost"
	port := 1234
	username := "solid"
	password := "snake"
	database := "database1"
	expected := fmt.Sprintf("postgres://%s:%s@%s:%d/%s", username, password, host, port, database)

	assert.Equal(t, expected, BuildUrl(host, port, username, password, database), "The build URLs should match")
}

func TestGenTempTableName(t *testing.T) {
	seed := int64(123456789)
	assert.Equal(t, "pggosync_users_123456789", GenTempTableName(seed, "users"))
	assert.NotEmpty(t, GenTempTableName(0, "users")) // 0 seed uses current time
}

func TestTableStruct_FullName(t *testing.T) {
	table := Table{
		Schema: "archer",
		Name:   "sterling_malory_archer",
	}

	assert.Equal(t, "archer.sterling_malory_archer", table.FullName(), "the FullName should match the expected value")
}

// TestTableStruct_SQLName guards H1: schema/table identifiers are quoted for SQL, so mixed-case,
// reserved-word, and special-character names are safe to interpolate.
func TestTableStruct_SQLName(t *testing.T) {
	assert.Equal(t, `"public"."users"`, (&Table{Schema: "public", Name: "users"}).SQLName())
	assert.Equal(t, `"public"."Order"`, (&Table{Schema: "public", Name: "Order"}).SQLName())
	assert.Equal(t, `"public"."my table"`, (&Table{Schema: "public", Name: "my table"}).SQLName())
	// Embedded double-quotes are escaped by doubling.
	assert.Equal(t, `"public"."a""b"`, (&Table{Schema: "public", Name: `a"b`}).SQLName())
}

func TestQuoteIdentifier(t *testing.T) {
	assert.Equal(t, `"seq"`, QuoteIdentifier("seq"))
	assert.Equal(t, `"Weird Name"`, QuoteIdentifier("Weird Name"))
}

func TestTableStruct_NotEqual(t *testing.T) {
	t1 := Table{Schema: "lana", Name: "kane"}
	t2 := Table{Schema: "pam", Name: "poovey"}

	assert.False(t, t1.Equal(t2), "the tables should not be equal")
}

func TestTableStruct_Equal(t *testing.T) {
	t1 := Table{Schema: "lana", Name: "kane"}
	t2 := Table{Schema: "lana", Name: "kane"}

	assert.True(t, t1.Equal(t2), "the tables should be equal")
}
