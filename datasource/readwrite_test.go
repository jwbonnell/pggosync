package datasource

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSeedTable(t *testing.T) {
	ctx := context.Background()
	rw, err := getReadWriterDataSource()
	assert.Nil(t, err)
	err = seedDummyTable(ctx, rw.DB, "dummy_seed", 30)
	assert.Nil(t, err)

	rowCount := 0
	rw.DB.QueryRow(ctx, "SELECT COUNT(id) FROM dummy_seed").Scan(&rowCount)
	assert.GreaterOrEqual(t, rowCount, 30)

	rw.DB.Exec(ctx, "TRUNCATE dummy_seed") //cleanup
}

func TestTruncate(t *testing.T) {
	ctx := context.Background()
	rw, err := getReadWriterDataSource()
	assert.Nil(t, err)
	err = seedDummyTable(ctx, rw.DB, "dummy_truncate", 30)
	assert.Nil(t, err)

	rowCount := 0
	rw.DB.QueryRow(ctx, "SELECT COUNT(id) FROM dummy_truncate").Scan(&rowCount)
	assert.Equal(t, 30, rowCount)

	rw.Truncate(ctx, "dummy_truncate")

	rowCount = -1
	rw.DB.QueryRow(ctx, "SELECT COUNT(id) FROM dummy_truncate").Scan(&rowCount)
	assert.Equal(t, 0, rowCount)
}

func TestDeleteAll(t *testing.T) {
	ctx := context.Background()
	rw, err := getReadWriterDataSource()
	assert.Nil(t, err)
	err = seedDummyTable(ctx, rw.DB, "dummy_delete", 30)
	assert.Nil(t, err)

	rowCount := 0
	rw.DB.QueryRow(ctx, "SELECT COUNT(id) FROM dummy_delete").Scan(&rowCount)
	assert.Equal(t, 30, rowCount)

	rw.DeleteAll(ctx, "dummy_delete")

	rowCount = -1
	rw.DB.QueryRow(ctx, "SELECT COUNT(id) FROM dummy_delete").Scan(&rowCount)
	assert.Equal(t, 0, rowCount)
}

func TestCreateTempTable(t *testing.T) {
	ctx := context.Background()
	rw, err := getReadWriterDataSource()
	assert.Nil(t, err)

	err = rw.CreateTempTable(ctx, "country_temp_table", "country")
	assert.Nil(t, err)

	rowCount := 0
	rw.DB.QueryRow(ctx, "SELECT COUNT(id) FROM country_temp_table").Scan(&rowCount)
	assert.GreaterOrEqual(t, rowCount, 0)

	rw.DB.Exec(ctx, "DROP TABLE country_temp_table")
}

func TestSetSequence(t *testing.T) {
	ctx := context.Background()
	rw, err := getReadWriterDataSource()
	assert.Nil(t, err)

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	val := r.Intn(100000-1+1) + 1

	err = rw.SetSequence(ctx, "dummy_seq", val)
	assert.Nil(t, err)

	actual := -1
	rw.DB.Exec(ctx, "SELECT nextval('dummy_seq')")
	rw.DB.QueryRow(ctx, "SELECT last_value FROM dummy_seq").Scan(&actual)
	assert.Equal(t, val+1, actual)
}
