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
	seed := 123456789
	expected := "pggosync_77885"
	assert.Equal(t, expected, GenTempTableName(int64(seed)), "the generated temp table name should match expected")
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

func TestSequenceStruct_NotEqual(t *testing.T) {
	s1 := Sequence{Schema: "cyril", Name: "figgis", Column: "column1"}
	s2 := Sequence{Schema: "malory", Name: "archer", Column: "column1"}

	assert.False(t, s1.Equal(s2), "the sequences should not be equal")
}

func TestSequenceStruct_Equal(t *testing.T) {
	s1 := Sequence{Schema: "cyril", Name: "figgis", Column: "column1"}
	s2 := Sequence{Schema: "cyril", Name: "figgis", Column: "column1"}

	assert.True(t, s1.Equal(s2), "the sequences should be equal")
}
