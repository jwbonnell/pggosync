package db

type PrimaryKey struct {
	Schema     string `db:"schema"`
	Table      string `db:"table"`
	Column     string `db:"column"`
	FormatType string `db:"format_type"`
	AttNum     string `db:"attnum"`
	IndKey     string `db:"indkey"`
}
