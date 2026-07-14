package sync

import (
	"context"
	"testing"

	"github.com/jwbonnell/pggosync/config"
	"github.com/stretchr/testify/assert"
)

// TestTableToTasksHonorsFlags guards the C3 fix: an explicit --table arg must reflect the
// resolver's truncate/preserve/defer-constraints flags instead of always truncating.
func TestTableToTasksHonorsFlags(t *testing.T) {
	cases := []struct {
		name             string
		truncate         bool
		cascade          bool
		preserve         bool
		deferConstraints bool
	}{
		{"preserve", false, false, true, false},
		{"upsert default", false, false, false, false},
		{"truncate", true, false, false, false},
		{"truncate with cascade", true, true, false, false},
		{"truncate with defer", true, false, false, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := NewTaskResolver(nil, nil, nil, tc.truncate, tc.cascade, tc.preserve, tc.deferConstraints, false, nil)
			task, err := tr.tableToTasks("public.users", nil)
			assert.NoError(t, err)
			assert.Equal(t, tc.truncate, task.Truncate, "Truncate")
			assert.Equal(t, tc.cascade, task.Cascade, "Cascade")
			assert.Equal(t, tc.preserve, task.Preserve, "Preserve")
			assert.Equal(t, tc.deferConstraints, task.DeferConstraints, "DeferConstraints")
		})
	}
}

// TestResolveRejectsTruncateWithPreserve: the resolver is the backstop for every caller —
// global truncate+preserve must be rejected before any database work.
func TestResolveRejectsTruncateWithPreserve(t *testing.T) {
	tr := NewTaskResolver(nil, nil, nil, true, false, true, false, false, nil)
	_, err := tr.Resolve(context.Background(), nil, nil)
	assert.ErrorContains(t, err, "truncate and preserve cannot be combined")
}

// TestGroupToTasksRejectsConflictingStrategy: a per-table override combined with a global
// flag (or two overrides) must never leave a task with both strategies set.
func TestGroupToTasksRejectsConflictingStrategy(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }

	cases := []struct {
		name     string
		truncate bool // global flag
		preserve bool // global flag
		entry    config.TableEntry
		wantErr  bool
	}{
		{
			name:     "global truncate + per-table preserve",
			truncate: true,
			entry:    config.TableEntry{Table: "public.users", Preserve: boolPtr(true)},
			wantErr:  true,
		},
		{
			name:     "global preserve + per-table truncate",
			preserve: true,
			entry:    config.TableEntry{Table: "public.users", Truncate: boolPtr(true)},
			wantErr:  true,
		},
		{
			name:    "both set on the entry",
			entry:   config.TableEntry{Table: "public.users", Truncate: boolPtr(true), Preserve: boolPtr(true)},
			wantErr: true,
		},
		{
			name:     "global truncate disabled per-table with preserve",
			truncate: true,
			entry:    config.TableEntry{Table: "public.users", Truncate: boolPtr(false), Preserve: boolPtr(true)},
			wantErr:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			groups := map[string]config.Group{"g": {Tables: []config.TableEntry{tc.entry}}}
			tr := NewTaskResolver(nil, nil, groups, tc.truncate, false, tc.preserve, false, false, nil)
			_, err := tr.groupToTasks("g")
			if tc.wantErr {
				assert.ErrorContains(t, err, "mutually exclusive")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
