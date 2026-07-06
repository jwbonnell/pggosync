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

Integration tests in `cmd/tests/` connect to the Docker databases directly (source: `localhost:5432`, dest: `localhost:5433`). Most skip automatically with `-short`; `TestTruncate` does not.

## Architecture

### Two Config Layers

**User config** (connection credentials) is stored at `$XDG_CONFIG_DIR/pggosync/<name>.yaml`. A file named `default` in that directory holds the active config name. Managed via `pggosync init` / `pggosync config`. The `config.UserConfigHandler` handles all reads and writes to this directory.

**Sync config** (groups and exclusions) is a YAML file passed at runtime via `--config`. It defines named groups of tables with optional SQL WHERE filters and a top-level `exclude` list. See `_configs/default.yml` for a reference example.

### Sync Config Groups and Param Substitution

Groups map table names to WHERE clause filters. Filters support `{N}` placeholders that are substituted with positional parameters passed as `--group groupname:param1,param2`. For example, `--group country_var_1:42` substitutes `{1}` with `42` in all filters for the `country_var_1` group.

### Data Flow

```
CLI flags + sync config
        │
        ▼
   TaskResolver.Resolve()
   ├─ Expands groups/tables into []Task
   ├─ Loads columns, PKs, and sequences for each task
   └─ Returns []Task with full metadata
        │
        ▼
   sync.Sync()
   ├─ Opens a single destination transaction
   ├─ Optionally defers FK constraints or disables user triggers
   ├─ Dispatches tasks via channel (maxConcurrency = 1)
   ├─ Each task handled by TableSync.Sync()
   └─ Commits or rolls back the transaction
```

### TableSync: Two Copy Strategies

**Truncate path** (`--truncate`): clears the destination table first (TRUNCATE or DELETE if `--defer-constraints`), then streams data directly using PostgreSQL `COPY TO STDOUT` / `COPY FROM STDIN`.

**Upsert path** (default): copies source data into a temp table on the destination, then runs `INSERT ... ON CONFLICT DO UPDATE` (upsert) or `DO NOTHING` (with `--preserve`). Requires a primary key on the destination table.

### Package Responsibilities

| Package | Responsibility |
|---|---|
| `cmd/` | CLI command definitions using `urfave/cli/v2` |
| `config/` | User config (credentials) and sync config (groups/filters) loading |
| `datasource/` | `ReaderDataSource` (read-only pgx connection) and `ReadWriteDatasource` (embeds reader, adds writes) |
| `db/` | Pure types (Table, Column, PrimaryKey, etc.) and low-level SQL helpers |
| `opts/` | Argument parsing for `--group name:params` and `--table schema.table:"filter"` |
| `sync/` | Task struct, TaskResolver, TableSync, and the top-level Sync orchestrator |
| `sync/table/` | Table filtering (shared tables, exclusion lists) |

### Safety Check

By default the destination database must resolve to `localhost` or `127.0.0.1`. Pass `--no-safety` to override. The check is enforced in `cmd/sync.go` before `sync.Sync()` is called.
