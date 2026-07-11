# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## About

pggosync is a CLI tool for syncing data between two PostgreSQL databases. It is inspired by [pgsync](https://github.com/ankane/pgsync) and built as a Go learning exercise.

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

**Sync config** (groups and exclusions) is a YAML file referenced at runtime via `--config`, either by path or by bare name. Names are resolved by `config/resolve.go` against `./.pggosync/configs/` (project-local) then `$XDG_CONFIG_DIR/pggosync/configs/` (user-level). It defines named groups of tables with optional SQL WHERE filters, per-table scrub rules, and a top-level `exclude` list. See `_configs/default.yml` for a reference example.

`config.UserConfigHandler` also manages sync profiles (one YAML file per profile — see below) and `history.json` (a rolling record of the last 20 TUI-run syncs) in the same config directory.

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
   │   into SafeBuffers; scrub rules are applied as SQL expressions in the source COPY query
   ├─ Drains each SafeBuffer sequentially via TableSync.SyncFromBuffer()
   └─ Commits or rolls back (rollback on any task error or when --dry-run)
```

Source pre-fetching is concurrent; destination writes are strictly sequential and share one transaction. `SafeBuffer` is a mutex/condition-variable pipe that lets a prefetch goroutine fill a buffer while the write loop drains it.

### TableSync: Two Copy Strategies

**Truncate path** (`--truncate`): clears the destination table first (TRUNCATE or DELETE if `--defer-constraints`), then streams data directly using PostgreSQL `COPY TO STDOUT` / `COPY FROM STDIN`.

**Upsert path** (default): copies source data into a temp table on the destination, then runs `INSERT ... ON CONFLICT DO UPDATE` (upsert) or `DO NOTHING` (with `--preserve`). Requires a primary key on the destination table.

### Data Scrubbing

Scrub rules anonymise column values during a sync. A rule ID (`hash`, `redact`, `null`, `random_int`, `random_float`, `random_email`, `partial[:n]`, `static[:value]`) maps to a SQL expression via `sync/data.SQLExpression`. The expression is spliced into the `SELECT` of the source-side `COPY TO STDOUT` query (`Task.GetSelectColumns`), so raw values never leave the source. Rules are configured per table in the sync config (`scrub:` block) or inline on `--table schema.table[:filter][:col=rule,...]`.

### Profiles

A `config.SyncProfile` is a named bundle of sync options (source, dest, config file, groups, flags) stored as one YAML file per profile; the name comes from the filename stem. Profiles are resolved by name or path against `./.pggosync/profiles/` then `$XDG_CONFIG_DIR/pggosync/profiles/` (the TUI saves to the latter; a legacy `profiles.json` is auto-migrated on first load). `pggosync profile sync <name>` loads one as defaults, filling only fields the user did not set explicitly (`cCtx.IsSet` guards in `cmd/profile.go`), then delegates to `executeSync` in `cmd/run.go`. Note: schema sync (`sync/schemasync.go`) is an unfinished stub and is not wired into any command.

### Package Responsibilities

| Package | Responsibility |
|---|---|
| `cmd/` | CLI command definitions using `urfave/cli/v2` (`run`, `tables`, `conn`, `profile`, `config`, `version`; no subcommand launches the TUI, unknown subcommands error) |
| `config/` | User config (credentials), sync config (groups/filters/scrub), name-or-path resolution, per-file YAML profiles, and sync history |
| `datasource/` | `ReaderDataSource` (read-only pgx connection) and `ReadWriteDatasource` (embeds reader, adds writes) |
| `db/` | Pure types (Table, Column, PrimaryKey, etc.) and low-level SQL helpers |
| `opts/` | Argument parsing for `--group name:params` and `--table schema.table[:filter][:col=rule]` |
| `sync/` | Task struct, TaskResolver, TableSync, SafeBuffer, and the top-level Sync orchestrator |
| `sync/table/` | Table filtering (shared tables, exclusion lists) |
| `sync/data/` | Scrub rule → SQL expression mapping |
| `tui/` | Interactive terminal UI (Bubble Tea + Huh): sync wizard, connection manager, sync-config builder, profiles |

### Safety Check

By default the destination database must resolve to `localhost` or `127.0.0.1`. Pass `--no-safety` to override. The check is enforced in `executeSync` (`cmd/run.go`) before `sync.Sync()` is called.
