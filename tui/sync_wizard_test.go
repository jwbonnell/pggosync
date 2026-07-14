package tui

import (
	"strings"
	"testing"

	"github.com/jwbonnell/pggosync/config"
	"github.com/jwbonnell/pggosync/db"
	"github.com/jwbonnell/pggosync/sync"
)

// TestRenderPreviewContent verifies the pure preview renderer produces the expected header,
// per-table strategy labels, row counts, and scrub annotations. This is the part of the async
// preview that carries display logic; the DB orchestration around it is exercised manually.
func TestRenderPreviewContent(t *testing.T) {
	tasks := []sync.Task{
		{
			Table:          db.Table{Schema: "public", Name: "users"},
			Truncate:       true,
			DestRowCount:   1500,
			SourceRowCount: 2000,
		},
		{
			Table:          db.Table{Schema: "public", Name: "accounts"},
			Preserve:       true,
			SourceRowCount: 42,
			ScrubRules:     []config.ScrubRule{{Column: "email", Rule: "redact"}},
		},
		{
			// Upsert (neither truncate nor preserve), unknown source count.
			Table: db.Table{Schema: "public", Name: "orders"},
		},
	}

	src := config.ConnectionConfig{Host: "localhost", Port: 5444, Database: "srcdb"}
	dst := config.ConnectionConfig{Host: "localhost", Port: 5445, Database: "dstdb"}
	opts := syncOptions{concurrency: 4, dryRun: true}

	out := renderPreviewContent(tasks, "src", "dst", src, dst, "default", opts)

	wants := []string{
		"Source:      src  (localhost:5444/srcdb)",
		"Destination: dst  (localhost:5445/dstdb)",
		"Sync config: default",
		"Concurrency: 4",
		"Dry run:     true",
		"Tables (3):",
		"public.users",
		"[truncate]",
		"(1,500 dest rows will be deleted)", // DestRowCount formatted, truncate only
		"~2,000 rows",                       // SourceRowCount formatted
		"public.accounts",
		"[preserve]",
		"~42 rows",
		"[email=redact]", // scrub annotation
		"public.orders",
		"[upsert]",
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("preview content missing %q\n---\n%s", w, out)
		}
	}

	// The upsert task has no source count and no scrub, so it must not carry either annotation.
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "public.orders") {
			if strings.Contains(line, "rows") || strings.Contains(line, "[email") {
				t.Errorf("orders line should have no row/scrub annotation, got %q", line)
			}
		}
	}
}
