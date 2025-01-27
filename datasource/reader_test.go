package datasource

//INTEGRATION TESTS - TODO separate from unit tests
//Requires both source and destination databases to be running(use docker compose up -d)

import (
	"context"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetTables(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode...skipping integration test")
	}
	ctx := context.Background()

	rd, err := getReadDataSource()
	assert.Nil(t, err)

	tables, err := rd.GetTables(ctx)
	assert.Nil(t, err)
	assert.Len(t, tables, 12)

	defer rd.DB.Close(ctx)
}

func TestGetSchemas(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode...skipping integration test")
	}
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

func TestGetUserTriggers(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode...skipping integration test")
	}
	ctx := context.Background()

	rd, err := getReadDataSource()
	assert.Nil(t, err)

	triggers, err := rd.GetUserTriggers(ctx)
	assert.Nil(t, err)
	assert.Len(t, triggers, 1)
	assert.Equal(t, "do_something_trigger", triggers[0].Name)

	defer rd.DB.Close(ctx)
}

func TestStatusCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode...skipping integration test")
	}
	ctx := context.Background()

	rd, err := getReadDataSource()
	assert.Nil(t, err)

	err = rd.StatusCheck(ctx)
	assert.Nil(t, err)

	defer rd.DB.Close(ctx)
}

func TestGetNonDeferrableConstraints(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode...skipping integration test")
	}
	ctx := context.Background()
	rd, err := getReadDataSource()
	assert.Nil(t, err)

	ndc, err := rd.GetNonDeferrableConstraints(ctx)
	assert.Nil(t, err)
	assert.Len(t, ndc, 7)

	defer rd.DB.Close(ctx)
}

func getReadDataSource() (*ReaderDataSource, error) {
	u := url.URL{
		Scheme: "postgres",
		Host:   "localhost:5432",
		User:   url.UserPassword("source_user", "source_pw"),
		Path:   "postgres",
	}

	return NewReadDataSource("reader", u)
}

func getReadWriterDataSource() (*ReadWriteDatasource, error) {
	u := url.URL{
		Scheme: "postgres",
		Host:   "localhost:5433",
		User:   url.UserPassword("dest_user", "dest_pw"),
		Path:   "postgres",
	}

	return NewReadWriteDataSource("readwrite", u)
}
