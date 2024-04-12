package datasource

//INTEGRATION TESTS - TODO separate from unit tests
//Requires both source and destination databases to be running(use docker compose up -d)

import (
	"context"
	"testing"

	"github.com/jwbonnell/pggosync/db"
	"github.com/stretchr/testify/assert"
)

func TestGetTables(t *testing.T) {
	ctx := context.Background()

	rd, err := getReadDataSource()
	assert.Nil(t, err)

	tables, err := rd.GetTables(ctx)
	assert.Nil(t, err)
	assert.Len(t, tables, 8)

	defer rd.DB.Close(ctx)
}

func TestGetSchemas(t *testing.T) {
	ctx := context.Background()

	rd, err := getReadDataSource()
	assert.Nil(t, err)

	schemas, err := rd.GetSchemas(ctx)
	assert.Nil(t, err)
	assert.Len(t, schemas, 4)
	found := false
	for i := range schemas {
		if schemas[i] == "public" {
			found = true
		}
	}

	assert.True(t, found)

	defer rd.DB.Close(ctx)
}

func TestGetTriggers(t *testing.T) {
	ctx := context.Background()
	table := "dummy"

	rd, err := getReadDataSource()
	assert.Nil(t, err)

	triggers, err := rd.GetTriggers(ctx, table)
	assert.Nil(t, err)
	assert.Len(t, triggers, 1)
	assert.Equal(t, "do_something_trigger", triggers[0].Name)

	defer rd.DB.Close(ctx)
}

func TestStatusCheck(t *testing.T) {
	ctx := context.Background()

	rd, err := getReadDataSource()
	assert.Nil(t, err)

	err = rd.StatusCheck(ctx)
	assert.Nil(t, err)

	defer rd.DB.Close(ctx)
}

func TestGetNonDeferrableConstraints(t *testing.T) {
	ctx := context.Background()

	rd, err := getReadDataSource()
	assert.Nil(t, err)

	ndc, err := rd.GetNonDeferrableConstraints(ctx)
	assert.Nil(t, err)
	assert.Len(t, ndc, 7)

	defer rd.DB.Close(ctx)
}

func getReadDataSource() (*ReaderDataSource, error) {
	s := struct {
		Host     string
		Port     int
		UserName string
		Password string
		Database string
	}{
		Host:     "localhost",
		Port:     5437,
		UserName: "source_user",
		Password: "source_pw",
		Database: "postgres",
	}

	return NewReadDataSource("reader", db.BuildUrl(s.Host, s.Port, s.UserName, s.Password, s.Database))
}

func getReadWriterDataSource() (*ReadWriteDatasource, error) {
	d := struct {
		Host     string
		Port     int
		UserName string
		Password string
		Database string
	}{
		Host:     "localhost",
		Port:     5438,
		UserName: "dest_user",
		Password: "dest_pw",
		Database: "postgres",
	}

	return NewReadWriteDataSource("readwrite", db.BuildUrl(d.Host, d.Port, d.UserName, d.Password, d.Database))
}
