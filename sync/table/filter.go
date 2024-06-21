package table

import "github.com/jwbonnell/pggosync/db"

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
