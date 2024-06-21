package table

import (
	"github.com/jwbonnell/pggosync/datasource"
	"github.com/jwbonnell/pggosync/db"
)

func FilterTables(tables []db.Table, excluded []db.Table) []db.Table {
	var out []db.Table
	for _, table := range tables {
		for _, exclude := range excluded {
			if !table.Equal(exclude) {
				out = append(out, table)
			}
		}
	}
	return out
}

func GetSharedTables(source datasource.ReaderDataSource, destination datasource.ReadWriteDatasource, excluded []db.Table) ([]db.Table, error) {
	sourceTables := FilterTables(source.Tables, excluded)
	destinationTables := FilterTables(destination.Tables, excluded)

	var sharedTables []db.Table
	for i := range destinationTables {
		for j := range sourceTables {
			if destinationTables[i].Equal(sourceTables[j]) {
				sharedTables = append(sharedTables, sourceTables[j])
			}
		}
	}
	return sharedTables, nil
}
