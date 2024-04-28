package sync

import (
	"context"
	"fmt"
	"testing"

	"github.com/jwbonnell/pggosync/datasource"
	"github.com/jwbonnell/pggosync/db"
	"github.com/stretchr/testify/assert"
)

func TestSync_UseTruncate(t *testing.T) {
	ctx := context.Background()
	ts, err := getTableSyncStruct()
	fmt.Println(ts.source.Url, ts.destination.Url)
	assert.Nil(t, err)
	t1 := Task{Table: db.Table{Schema: "public", Name: "country"}, Truncate: true, Preserve: false}
	err = ts.Sync(ctx, &t1)
	assert.Nil(t, err)

	rowCount := -1
	row := ts.destination.DB.QueryRow(ctx, "SELECT COUNT(country_id) FROM country")
	err = row.Scan(&rowCount)
	assert.Nil(t, err)
	assert.Equal(t, 100, rowCount)

	t2 := Task{Table: db.Table{Schema: "public", Name: "city"}, Truncate: true, Preserve: false}
	err = ts.Sync(ctx, &t2)
	assert.Nil(t, err)

	rowCount = -1
	row = ts.destination.DB.QueryRow(ctx, "SELECT COUNT(city_id) FROM city")
	err = row.Scan(&rowCount)
	assert.Nil(t, err)

	assert.Equal(t, 30, rowCount)
}

func getTableSyncStruct() (*TableSync, error) {
	var ts *TableSync
	r, err := getReadDataSource()
	if err != nil {
		return ts, err
	}
	rw, err := getReadWriterDataSource()
	if err != nil {
		return ts, err
	}
	return NewTableSync(r, rw), nil
}

func getReadDataSource() (*datasource.ReaderDataSource, error) {
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

	return datasource.NewReadDataSource("reader", db.BuildUrl(s.Host, s.Port, s.UserName, s.Password, s.Database))
}

func getReadWriterDataSource() (*datasource.ReadWriteDatasource, error) {
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

	return datasource.NewReadWriteDataSource("readwrite", db.BuildUrl(d.Host, d.Port, d.UserName, d.Password, d.Database))
}
