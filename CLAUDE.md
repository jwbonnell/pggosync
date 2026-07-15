# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## About

pggosync is a CLI tool for syncing data between two PostgreSQL databases. It is inspired by [pgsync](https://github.com/ankane/pgsync) and built as a Go learning exercise.

## Documentation Maintenance

`docs/GUIDE.md` and the sub-guides it links (`docs/guide/commands.md`, `docs/guide/usage.md`, `docs/guide/tui.md`) are the detailed user-facing guide. **With every change, check whether the guide needs updating** — anything touching commands, flags, defaults, config file formats, search/resolution order, sync strategies, scrub rules, safety behavior, or TUI screens/keybindings is documented there and must be updated in the same change.

## Commands

```bash
# Run all tests (requires Docker Compose running)
make test

# Run only unit tests (skip integration tests that need Docker)
make test-short

# Run a single test
go test -count=1 -v -run TestName ./path/to/package/...

# Build binaries (amd64 + arm64)
make build

# Start Docker databases for integration testing
docker compose up -d

# Reset Docker databases to clean state
make reset-docker-databases
```

Integration tests in `cmd/tests/` connect to the Docker databases directly (source: `localhost:5444`, dest: `localhost:5445`) and skip automatically with `-short`.

## Architecture

### Two Config Layers

**User config** (connection credentials) is stored at `$XDG_CONFIG_DIR/pggosync/<name>.yaml`. Managed via `pggosync conn` (`init`, `new`, `list`, `get`, `test`). The `config.UserConfigHandler` handles all reads and writes to this directory.

**Sync config** (groups and exclusions) is a YAML file referenced at runtime via `--config`, either by path or by bare name. Names are resolved by `config/resolve.go` against `./.pggosync/configs/` (project-local) then `$XDG_CONFIG_DIR/pggosync/configs/` (user-level), then any extra include paths (see below). It defines named groups of tables with optional SQL WHERE filters, per-table scrub rules, and a top-level `exclude` list. See `_configs/configs/default.ym` for a reference example.

`config.UserConfigHandler` also manages sync profiles (one YAML file per profile — see below), `history.json` (a rolling record of the last 20 TUI-run syncs), and `prefs.yaml` (general settings) in the same config directory.

**Include paths** let users register extra base directories to search for configs and profiles. They are stored in `prefs.yaml` under `include.paths` (`config/prefs.go`) and appended to the search order after the project and user dirs (so the defaults shadow them). Managed via `pggosync config paths` (list) and `pggosync config paths add <path>`; `add` requires the path to exist and contain a `configs/` and/or `profiles/` subdirectory, and stores it as an absolute path.

> **TODO — search-order / duplicate-name collisions:** The precedence of include paths only matters when the same bare config/profile name exists in more than one search dir. In that case `resolveNamed` returns the first match and `listNamed` shadows later dirs by name, so today the project dir wins over user, and user wins over include paths (include paths are lowest priority, in insertion order). Literal paths bypass search and are unaffected. This is not yet surfaced to the user — there is no warning on a shadowed name, and the precedence ordering is not configurable. Decide later whether explicitly-added include paths should be able to outrank the defaults and whether to warn on collisions. See `searchDirs` in `config/resolve.go`.

### Sync Config Groups and Param Substitution

Groups map table names to WHERE clause filters. Filters support `{N}` placeholders that are substituted with positional parameters passed as `--group groupname:param1,param2`. For example, `--group country_var_1:42` substitutes `{1}` with `42` in all filters for the `country_var_1` group.

### Data Flow

```
CLI flags + sync config
        │
        ▼
   TaskResolver.Resolve()
   ├─ Expands groups/tables into []Task
   ├─ Loads columns, PKs, sequences, and scrub rules for each task
   └─ Returns []Task with full metadata
        │
        ▼
   sync.Sync()
   ├─ Opens a single destination transaction
   ├─ Optionally defers FK constraints or disables user triggers
   ├─ Pre-fetches source rows concurrently (one goroutine per task, capped by --concurrency)
   │   into bounded SafeBuffers (--buffer-size cap, default 32 MiB; the source COPY blocks when
   │   full — backpressure); scrub rules are applied as SQL expressions in the source COPY query
   ├─ Drains each SafeBuffer sequentially via TableSync.SyncFromBuffer()
   └─ Commits or rolls back (rollback on any task error or when --dry-run)
```

Source pre-fetching is concurrent; destination writes are strictly sequential and share one transaction. `SafeBuffer` is a mutex/condition-variable pipe that lets a prefetch goroutine fill a buffer while the write loop drains it. It is **bounded** by `--buffer-size` (bytes; default 32 MiB, passed to `sync.Sync`): `Write` blocks once the buffer is full and resumes as the reader drains it, so peak prefetch memory is bounded by `concurrency × buffer-size` rather than table size (real RSS runs several times higher — `bytes.Buffer` growth plus pgx/COPY driver buffers). `Sync` calls `SafeBuffer.Close()` on return to unblock any prefetch goroutine parked at the cap (which is not context-aware), preventing goroutine leaks on early error.

### TableSync: Two Copy Strategies

**Truncate path** (`--truncate`): clears the destination table first (TRUNCATE or DELETE if `--defer-constraints`), then streams data directly using PostgreSQL `COPY TO STDOUT` / `COPY FROM STDIN`.

**Upsert path** (default): copies source data into a temp table on the destination, then runs `INSERT ... ON CONFLICT DO UPDATE` (upsert) or `DO NOTHING` (with `--preserve`). Requires a primary key on the destination table.

### Data Scrubbing

Scrub rules anonymise column values during a sync. A rule ID (`hash`, `redact`, `null`, `random_int`, `random_float`, `random_email`, `partial[:n]`, `static[:value]`) maps to a SQL expression via `sync/data.SQLExpression`. The expression is spliced into the `SELECT` of the source-side `COPY TO STDOUT` query (`Task.GetSelectColumns`), so raw values never leave the source. Rules are configured per table in the sync config (`scrub:` block) or inline on `--table schema.table[:filter][:col=rule,...]`.

### Profiles

A `config.SyncProfile` is a named bundle of sync options (source, dest, config file, groups, flags) stored as one YAML file per profile; the name comes from the filename stem. Profiles are resolved by name or path against `./.pggosync/profiles/` then `$XDG_CONFIG_DIR/pggosync/profiles/` (the TUI saves to the latter; a legacy `profiles.json` is auto-migrated on first load). `pggosync profile sync <name>` loads one as defaults, filling only fields the user did not set explicitly (`cCtx.IsSet` guards in `cmd/profile.go`), then delegates to `executeSync` in `cmd/run.go`. Profiles carry data-sync options only; schema sync (below) is a separate CLI-only command and is not part of a profile.

### Package Responsibilities

| Package | Responsibility |
|---|---|
| `cmd/` | CLI command definitions using `urfave/cli/v2` (`run`, `tables`, `conn`, `profile`, `config`, `schema`, `version`; no subcommand launches the TUI, unknown subcommands error) |
| `config/` | User config (credentials), sync config (groups/filters/scrub), name-or-path resolution, per-file YAML profiles, and sync history |
| `datasource/` | `ReaderDataSource` (read-only pgx connection) and `ReadWriteDatasource` (embeds reader, adds writes) |
| `db/` | Pure types (Table, Column, PrimaryKey, etc.) and low-level SQL helpers |
| `opts/` | Argument parsing for `--group name:params` and `--table schema.table[:filter][:col=rule]` |
| `sync/` | Task struct, TaskResolver, TableSync, SafeBuffer, the top-level Sync orchestrator, the post-commit `Verify` row-count check, and `SchemaSync` (schema-only DDL copy) |
| `sync/table/` | Table filtering (shared tables, exclusion lists) |
| `sync/data/` | Scrub rule → SQL expression mapping |
| `tui/` | Interactive terminal UI (Bubble Tea + Huh): sync wizard, connection manager, sync-config builder, profiles |

### Post-Sync Verification

`--verify` runs `sync.Verify` (`sync/verify.go`) **after** `sync.Sync` commits — it is invoked from `executeSync` (`cmd/run.go`) for the CLI and from the wizard's sync goroutine (`tui/sync_wizard.go`) for the TUI. For each task it re-counts the source with the task filter and the destination whole-table, then compares by strategy: truncate must match exactly, upsert/preserve must have `dest >= source` (the destination keeps rows outside the synced slice). A mismatch is surfaced as a non-zero CLI exit / a red results banner in the TUI; the sync has already committed, so verification cannot roll anything back. It is a **row-count** check only — never a value/checksum comparison, because scrub rules make source and destination values differ by design (and non-deterministic rules would never match). Skipped under `--dry-run`. Threaded through the CLI flag, profiles (`Verify`), and the TUI options like the other flags.

### Schema Sync (`pggosync schema sync`)

`sync.SchemaSync` (`sync/schemasync.go`) copies the **whole database schema** (DDL) from source to destination by shelling out to **`pg_dump --schema-only --no-owner --no-acl`** piped to **`psql`** (both must be on `PATH`; checked via `exec.LookPath`). Connection credentials are passed to the child processes as **libpq environment variables** (`PGHOST`, `PGPORT`, `PGUSER`, `PGPASSWORD`, `PGDATABASE`, `PGSSLMODE`) via `SchemaSyncParams.pgEnv`, so the password never lands in `argv`. The pipeline uses a raw `os.Pipe` between the two `exec.Cmd`s.

Two behaviours (`SchemaSyncOptions`): the default creates objects missing on the destination and leaves existing ones untouched — `psql` runs without `ON_ERROR_STOP`, so "already exists" errors are non-fatal and it still exits 0 (it does **not** reconcile a drifted table missing a source column). `Clean` adds `pg_dump --clean --if-exists` to drop & recreate every object (destructive). `DryRun` runs `pg_dump` alone and writes the DDL to `out`, applying nothing.

The command lives in `cmd/schema.go` (`schema` group → `sync` subcommand). It is data-agnostic — no groups/`--table`/scrub — reuses the `run` helpers (`requireConnections`, `resolveConnections`, `setupDatasources`) and the same localhost destination safety check, and is **CLI-only**: not wired into profiles or the TUI (`schemasync.go` imports only stdlib, no `config`).

### JSON Output (`--output json`)

`run` and `profile sync` accept `--output json` (`opts.CLIArgs.Output`), which prints a single machine-readable summary (`cmd/jsonoutput.go`, `printJSONSummary`) to **stdout** — per-table strategy/rows/error, the `Verify` results when enabled, `success`, `error`, and `elapsed_ms` — while all human progress (from `sync.Sync`) and verify lines are routed to **stderr** so stdout stays parseable. The summary is emitted even on failure (with `error` set); the process exit code still reflects success. It requires `--skip-confirmation` (the prompt can't share stdout) and is a per-invocation flag — deliberately not stored in profiles and not exposed in the TUI (JSON output is meaningless in an interactive UI).

### Safety Check

By default the destination database must resolve to `localhost` or `127.0.0.1`. Pass `--no-safety` to override. The check is enforced in `executeSync` (`cmd/run.go`) before `sync.Sync()` is called.

## Commit Messages

Any time you make changes to the repository — code, tests, docs, config, or anything else — always finish by providing a brief draft commit message for the user to review and copy. Provide one every time changes are made, including small or partial updates. Never commit automatically; just supply the suggested message and let the user decide whether to use it.
