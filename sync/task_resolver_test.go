package sync

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestTableToTasksHonorsFlags guards the C3 fix: an explicit --table arg must reflect the
// resolver's truncate/preserve/defer-constraints flags instead of always truncating.
func TestTableToTasksHonorsFlags(t *testing.T) {
	cases := []struct {
		name             string
		truncate         bool
		preserve         bool
		deferConstraints bool
	}{
		{"preserve", false, true, false},
		{"upsert default", false, false, false},
		{"truncate", true, false, false},
		{"truncate with defer", true, false, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := NewTaskResolver(nil, nil, nil, tc.truncate, tc.preserve, tc.deferConstraints, false, nil)
			task, err := tr.tableToTasks("public.users", nil)
			assert.NoError(t, err)
			assert.Equal(t, tc.truncate, task.Truncate, "Truncate")
			assert.Equal(t, tc.preserve, task.Preserve, "Preserve")
			assert.Equal(t, tc.deferConstraints, task.DeferConstraints, "DeferConstraints")
		})
	}
}
