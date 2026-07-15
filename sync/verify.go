package sync

import (
	"context"
	"fmt"

	"github.com/jwbonnell/pggosync/datasource"
)

// TableVerify is the row-count comparison for one table after a sync.
type TableVerify struct {
	Table       string
	Strategy    string
	SourceCount int64
	DestCount   int64
	OK          bool
	Detail      string // human-readable comparison, or the failure reason
	Err         error  // non-nil if a count query failed (OK is then false)
}

// VerifyResult aggregates the per-table verification outcomes.
type VerifyResult struct {
	Tables []TableVerify
}

// OK reports whether every table passed its count check and no count query errored.
func (v VerifyResult) OK() bool {
	for i := range v.Tables {
		if v.Tables[i].Err != nil || !v.Tables[i].OK {
			return false
		}
	}
	return true
}

// Verify re-counts each synced table on the source (with its filter applied) and the destination,
// then compares the two according to the write strategy. It is a row-count sanity check meant to run
// AFTER the sync transaction has committed.
//
// It deliberately does not compare column values: scrub rules make source and destination values
// differ by design, and non-deterministic rules (random_int, random_email, …) would never match, so
// a value/checksum comparison would be meaningless. A count-query failure is recorded on that table
// and fails overall verification. Because it re-queries the live source, concurrent writes there can
// cause a spurious truncate mismatch — it is a best-effort check, not a transactional guarantee.
func Verify(ctx context.Context, tasks []Task, source *datasource.ReaderDataSource, dest *datasource.ReadWriteDatasource) VerifyResult {
	res := VerifyResult{Tables: make([]TableVerify, 0, len(tasks))}
	for i := range tasks {
		t := &tasks[i]
		tv := TableVerify{Table: t.FullName(), Strategy: taskStrategy(t)}

		srcCount, err := source.GetRowCountFiltered(ctx, t.SQLName(), t.Filter)
		if err != nil {
			tv.Err = fmt.Errorf("source count: %w", err)
			res.Tables = append(res.Tables, tv)
			continue
		}
		destCount, err := dest.GetRowCountFiltered(ctx, t.SQLName(), "")
		if err != nil {
			tv.Err = fmt.Errorf("destination count: %w", err)
			res.Tables = append(res.Tables, tv)
			continue
		}

		tv.SourceCount = srcCount
		tv.DestCount = destCount
		tv.OK, tv.Detail = verifyVerdict(t.Truncate && !t.Preserve, srcCount, destCount)
		res.Tables = append(res.Tables, tv)
	}
	return res
}

// verifyVerdict compares source and destination row counts for one table. A truncated table holds
// exactly the synced slice, so the counts must be equal; an upsert/preserve table keeps rows that
// fall outside the slice, so the destination must simply hold at least as many rows as the source.
func verifyVerdict(truncated bool, src, dest int64) (ok bool, detail string) {
	if truncated {
		if dest == src {
			return true, fmt.Sprintf("exact match (%s rows)", FormatCount(src))
		}
		return false, fmt.Sprintf("expected dest == source %s, got %s", FormatCount(src), FormatCount(dest))
	}
	if dest >= src {
		return true, fmt.Sprintf("dest ≥ source (%s ≥ %s)", FormatCount(dest), FormatCount(src))
	}
	return false, fmt.Sprintf("expected dest ≥ source %s, got %s", FormatCount(src), FormatCount(dest))
}
