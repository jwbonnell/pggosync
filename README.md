# pggosync

A CLI tool for syncing data between two PostgreSQL databases. Inspired by [pgsync](https://github.com/ankane/pgsync) and built as a Go learning exercise.

> **Note:** This is a personal learning project. Use at your own risk — not recommended for production.

## Contents

- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Connection Management](#connection-management)
- [Sync Config](#sync-config)
- [Commands](#commands)
- [Architecture](#architecture)
- [Development](#development)

---

## Prerequisites

- Go 1.21+
- PostgreSQL 12+
- Docker & Docker Compose (for integration tests only)

---

## Installation

Build from source:

```bash
make build   # produces binaries for linux/amd64 and linux/arm64 in ./bin/
```

Or install directly with Go:

```bash
go install github.com/jwbonnell/pggosync@latest
```

---

## Quick Start

**1. Create connection configs for your source and destination databases:**

```bash
pggosync init source
pggosync init dest
```

This writes placeholder YAML files to `$XDG_CONFIG_DIR/pggosync/` (e.g. `~/.config/pggosync/source.yaml`). Open each file and fill in your credentials.

**2. Create a sync config file** (see [Sync Config](#sync-config)).

**3. Run a sync:**

```bash
pggosync sync \
  --source source \
  --dest dest \
  --config my-sync.yml \
  --group my_group
```

Or launch the interactive TUI by running `pggosync` with no arguments.

---

## Connection Management

Each connection is a YAML file stored in `$XDG_CONFIG_DIR/pggosync/<name>.yaml`.

```yaml
host: localhost
port: 5432
database: mydb
user: myuser
password: secret
sslmode: disable
```

### Commands

```bash
# Create a new connection config with sensible defaults
pggosync init <name>

# List all saved connection names
pggosync config list

# Print a connection config (password is masked)
pggosync config get <name>
```

---

## Sync Config

The sync config is a YAML file you pass to `--config`. It defines named groups of tables with optional SQL WHERE filters and a top-level exclude list.

```yaml
description: "Sync config for staging refresh"

# Tables to always exclude, regardless of which groups are selected
exclude:
  - public.audit_log
  - public.sessions

groups:
  # A static group
  users:
    tables:
      - table: public.users
        filter: "active = true"
      - table: public.user_preferences

  # A parameterised group — pass values with --group by_tenant:42
  by_tenant:
    tables:
      - table: public.orders
        filter: "tenant_id = {1}"
      - table: public.order_items
        filter: "order_id IN (SELECT id FROM orders WHERE tenant_id = {1})"
```

### Parameter substitution

`{1}`, `{2}`, … in a filter are replaced with the corresponding positional values supplied via `--group <name>:<p1>,<p2>`:

```bash
pggosync sync ... --group by_tenant:42
# → tenant_id = 42

pggosync sync ... --group by_tenant:42,99
# → {1}=42, {2}=99
```

### Per-table overrides

Each table entry can override the global `--truncate` / `--preserve` flags:

```yaml
groups:
  mixed:
    tables:
      - table: public.large_table
        truncate: true    # always truncate this table
      - table: public.lookup
        preserve: true    # always preserve this table
```

---

## Commands

### `pggosync` (no subcommand)

Launches the interactive TUI. Navigate with arrow keys; Enter to select. Available screens:

- **Run Sync** — guided wizard to configure and execute a sync
- **Manage Connections** — create, view, and edit connection configs
- **Build Sync Config** — interactively compose a sync config YAML file

### `pggosync sync`

Syncs one or more groups or explicit tables from source to destination.

```
pggosync sync --source <name> --dest <name> --config <path> [flags]
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--source` | `-s` | — | Source connection name **(required)** |
| `--dest` | `-d` | — | Destination connection name **(required)** |
| `--config` | `-c` | — | Path to sync config YAML **(required)** |
| `--group` | `-g` | — | Group(s) to sync. Repeatable. Format: `name` or `name:p1,p2` |
| `--table` | `-t` | — | Explicit table(s) to sync. Repeatable. Format: `schema.table` or `schema.table:"filter"` |
| `--exclude` | `-e` | — | Table(s) to exclude. Repeatable. Format: `schema.table` |
| `--truncate` | `-tr` | false | Clear destination table before inserting |
| `--preserve` | `-p` | false | `ON CONFLICT DO NOTHING` — skip rows that already exist |
| `--defer-constraints` | `-dc` | false | Defer FK constraints for the duration of the transaction |
| `--disable-triggers` | `-dt` | false | Disable user-defined triggers during sync |
| `--no-safety` | `-ns` | false | Allow non-localhost destination |
| `--skip-confirmation` | `-sc` | false | Skip the interactive confirmation prompt |
| `--quiet` | `-q` | false | Suppress per-table progress output |
| `--dry-run` | `-dr` | false | Simulate without committing changes |
| `--concurrency` | `-con` | 1 | Number of source tables to pre-fetch concurrently |

#### Sync strategies

| Strategy | When | Behaviour |
|----------|------|-----------|
| **Upsert** (default) | No flags | Stages rows in a temp table, then `INSERT … ON CONFLICT DO UPDATE` on all non-PK columns |
| **Preserve** | `--preserve` | Same staging, but `ON CONFLICT DO NOTHING` — existing rows are left untouched |
| **Truncate** | `--truncate` | Clears the destination table first (TRUNCATE or DELETE when `--defer-constraints`), then streams directly via COPY |

Upsert and preserve require a primary key on the destination table. Use `--truncate` for tables without one.

#### Safety check

By default, the destination must resolve to `localhost` or `127.0.0.1`. Pass `--no-safety` to allow a remote destination.

#### Interactive confirmation

Without `--skip-confirmation`, the sync command prints a summary banner and waits for:
- `yes` — start the sync
- `no` — abort
- `more` — list each table with its strategy and (for truncate) current destination row count

#### Default scope

When neither `--group` nor `--table` is passed, all tables present in both source and destination (minus excludes) are synced.

---

### `pggosync validate`

Resolves tasks against both databases without syncing. Useful for checking that all tables exist and that PK requirements are met before running a full sync.

```
pggosync validate --source <name> --dest <name> --config <path> [--group ...] [--table ...] [--exclude ...]
```

Accepts `--truncate` and `--preserve` so you can validate under the same strategy you intend to use.

---

### `pggosync list`

Lists tables in source and destination, grouped as: both, source-only, destination-only.

```
pggosync list --source <name> --dest <name>
```

---

### `pggosync init <name>`

Creates a placeholder connection config file. Defaults to `default` if no name is given.

---

### `pggosync version`

Prints the build string set at compile time.

---

## Architecture

### Two config layers

**User config** — connection credentials, one YAML file per named connection in `$XDG_CONFIG_DIR/pggosync/`. Managed by `config.UserConfigHandler`.

**Sync config** — a YAML file passed at runtime via `--config`. Defines named groups of tables with optional WHERE filters and a global exclude list.

### Data flow

```
CLI flags + sync config YAML
          │
          ▼
    TaskResolver.Resolve()
    ├─ Expands --group and --table args into []Task
    ├─ Falls back to all shared tables when neither is supplied
    ├─ Loads columns, PKs, and sequences for each task
    ├─ Validates that all tables exist in both databases
    └─ Enforces PK requirement for upsert/preserve tasks
          │
          ▼
    sync.Sync()
    ├─ Opens a single destination transaction
    ├─ Optionally defers FK constraints (ALTER CONSTRAINT … DEFERRABLE)
    ├─ Optionally disables user triggers (ALTER TABLE … DISABLE TRIGGER)
    ├─ Launches pre-fetch goroutines: one per task, bounded by --concurrency
    │   Each goroutine streams COPY TO STDOUT from source into a SafeBuffer
    ├─ Sequential write loop: drains each SafeBuffer into the transaction
    │   ├─ Truncate path: TRUNCATE/DELETE → COPY FROM STDIN
    │   └─ Upsert/preserve path: create temp table → COPY FROM STDIN → INSERT … ON CONFLICT
    ├─ Syncs sequence values from source to destination
    └─ Commits (or rolls back on error / dry run)
```

### Copy strategies (TableSync)

**Truncate path** (`--truncate`): Clears the destination table first — `TRUNCATE CASCADE` normally, `DELETE FROM` when `--defer-constraints` (TRUNCATE cannot run inside a deferred-constraint transaction). Then streams rows directly via `COPY FROM STDIN`.

**Upsert / Preserve path** (default): Creates a temporary table on the destination, streams source rows into it via `COPY FROM STDIN`, then issues `INSERT … ON CONFLICT DO UPDATE` (upsert) or `DO NOTHING` (preserve). Requires a primary key on the destination table.

### Concurrency model

Source pre-fetching is concurrent; destination writes are strictly sequential (one task at a time, all inside the same transaction). The `SafeBuffer` type provides a mutex-protected, condition-variable-backed pipe so that a pre-fetch goroutine can write into a buffer while the destination write loop drains it, without locking both sides simultaneously.

### Package layout

| Package | Responsibility |
|---------|---------------|
| `cmd/` | CLI command definitions using `urfave/cli/v2` |
| `config/` | Connection credentials (`user.go`) and sync config parsing (`sync.go`) |
| `datasource/` | Read-only (`ReaderDataSource`) and read-write (`ReadWriteDatasource`) pgx connection wrappers |
| `db/` | Pure types (`Table`, `Column`, `PrimaryKey`, `Sequence`, `Trigger`) and low-level SQL helpers |
| `opts/` | Argument parsing for `--group name:params` and `--table schema.table:"filter"` |
| `sync/` | `Task`, `TaskResolver`, `TableSync`, `SafeBuffer`, and the top-level `Sync` orchestrator |
| `sync/table/` | Shared-table and exclusion-list filtering |
| `sync/data/` | Data scrubbing helpers (placeholder, not yet functional) |
| `tui/` | Interactive terminal UI built with Bubble Tea and Huh |

---

## Development

### Setup

```bash
# Start the test databases
docker compose up -d

# Verify they're reachable
psql postgresql://postgres:postgres@localhost:5432/postgres -c '\l'
psql postgresql://postgres:postgres@localhost:5433/postgres -c '\l'
```

### Running tests

```bash
# All tests (integration + unit) — requires Docker databases
make test

# Unit tests only — no Docker needed
make test-short

# A single named test
go test -count=1 -v -run TestName ./path/to/package/...
```

Integration tests live in `cmd/tests/` and connect directly to:
- Source: `localhost:5432`
- Destination: `localhost:5433`

Most skip automatically under `-short`; `TestTruncate` is the exception and always runs.

### Resetting test databases

```bash
make reset-docker-databases
```

### Building

```bash
make build   # linux/amd64 + linux/arm64 binaries in ./bin/
```
