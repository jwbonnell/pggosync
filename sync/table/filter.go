package table

import (
	"slices"

	"github.com/jwbonnell/pggosync/datasource"
	"github.com/jwbonnell/pggosync/db"
)

// FilterTables returns tables with any entry from excluded removed.
func FilterTables(tables []db.Table, excluded []db.Table) []db.Table {
	var out []db.Table
	for _, t := range tables {
		if !slices.ContainsFunc(excluded, t.Equal) {
			out = append(out, t)
		}
	}
	return out
}

// GetSharedTables returns tables present in both source and destination after applying the exclusion list.
func GetSharedTables(source *datasource.ReaderDataSource, destination *datasource.ReadWriteDatasource, excluded []db.Table) []db.Table {
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
	return sharedTables
}
