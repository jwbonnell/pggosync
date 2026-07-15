package sync

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
)

// SchemaSyncParams holds the connection details needed to shell out to pg_dump/psql. Credentials are
// passed to the child processes as libpq environment variables (see pgEnv) rather than on the command
// line, so the password never appears in argv / a process listing.
type SchemaSyncParams struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	SSLMode  string
}

// SchemaSyncOptions selects the schema-sync behaviour.
//
//	Clean:  drop & recreate every object so the destination schema exactly matches source
//	        (pg_dump --clean --if-exists). Destructive — wipes data in recreated tables.
//	DryRun: run pg_dump only and write the DDL to out; apply nothing to the destination.
type SchemaSyncOptions struct {
	Clean  bool
	DryRun bool
}

// pgEnv returns the process environment for a pg_dump/psql invocation: the parent environment plus the
// libpq connection variables. PGSSLMODE is only set when a mode is configured so the driver default is
// preserved otherwise.
func (p SchemaSyncParams) pgEnv() []string {
	env := append(os.Environ(),
		"PGHOST="+p.Host,
		"PGPORT="+strconv.Itoa(p.Port),
		"PGUSER="+p.User,
		"PGPASSWORD="+p.Password,
		"PGDATABASE="+p.Database,
	)
	if p.SSLMode != "" {
		env = append(env, "PGSSLMODE="+p.SSLMode)
	}
	return env
}

// pgDumpArgs builds the pg_dump argument list for a schema-only dump. With clean it emits
// DROP ... IF EXISTS before each CREATE so an existing object is replaced rather than skipped.
func pgDumpArgs(clean bool) []string {
	args := []string{"--schema-only", "--no-owner", "--no-acl"}
	if clean {
		args = append(args, "--clean", "--if-exists")
	}
	return args
}

// SchemaSync copies the whole database schema (DDL) from source to dest by shelling out to
// pg_dump --schema-only, piped to psql. It requires pg_dump (and, unless DryRun, psql) on PATH.
//
// Default behaviour creates objects missing on the destination and leaves existing ones untouched:
// psql runs without ON_ERROR_STOP, so an "already exists" error on an object is reported but
// non-fatal and psql still exits 0. It does not reconcile drift (e.g. a table missing a source
// column) — for that, use Clean to drop & recreate. On DryRun the generated DDL is written to out and
// nothing is applied.
//
// out receives the generated DDL (DryRun) or psql's stdout (apply); errOut receives pg_dump/psql
// diagnostics. pg_dump's stderr is also captured so a dump failure can surface its message.
func SchemaSync(ctx context.Context, source, dest SchemaSyncParams, o SchemaSyncOptions, out, errOut io.Writer) error {
	if _, err := exec.LookPath("pg_dump"); err != nil {
		return fmt.Errorf("pg_dump not found on PATH — schema sync requires the PostgreSQL client tools (pg_dump, psql): %w", err)
	}
	if !o.DryRun {
		if _, err := exec.LookPath("psql"); err != nil {
			return fmt.Errorf("psql not found on PATH — schema sync requires the PostgreSQL client tools (pg_dump, psql): %w", err)
		}
	}

	dump := exec.CommandContext(ctx, "pg_dump", pgDumpArgs(o.Clean)...)
	dump.Env = source.pgEnv()

	var dumpErr bytes.Buffer
	dump.Stderr = io.MultiWriter(&dumpErr, errOut)

	if o.DryRun {
		dump.Stdout = out
		if err := dump.Run(); err != nil {
			return fmt.Errorf("pg_dump failed: %w\n%s", err, dumpErr.String())
		}
		return nil
	}

	psql := exec.CommandContext(ctx, "psql")
	psql.Env = dest.pgEnv()
	psql.Stdout = out
	psql.Stderr = errOut

	// Wire pg_dump's stdout to psql's stdin with an OS pipe. Passing *os.File ends directly lets
	// os/exec dup them into the children without spawning copier goroutines; the parent then closes
	// its own copies so psql sees EOF once pg_dump exits.
	pr, pw, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("could not create pipe: %w", err)
	}
	dump.Stdout = pw
	psql.Stdin = pr

	if err := dump.Start(); err != nil {
		pr.Close()
		pw.Close()
		return fmt.Errorf("could not start pg_dump: %w", err)
	}
	if err := psql.Start(); err != nil {
		pr.Close()
		pw.Close()
		_ = dump.Process.Kill()
		_ = dump.Wait()
		return fmt.Errorf("could not start psql: %w", err)
	}

	// The children hold their own dup'd fds; close the parent's copies so EOF propagates correctly.
	pw.Close()
	pr.Close()

	dumpWaitErr := dump.Wait()
	psqlWaitErr := psql.Wait()

	if dumpWaitErr != nil {
		return fmt.Errorf("pg_dump failed: %w\n%s", dumpWaitErr, dumpErr.String())
	}
	if psqlWaitErr != nil {
		return fmt.Errorf("psql failed applying schema to destination: %w", psqlWaitErr)
	}
	return nil
}
