# pggosync

A CLI tool for syncing data between two PostgreSQL databases. Inspired by [pgsync](https://github.com/ankane/pgsync) and built as a Go learning exercise.

> **Note:** This is a personal learning project. Use at your own risk — not recommended for production.

## Contents

- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Connection Management](#connection-management)
- [Sync Config](#sync-config)
- [Data Scrubbing](#data-scrubbing)
- [Profiles](#profiles)
- [Commands](#commands)
- [Architecture](#architecture)
- [Development](#development)

---

## Prerequisites

- Go 1.26+
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
pggosync conn init source
pggosync conn init dest
```

This writes placeholder YAML files to `$XDG_CONFIG_DIR/pggosync/` (e.g. `~/.config/pggosync/source.yaml`). Open each file and fill in your credentials — or use `pggosync conn new` for an interactive form.

**2. Create a sync config file** (see [Sync Config](#sync-config)).

**3. Run a sync:**

```bash
pggosync run \
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
port: 5444
database: mydb
user: myuser
password: secret
sslmode: disable
```

### Commands

```bash
# Create a new connection config with placeholder defaults
pggosync conn init <name>

# Create a connection interactively
pggosync conn new

# List all saved connection names
pggosync conn list

# Print a connection config (password is masked)
pggosync conn get <name>

# Check that a connection can actually reach its database
pggosync conn test <name>
```

---

## Sync Config

The sync config is a YAML file referenced by `--config`. It defines named groups of tables with optional SQL WHERE filters and a top-level exclude list.

`--config` accepts either a file path or a bare name. A name is looked up as `<name>.yaml`/`<name>.yml` in, order of precedence:

1. `./.pggosync/configs/` — project-local, commit these to share configs with your team
2. `$XDG_CONFIG_DIR/pggosync/configs/` — user-level

Use `pggosync config list` to see what's discoverable and `pggosync config validate <name-or-path>` to check a config.

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
pggosync run ... --group by_tenant:42
# → tenant_id = 42

pggosync run ... --group by_tenant:42,99
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
        truncate: false   # required if the run may pass --truncate globally
```

A table must resolve to exactly one strategy: an entry that ends up with both truncate and preserve set (for example, a per-table `preserve: true` combined with a global `--truncate`) is rejected — add an explicit `truncate: false`/`preserve: false` override to disambiguate.

### Per-table scrub rules

Each table entry can also anonymise columns on the way through (see [Data Scrubbing](#data-scrubbing)):

```yaml
groups:
  users:
    tables:
      - table: public.users
        scrub:
          - column: email
            rule: random_email
          - column: ssn
            rule: redact
          - column: name
            rule: "partial:3"
```

---

## Data Scrubbing

Scrubbing anonymizes column values during the sync. The rule is compiled into a
SQL expression that runs **on the source** inside the `COPY TO STDOUT` query, so
raw values never leave the source database — only the transformed value is streamed
to the destination.

Configure scrub rules per table either in the sync config (`scrub:` block, above) or
inline on a `--table` argument:

```bash
pggosync run ... \
  --table 'public.users::email=random_email,ssn=redact,name=partial:3'
```

The `--table` format is `schema.table[:filter][:col1=rule1,col2=rule2]`. Note the
empty filter segment (`::`) when scrubbing without a WHERE filter.

### Available rules

| Rule | Result |
|------|--------|
| `hash` | `MD5(value)` |
| `redact` | Constant `'***REDACTED***'` |
| `null` | `NULL` |
| `random_int` | Random integer 0–100000 |
| `random_float` | Random `numeric(10,2)` 0–1000 |
| `random_email` | `user<random>@example.com` |
| `partial` / `partial:N` | First `N` chars (default 3) then `***` |
| `static` / `static:value` | Constant `value` (default `***`) |

Parameterised rules take their argument after a colon (`partial:5`, `static:test@example.com`).

---

## Profiles

A profile is a saved, named bundle of sync options (source, dest, config file,
groups, and flags). Each profile is a single YAML file, resolved like sync
configs — by name or path, searching:

1. `./.pggosync/profiles/` — project-local, commit these to share profiles with your team
2. `$XDG_CONFIG_DIR/pggosync/profiles/` — user-level (this is where the TUI's
   **Manage Profiles** screen saves them)

A legacy `profiles.json` is migrated automatically into per-file YAML on first use.

```yaml
# ./.pggosync/profiles/nightly-staging.yaml — the name comes from the filename
source: prod
dest: local
config_file: default        # name or path, resolved like --config
groups: [users]
truncate: true
concurrency: 4
```

Run a sync from a profile with `profile sync`. Any explicit flag overrides the
profile's stored value (flags go before the profile name):

```bash
pggosync profile sync nightly-staging
pggosync profile sync --dry-run nightly-staging   # override just the dry-run flag

pggosync profile list                     # list discoverable profiles
pggosync profile show nightly-staging     # print a profile's contents
pggosync profile validate nightly-staging # check connections + config file exist and parse
```

---

## Commands

```
pggosync                                → interactive TUI
pggosync run [flags]                    → run a sync
pggosync tables -s <src> -d <dst>       → diff tables between the two databases
pggosync conn init|new|list|get|test    → manage connections
pggosync profile list|show|sync|validate → manage and run profiles
pggosync config list|validate           → manage sync configs
pggosync version                        → print version
```

Unknown subcommands are an error; only a bare `pggosync` opens the TUI.

### `pggosync` (no subcommand)

Launches the interactive TUI. Navigate with arrow keys; Enter to select. Available screens:

- **Run Sync** — guided wizard to configure and execute a sync
- **Manage Connections** — create, view, and switch connection configs
- **Build Sync Config** — interactively compose a sync config YAML file
- **Manage Profiles** — save and launch named sync configurations (see [Profiles](#profiles))

The menu also shows a **last sync** summary line. TUI-run syncs are recorded to a
rolling history file (`history.json`, last 20 runs) in `$XDG_CONFIG_DIR/pggosync/`.

### `pggosync run`

Syncs one or more groups or explicit tables from source to destination.

```
pggosync run --source <name> --dest <name> --config <name-or-path> [flags]
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--source` | `-s` | — | Source connection name **(required)** |
| `--dest` | `-d` | — | Destination connection name **(required)** |
| `--config` | `-c` | — | Sync config name or path **(required)** |
| `--group` | `-g` | — | Group(s) to sync. Repeatable. Format: `name` or `name:p1,p2` |
| `--table` | `-t` | — | Explicit table(s) to sync. Repeatable. Format: `schema.table[:filter][:col1=rule1,col2=rule2]` |
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
| `--buffer-size` | `-bs` | 32 | Per-table prefetch buffer cap in MiB (peak memory on the order of concurrency × this, higher in practice) |
| `--verify` | `-vf` | false | After commit, re-count each table on source and destination; non-zero exit on mismatch (row-count check, not a value/checksum comparison; skipped on `--dry-run`) |
| `--output` | `-o` | text | Output format: `text` or `json`. `json` prints a machine-readable summary to stdout (progress → stderr) for scripting/CI; requires `--skip-confirmation` |

#### Sync strategies

| Strategy | When | Behaviour |
|----------|------|-----------|
| **Upsert** (default) | No flags | Stages rows in a temp table, then `INSERT … ON CONFLICT DO UPDATE` on all non-PK columns |
| **Preserve** | `--preserve` | Same staging, but `ON CONFLICT DO NOTHING` — existing rows are left untouched |
| **Truncate** | `--truncate` | Clears the destination table first (TRUNCATE or DELETE when `--defer-constraints`), then streams directly via COPY |

`--truncate` and `--preserve` are mutually exclusive — passing both is an error.

Upsert and preserve require a primary key on the destination table. Use `--truncate` for tables without one.

#### Safety check

By default, the destination must resolve to `localhost` or `127.0.0.1`. Pass `--no-safety` to allow a remote destination.

#### Interactive confirmation

Without `--skip-confirmation`, the run command prints a summary banner and waits for:
- `yes` — start the sync
- `no` — abort
- `more` — list each table with its strategy and (for truncate) current destination row count

#### Default scope

When neither `--group` nor `--table` is passed, all tables present in both source and destination (minus excludes) are synced.

---

### `pggosync profile sync <name-or-path>`

Runs a sync with a [profile](#profiles) providing the defaults. Accepts the same
flags as `run`; explicit flags override the profile's values and must come
before the profile name.

---

### `pggosync config validate <name-or-path>`

Validates a sync config. Without connection flags it is an offline check: the
YAML must parse, every group needs tables, and scrub rules must be known. With
`--source` and `--dest` it additionally resolves tasks against both databases —
checking that all tables exist and PK requirements are met — and accepts
`--group`, `--table`, `--exclude`, `--truncate`, and `--preserve` so you can
validate under the same strategy you intend to use.

---

### `pggosync tables`

Diffs tables between source and destination, grouped as: both, source-only, destination-only.

```
pggosync tables --source <name> --dest <name>
```

---

### `pggosync version`

Prints the version. Release builds embed `git describe` output via the
makefile; `go install` builds fall back to the module version from Go build info.

---

## Architecture

### Two config layers

**User config** — connection credentials, one YAML file per named connection in `$XDG_CONFIG_DIR/pggosync/`. Managed by `config.UserConfigHandler`.

**Sync config** — a YAML file referenced at runtime via `--config` (by path, or by name searched in `./.pggosync/configs/` then `$XDG_CONFIG_DIR/pggosync/configs/`). Defines named groups of tables with optional WHERE filters and a global exclude list.

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
    │   Each goroutine streams COPY TO STDOUT from source into a bounded SafeBuffer
    │   (--buffer-size cap; the source COPY blocks when full — backpressure)
    │   (scrub rules are applied as SQL expressions inside this source query)
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

Source pre-fetching is concurrent; destination writes are strictly sequential (one task at a time, all inside the same transaction). The `SafeBuffer` type provides a mutex-protected, condition-variable-backed pipe so that a pre-fetch goroutine can write into a buffer while the destination write loop drains it, without locking both sides simultaneously. Each buffer is **bounded** by `--buffer-size` (default 32 MiB): the source `COPY` blocks once its buffer is full and resumes as the write loop drains it, so peak memory stays bounded and independent of table size — on the order of `concurrency × --buffer-size`, though real RSS runs several times higher (`bytes.Buffer` growth plus pgx/COPY driver buffers).

### Package layout

| Package | Responsibility |
|---------|---------------|
| `cmd/` | CLI command definitions using `urfave/cli/v2` |
| `config/` | Connection credentials (`user.go`), sync config parsing (`sync.go`), name-or-path resolution (`resolve.go`), per-file YAML profiles (`profiles.go`), and sync history (`history.go`) |
| `datasource/` | Read-only (`ReaderDataSource`) and read-write (`ReadWriteDatasource`) pgx connection wrappers |
| `db/` | Pure types (`Table`, `Column`, `PrimaryKey`, `Sequence`, `Trigger`) and low-level SQL helpers |
| `opts/` | Argument parsing for `--group name:params` and `--table schema.table[:filter][:col=rule]` |
| `sync/` | `Task`, `TaskResolver`, `TableSync`, `SafeBuffer`, and the top-level `Sync` orchestrator |
| `sync/table/` | Shared-table and exclusion-list filtering |
| `sync/data/` | Data scrubbing rules — maps a rule ID to the SQL expression run on the source |
| `tui/` | Interactive terminal UI built with Bubble Tea and Huh |

---

## Development

### Setup

```bash
# Start the test databases
docker compose up -d

# Verify they're reachable
psql postgresql://postgres:postgres@localhost:5444/postgres -c '\l'
psql postgresql://postgres:postgres@localhost:5445/postgres -c '\l'
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
- Source: `localhost:5444`
- Destination: `localhost:5445`

They skip automatically under `-short`, so `make test-short` runs only the unit tests.

### Resetting test databases

```bash
make reset-docker-databases
```

### Building

```bash
make build   # linux/amd64 + linux/arm64 binaries in ./bin/
```
