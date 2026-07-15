package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jwbonnell/pggosync/opts"
	"github.com/jwbonnell/pggosync/sync"
)

func decodeSummary(t *testing.T, b []byte) jsonSummary {
	t.Helper()
	var s jsonSummary
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, b)
	}
	return s
}

func TestPrintJSONSummary_SuccessWithVerify(t *testing.T) {
	res := sync.SyncResult{Tables: []sync.TableResult{{Table: "public.a", Strategy: "truncate", Rows: 10}}}
	vr := sync.VerifyResult{Tables: []sync.TableVerify{{Table: "public.a", Strategy: "truncate", SourceCount: 10, DestCount: 10, OK: true, Detail: "exact match (10 rows)"}}}
	var buf bytes.Buffer
	if err := printJSONSummary(&buf, "src", "dst", opts.CLIArgs{}, res, nil, vr, true, time.Duration(0)); err != nil {
		t.Fatalf("printJSONSummary: %v", err)
	}
	s := decodeSummary(t, buf.Bytes())
	if !s.Success {
		t.Error("expected success=true")
	}
	if !s.Verified || !s.VerifyOK {
		t.Errorf("expected verified & verify_ok true, got verified=%v verify_ok=%v", s.Verified, s.VerifyOK)
	}
	if len(s.Tables) != 1 || s.Tables[0].Rows != 10 {
		t.Errorf("unexpected tables: %+v", s.Tables)
	}
	if len(s.Verify) != 1 || s.Verify[0].DestCount != 10 {
		t.Errorf("unexpected verify: %+v", s.Verify)
	}
	if s.Error != "" {
		t.Errorf("expected no error, got %q", s.Error)
	}
}

func TestPrintJSONSummary_SyncError(t *testing.T) {
	res := sync.SyncResult{Tables: []sync.TableResult{{Table: "public.a", Strategy: "truncate", Err: errors.New("boom")}}}
	var buf bytes.Buffer
	_ = printJSONSummary(&buf, "src", "dst", opts.CLIArgs{}, res, errors.New("1 task(s) failed"), sync.VerifyResult{}, false, 0)
	s := decodeSummary(t, buf.Bytes())
	if s.Success {
		t.Error("expected success=false on sync error")
	}
	if s.Verified {
		t.Error("expected verified=false when verification did not run")
	}
	if s.Error == "" {
		t.Error("expected top-level error populated")
	}
	if len(s.Tables) != 1 || s.Tables[0].Error == "" {
		t.Errorf("expected per-table error populated, got %+v", s.Tables)
	}
}

func TestPrintJSONSummary_VerifyFailed(t *testing.T) {
	res := sync.SyncResult{Tables: []sync.TableResult{{Table: "public.a", Strategy: "truncate", Rows: 10}}}
	vr := sync.VerifyResult{Tables: []sync.TableVerify{{Table: "public.a", Strategy: "truncate", SourceCount: 10, DestCount: 5, OK: false, Detail: "expected dest == source 10, got 5"}}}
	var buf bytes.Buffer
	_ = printJSONSummary(&buf, "src", "dst", opts.CLIArgs{}, res, nil, vr, true, 0)
	s := decodeSummary(t, buf.Bytes())
	if s.Success {
		t.Error("expected success=false when verification failed")
	}
	if s.VerifyOK {
		t.Error("expected verify_ok=false")
	}
	if s.Error == "" {
		t.Error("expected error to note the verification failure")
	}
}
