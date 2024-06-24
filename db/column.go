package db

type Column struct {
	Schema string `db:"schema"`
	Table  string `db:"table"`
	Name   string `db:"column"`
	Type   string `db:"type"`
}
