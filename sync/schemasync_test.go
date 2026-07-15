package sync

import (
	"slices"
	"testing"
)

func TestPGDumpArgs(t *testing.T) {
	base := pgDumpArgs(false)
	want := []string{"--schema-only", "--no-owner", "--no-acl"}
	if !slices.Equal(base, want) {
		t.Fatalf("pgDumpArgs(false) = %v, want %v", base, want)
	}

	clean := pgDumpArgs(true)
	if !slices.Contains(clean, "--clean") || !slices.Contains(clean, "--if-exists") {
		t.Fatalf("pgDumpArgs(true) = %v, want it to contain --clean and --if-exists", clean)
	}
	// --schema-only must still be present alongside the clean flags.
	if !slices.Contains(clean, "--schema-only") {
		t.Fatalf("pgDumpArgs(true) = %v, want it to still contain --schema-only", clean)
	}
}

func TestPGEnv(t *testing.T) {
	p := SchemaSyncParams{
		Host:     "db.example.com",
		Port:     5433,
		User:     "svc",
		Password: "s3cr3t",
		Database: "app",
	}
	env := p.pgEnv()

	wantPresent := []string{
		"PGHOST=db.example.com",
		"PGPORT=5433",
		"PGUSER=svc",
		"PGPASSWORD=s3cr3t",
		"PGDATABASE=app",
	}
	for _, kv := range wantPresent {
		if !slices.Contains(env, kv) {
			t.Errorf("pgEnv() missing %q; got %v", kv, env)
		}
	}

	// PGSSLMODE must be absent when no mode is configured, so the driver default is preserved.
	for _, kv := range env {
		if len(kv) >= 9 && kv[:9] == "PGSSLMODE" {
			t.Errorf("pgEnv() set %q with empty SSLMode; expected it to be omitted", kv)
		}
	}

	// ...and present when a mode is set.
	p.SSLMode = "require"
	if !slices.Contains(p.pgEnv(), "PGSSLMODE=require") {
		t.Errorf("pgEnv() missing PGSSLMODE=require when SSLMode set")
	}
}
