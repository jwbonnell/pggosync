package sync

import (
	"github.com/jwbonnell/pggosync/db"
	"slices"
)

type Task struct {
	db.Table
	DestColumns      []db.Column
	SourceColumns    []db.Column
	DestPK           []db.PrimaryKey
	SourceSequences  []db.Sequence
	DestSequences    []db.Sequence
	Filter           string
	Preserve         bool
	Truncate         bool
	DeferConstraints bool
}

func (t *Task) GetDestPKs() []string {
	var s []string
	for i := range t.DestPK {
		s = append(s, t.DestPK[i].Column)
	}
	return s
}

func (t *Task) GetSharedColumnNames() []string {
	var s []string
	for i := range t.DestColumns {
		if slices.ContainsFunc(t.SourceColumns, func(column db.Column) bool {
			return column.Name == t.DestColumns[i].Name
		}) {
			s = append(s, t.DestColumns[i].Name)
		}
	}
	return s
}
