package db

import (
	"database/sql"
	"log"
)

func getConn() (*sql.DB, error) {
	connStr := "user=pqgotest dbname=pqgotest sslmode=verify-full"
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	return db, nil
}

func getTables() (*sql.Rows, error) {
	db, err := getConn()
	if err != nil {
		return nil, err
	}

	return db.Query(`
		SELECT
				table_schema AS schema,
				table_name AS table
			FROM information_schema.tables
 			WHERE	table_type = 'BASE TABLE' 
			  AND table_schema NOT IN ('information_schema', 'pg_catalog')
 			ORDER BY 1, 2
	`)
}

func getSchemas() (*sql.Rows, error) {
	db, err := getConn()
	if err != nil {
		return nil, err
	}

	return db.Query(`SELECT schema_name FROM information_schema.schemata	ORDER BY 1`)
}

func getTriggers(tableName string) (*sql.Rows, error) {
	db, err := getConn()
	if err != nil {
		return nil, err
	}

	return db.Query(`
	SELECT
		tgname AS name,
		tgisinternal AS internal,
		tgenabled != 'D' AS enabled,
		tgconstraint != 0 AS integrity
	FROM
		pg_trigger
	WHERE
		pg_trigger.tgrelid = $1::regclass
	`, tableName)
}
