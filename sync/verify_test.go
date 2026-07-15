package sync

import (
	"errors"
	"testing"
)

func TestVerifyVerdict(t *testing.T) {
	cases := []struct {
		name      string
		truncated bool
		src, dest int64
		wantOK    bool
	}{
		{"truncate exact match", true, 100, 100, true},
		{"truncate dest short", true, 100, 90, false},
		{"truncate dest over", true, 100, 110, false},
		{"truncate both empty", true, 0, 0, true},
		{"upsert dest equal", false, 100, 100, true},
		{"upsert dest greater keeps pre-existing rows", false, 100, 150, true},
		{"upsert dest short means rows went missing", false, 100, 99, false},
		{"preserve dest equal", false, 50, 50, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, detail := verifyVerdict(tc.truncated, tc.src, tc.dest)
			if ok != tc.wantOK {
				t.Errorf("verifyVerdict(%v, %d, %d) ok = %v, want %v (detail: %q)", tc.truncated, tc.src, tc.dest, ok, tc.wantOK, detail)
			}
			if detail == "" {
				t.Error("expected a non-empty detail string")
			}
		})
	}
}

func TestVerifyResultOK(t *testing.T) {
	if !(VerifyResult{}).OK() {
		t.Error("an empty result should be OK")
	}
	if !(VerifyResult{Tables: []TableVerify{{OK: true}, {OK: true}}}).OK() {
		t.Error("an all-passing result should be OK")
	}
	if (VerifyResult{Tables: []TableVerify{{OK: true}, {OK: false}}}).OK() {
		t.Error("a failing table should make the result not OK")
	}
	if (VerifyResult{Tables: []TableVerify{{OK: true, Err: errors.New("boom")}}}).OK() {
		t.Error("a table with a count error should make the result not OK")
	}
}
