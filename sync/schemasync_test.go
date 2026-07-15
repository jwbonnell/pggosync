package sync

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
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

// TestSchemaSync_MissingBinaries verifies SchemaSync fails fast with a clear message when the
// PostgreSQL client tools it shells out to are not on PATH, rather than attempting to run anything.
func TestSchemaSync_MissingBinaries(t *testing.T) {
	// Empty PATH — pg_dump can't be found. DryRun so only pg_dump is required.
	t.Setenv("PATH", "")
	err := SchemaSync(context.Background(), SchemaSyncParams{}, SchemaSyncParams{}, SchemaSyncOptions{DryRun: true}, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "pg_dump not found on PATH") {
		t.Fatalf("empty PATH: want pg_dump-not-found error, got %v", err)
	}

	// pg_dump present but psql absent — the apply path must report psql missing. The fake pg_dump is
	// never executed (the psql lookup fails first), so its contents don't matter.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pg_dump"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("writing fake pg_dump: %v", err)
	}
	t.Setenv("PATH", dir)
	err = SchemaSync(context.Background(), SchemaSyncParams{}, SchemaSyncParams{}, SchemaSyncOptions{}, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "psql not found on PATH") {
		t.Fatalf("pg_dump-only PATH: want psql-not-found error, got %v", err)
	}
}

// TestSchemaSync_DumpFailureSurfaced verifies a pg_dump failure (here, an unreachable source) is
// surfaced as an error rather than swallowed. Requires the pg_dump binary; skipped otherwise.
func TestSchemaSync_DumpFailureSurfaced(t *testing.T) {
	if _, err := exec.LookPath("pg_dump"); err != nil {
		t.Skip("pg_dump not on PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Nothing listens on 127.0.0.1:1, so pg_dump fails fast. DryRun so no psql is involved.
	bad := SchemaSyncParams{Host: "127.0.0.1", Port: 1, User: "x", Password: "x", Database: "x"}
	err := SchemaSync(ctx, bad, SchemaSyncParams{}, SchemaSyncOptions{DryRun: true}, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "pg_dump failed") {
		t.Fatalf("want pg_dump failure error, got %v", err)
	}
}
