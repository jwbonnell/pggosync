package opts

import "github.com/jwbonnell/pggosync/db"

func ProcessExcludedArgs(args []string) ([]db.Table, error) {
	var excludedTables []db.Table
	for _, arg := range args {
		schema, name, err := ParseFullTableName(arg)
		if err != nil {
			return nil, err
		}

		excludedTables = append(excludedTables, db.Table{
			Schema: schema,
			Name:   name,
		})
	}
	return excludedTables, nil
}
