package sync

import (
	"slices"
	"strings"

	"github.com/jwbonnell/pggosync/db"
)

// systemColumns are PostgreSQL internal columns that must never be included in COPY.
var systemColumns = []string{"ctid", "xmin", "xmax", "cmin", "cmax", "tableoid", "oid"}

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
	DestRowCount     int64
}

// GetDestPKs extracts just the column names from the destination PK definition.
func (t *Task) GetDestPKs() []string {
	var s []string
	for i := range t.DestPK {
		s = append(s, t.DestPK[i].Column)
	}
	return s
}

// ScrubColumns filters out PostgreSQL system columns that must not appear in COPY.
// Reserved SQL keywords are already quoted by GetColumns; this is a safety net for
// system columns that should never be copied regardless of their quoting.
func (t *Task) ScrubColumns(cols []string) []string {
	result := make([]string, 0, len(cols))
	for _, c := range cols {
		bare := strings.Trim(c, `"`)
		if !slices.Contains(systemColumns, bare) {
			result = append(result, c)
		}
	}
	return result
}

// GetSharedColumnNames returns columns present in both source and destination; used to build the COPY column list.
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
