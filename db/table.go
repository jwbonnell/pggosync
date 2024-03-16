package db

import "fmt"

type Table struct {
	Schema string `db:"schema"`
	Name   string `db:"name"`
}

func (t *Table) FullName() string {
	return fmt.Sprintf("%s.%s", t.Schema, t.Name)
}

func (t *Table) Equal(other Table) bool {
	return t.Schema == other.Schema && t.Name == other.Name
}
