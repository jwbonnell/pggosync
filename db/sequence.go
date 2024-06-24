package db

type Sequence struct {
	Schema         string `db:"schema"`
	Table          string `db:"table"`
	Column         string `db:"column"`
	SequenceSchema string `db:"sequence_schema"`
	Sequence       string `db:"sequence"`
}
