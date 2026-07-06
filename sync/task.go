package sync

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jwbonnell/pggosync/config"
	"github.com/jwbonnell/pggosync/db"
	"github.com/jwbonnell/pggosync/sync/data"
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
	SourceRowCount   int64 // set by TUI preview via GetRowCountFiltered; 0 means unknown
	ScrubRules       []config.ScrubRule
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

// GetSelectColumns builds the SELECT column list for the COPY TO query, applying scrub expressions
// where rules are defined. The expressions execute on the source database during prefetch, so raw
// values never leave the source. Columns without scrub rules are returned as-is; scrubbed columns
// are replaced with their SQL expression aliased to the original column name so the destination
// COPY works unchanged. Expressions are built from the shared column name (already quoted for
// reserved words), not the raw config string.
func (t *Task) GetSelectColumns() []string {
	shared := t.GetSharedColumnNames()
	if len(t.ScrubRules) == 0 {
		return shared
	}

	ruleMap := make(map[string]string, len(t.ScrubRules))
	for _, r := range t.ScrubRules {
		ruleMap[r.Column] = r.Rule
	}

	result := make([]string, len(shared))
	for i, col := range shared {
		bare := strings.Trim(col, `"`)
		result[i] = col
		if rule, ok := ruleMap[bare]; ok {
			if expr := data.SQLExpression(rule, col); expr != "" {
				result[i] = fmt.Sprintf("%s AS %s", expr, col)
			}
		}
	}
	return result
}
