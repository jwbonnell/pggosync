package db

type NonDeferrableConstraints struct {
	Schema         string `db:"schema"`
	Table          string `db:"table"`
	ConstraintName string `db:"constraint_name"`
}
