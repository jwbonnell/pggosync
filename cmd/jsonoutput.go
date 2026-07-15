package cmd

import (
	"encoding/json"
	"io"
	"time"

	"github.com/jwbonnell/pggosync/opts"
	"github.com/jwbonnell/pggosync/sync"
)

type jsonTableResult struct {
	Table    string `json:"table"`
	Strategy string `json:"strategy"`
	Rows     int64  `json:"rows"`
	Error    string `json:"error,omitempty"`
}

type jsonVerifyResult struct {
	Table       string `json:"table"`
	Strategy    string `json:"strategy"`
	SourceCount int64  `json:"source_count"`
	DestCount   int64  `json:"dest_count"`
	OK          bool   `json:"ok"`
	Detail      string `json:"detail,omitempty"`
	Error       string `json:"error,omitempty"`
}

type jsonSummary struct {
	Success   bool               `json:"success"`
	DryRun    bool               `json:"dry_run"`
	Source    string             `json:"source"`
	Dest      string             `json:"dest"`
	ElapsedMs int64              `json:"elapsed_ms"`
	Tables    []jsonTableResult  `json:"tables"`
	Verified  bool               `json:"verified"`
	VerifyOK  bool               `json:"verify_ok"`
	Verify    []jsonVerifyResult `json:"verify,omitempty"`
	Error     string             `json:"error,omitempty"`
}

// printJSONSummary writes a machine-readable summary of the sync (and optional verification) to w.
// success is true only when the sync committed and, if verification ran, every table matched. The
// summary is emitted even on failure (with error populated) so callers always get a parseable result.
func printJSONSummary(w io.Writer, source, dest string, args opts.CLIArgs, res sync.SyncResult, syncErr error, vr sync.VerifyResult, verified bool, elapsed time.Duration) error {
	s := jsonSummary{
		DryRun:    args.DryRun,
		Source:    source,
		Dest:      dest,
		ElapsedMs: elapsed.Milliseconds(),
		Verified:  verified,
	}

	s.Tables = make([]jsonTableResult, 0, len(res.Tables))
	for _, tr := range res.Tables {
		jt := jsonTableResult{Table: tr.Table, Strategy: tr.Strategy, Rows: tr.Rows}
		if tr.Err != nil {
			jt.Error = tr.Err.Error()
		}
		s.Tables = append(s.Tables, jt)
	}

	if verified {
		s.VerifyOK = vr.OK()
		s.Verify = make([]jsonVerifyResult, 0, len(vr.Tables))
		for _, tv := range vr.Tables {
			jv := jsonVerifyResult{
				Table:       tv.Table,
				Strategy:    tv.Strategy,
				SourceCount: tv.SourceCount,
				DestCount:   tv.DestCount,
				OK:          tv.OK,
				Detail:      tv.Detail,
			}
			if tv.Err != nil {
				jv.Error = tv.Err.Error()
			}
			s.Verify = append(s.Verify, jv)
		}
	}

	s.Success = syncErr == nil && (!verified || vr.OK())
	switch {
	case syncErr != nil:
		s.Error = syncErr.Error()
	case verified && !vr.OK():
		s.Error = "verification failed: destination row counts do not match the source"
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(&s)
}
