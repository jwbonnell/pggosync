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
