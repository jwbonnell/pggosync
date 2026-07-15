# Command Reference

Part of the [pggosync Guide](../GUIDE.md). See also: [Usage Examples & Flag Combinations](usage.md) · [TUI Walkthrough](tui.md).

```
pggosync                                 → interactive TUI (bare invocation only)
pggosync run [flags]                     → run a sync
pggosync tables -s <src> -d <dst>        → diff tables between the two databases
pggosync conn init|new|list|get|test     → manage connections
pggosync profile list|show|validate|sync → manage and run profiles
pggosync config list|paths|validate      → manage sync configs
pggosync version                         → print version
```

Unknown subcommands are an error — only a bare `pggosync` opens the TUI.

A general rule for all commands that take a positional argument (`conn get`, `profile sync`, …): **flags must come before the positional argument.** `urfave/cli` stops flag parsing at the first positional arg, so `pggosync profile sync nightly --dry-run` would silently ignore `--dry-run`; pggosync detects trailing arguments and errors instead.

---

## `pggosync run`

Runs a sync. The workhorse command.

```
pggosync run --source <conn> --dest <conn> --config <name-or-path> [flags]
```

`--source`, `--dest`, and `--config` are required. If no connections exist at all, the command exits early with a pointer to `pggosync conn init`.

### Selection flags — *what* gets synced

| Flag | Alias | Purpose |
|---|---|---|
| `--source <name>` | `-s` | Source connection name. Required. |
| `--dest <name>` | `-d` | Destination connection name. Required. |
| `--config <name-or-path>` | `-c` | Sync config, resolved by name through the search dirs or used as a literal path. Required even when only using `--table`, because it also supplies the `exclude` list and group definitions. |
| `--group <name[:p1,p2,…]>` | `-g` | Sync a named group from the config. Repeatable. Positional params substitute `{1}`, `{2}`, … in the group's filters. A group whose filters contain placeholders **must** be given params, otherwise the run is rejected before touching data ("unfilled placeholder"). |
| `--table <spec>` | `-t` | Sync one explicit table, bypassing groups. Repeatable. Format: `schema.table[:filter][:col1=rule1,col2=rule2]` — see below. |
| `--exclude <schema.table>` | `-e` | Skip a table. Repeatable; merged with the sync config's `exclude` list. |

**Default scope:** when neither `--group` nor `--table` is passed, *all tables that exist in both databases* (minus exclusions) are synced.

**`--table` spec format.** Colon-separated segments: table, optional filter, optional scrub rules.

```
--table orders                                        # public schema assumed
--table sales.orders                                  # explicit schema
--table 'sales.orders:tenant_id = 42'                 # with a WHERE filter
--table 'public.users::email=random_email,ssn=redact' # scrub only — note the empty
                                                      # filter segment (::)
--table "orders:created_at > '2024-01-01 12:30:00'"   # colons inside quoted SQL
                                                      # literals are respected
```

The problem each piece solves: the filter lets you take a slice without defining a group; inline scrub rules let you anonymise ad hoc without editing a config file. Explicit `--table` args error if the table is on the exclusion list (rather than silently skipping).

### Strategy flags — *how* rows are written

| Flag | Alias | Problem it solves |
|---|---|---|
| `--truncate` | `-tr` | "I want the destination table to exactly match the source slice." Clears the destination table first, then streams rows straight in with `COPY`. Without it, rows that exist only on the destination survive the sync. Also the only strategy that works on tables **without a primary key**. Clearing uses `TRUNCATE`, or `DELETE FROM` when `--defer-constraints` is active (TRUNCATE cannot run against deferred constraints). |
| `--cascade` | `-ca` | "TRUNCATE fails because other tables have foreign keys into this one." Switches to `TRUNCATE … CASCADE`, which **also empties every referencing table** — including ones you did not list. Only meaningful with `--truncate` and without `--defer-constraints` (the DELETE path ignores it). |
| `--preserve` | `-p` | "Add missing rows but never touch existing ones." Uses `INSERT … ON CONFLICT DO NOTHING` instead of the default `DO UPDATE`. Good for topping up lookup/seed data. |
| *(neither)* | | Default **upsert**: rows are staged in a temp table, then `INSERT … ON CONFLICT DO UPDATE` overwrites all non-PK columns of existing rows and inserts new ones. Requires a destination primary key. |

`--truncate` and `--preserve` are **mutually exclusive** — passing both is an error, checked before anything connects. The same rule applies per table: a config entry that resolves to both strategies (e.g. global `--truncate` plus a per-table `preserve: true` without `truncate: false`) is rejected with instructions to disambiguate.

Upsert and preserve require a primary key on the destination table; the resolver rejects PK-less tables up front with a suggestion to use `--truncate`. Per-table `truncate:`/`preserve:` entries in the sync config override these global flags for individual tables.

### Constraint & trigger flags — making the write possible

| Flag | Alias | Problem it solves |
|---|---|---|
| `--defer-constraints` | `-dc` | "Inserts fail with FK violations because tables load in the wrong order (or a table references itself)." Temporarily makes the destination's non-deferrable FK constraints `DEFERRABLE`, issues `SET CONSTRAINTS ALL DEFERRED` for the transaction, and restores them afterwards. Constraints are then only checked at commit, so intra-sync ordering stops mattering. Side effect: the truncate path clears with `DELETE FROM` instead of `TRUNCATE`. |
| `--disable-triggers` | `-dt` | "User triggers on the destination fire during the sync — audit rows, denormalisation, side effects I don't want." Disables user-defined triggers on the destination for the duration of the transaction and re-enables them before commit. |

### Safety & workflow flags

| Flag | Alias | Problem it solves |
|---|---|---|
| `--no-safety` | `-ns` | By default the destination host must be exactly `localhost` or `127.0.0.1`; anything else aborts. This flag overrides the check when you genuinely mean to write to a remote database (e.g. refreshing a shared staging box). |
| `--skip-confirmation` | `-sc` | Skips the interactive yes/no/more banner. Required for scripts and cron — without it, piped input hits EOF and the run errors with a hint pointing at this flag. |
| `--dry-run` | `-dr` | Does the full sync — connects, prefetches, writes inside the transaction — then **rolls back** instead of committing. Validates the entire pipeline (permissions, filters, PKs, data types) with zero destination changes. Costs real time and I/O. |
| `--quiet` | `-q` | Suppresses per-table progress lines. Combine with `--skip-confirmation` in scripts to keep logs small. |
| `--concurrency <n>` | `-con` | Number of source tables to prefetch in parallel (default 1). Reads overlap with writes, hiding source latency on multi-table syncs. Destination writes are always sequential regardless. Values below 1 are clamped to 1. |
| `--buffer-size <mib>` | `-bs` | Per-table prefetch buffer cap in MiB (default 32). Each prefetch streams into a bounded buffer and blocks (backpressure) once full, so peak memory stays bounded and independent of table size — on the order of `concurrency × buffer-size`, though real RSS runs several times higher (`bytes.Buffer` growth plus pgx/COPY driver buffers). Non-positive values fall back to the default. |
| `--verify` | `-vf` | After the sync **commits**, re-count each table on the source (with its filter) and the destination and fail with a non-zero exit if they don't match. Truncate tables must match exactly; upsert/preserve tables must hold **at least** as many rows on the destination (it retains rows outside the synced slice). This is a **row-count sanity check**, not a value/checksum comparison — scrub rules make source and destination values differ by design, and non-deterministic rules would never match, so counts are the meaningful invariant. Because it re-queries the live source, concurrent writes there can cause a spurious truncate mismatch. Skipped under `--dry-run` (nothing was committed to verify). |
| `--output <fmt>` | `-o` | Output format: `text` (default) or `json`. `json` prints a single machine-readable summary object to **stdout** — per-table strategy/rows/error, the `--verify` results if enabled, `success`, and `elapsed_ms` — and routes all human progress to **stderr** so stdout stays parseable. Emitted even on failure (with `error` set) and the exit code still reflects success. Requires `--skip-confirmation` (the interactive prompt can't share stdout with the JSON). Available on `run` and `profile sync`; it is a per-invocation flag, not stored in profiles. |

### The confirmation prompt

Without `--skip-confirmation`, `run` prints a banner (source, destination, flags, table count) and asks `Do you want to proceed? (yes/no/more)`:

- `yes` / `y` — start
- `no` / `n` — abort cleanly
- `more` / `m` — list every table with its strategy, the number of destination rows that will be deleted (truncate tables), and any scrub rules, then re-prompt

---

## `pggosync profile sync <name-or-path>`

Runs a sync using a saved [profile](../GUIDE.md#profiles-the-how-of-a-sync-you-run-repeatedly) for defaults. Accepts **all the same flags as `run`**; any flag you set explicitly wins over the profile's stored value. Remember: flags go *before* the profile name.

```bash
pggosync profile sync nightly-staging
pggosync profile sync --dry-run nightly-staging      # this run only: no commit
pggosync profile sync -g by_tenant:99 nightly-staging # override the stored groups
```

Resolution rules worth knowing:

- `--group` flags replace the profile's *entire* `groups` list (they don't append).
- `--table` flags replace the profile's stored table input.
- If the profile lacks `source`/`dest`/`config_file` and the corresponding flag isn't passed, the command errors naming the missing piece.

## `pggosync profile list`

Lists every profile discoverable through the search directories (project → user → include paths), one line each: name, `source → dest`, config file.

## `pggosync profile show <name-or-path>`

Prints a profile's full contents as YAML — the quickest way to see exactly which defaults a `profile sync` run would use.

## `pggosync profile validate <name-or-path>`

Checks that the profile is *runnable*: source and dest connections are set and exist, `config_file` is set, resolves through the search path, and parses, and the profile doesn't set both `truncate` and `preserve` (they are mutually exclusive). Exits non-zero listing each problem. Use it after renaming connections or moving config files, or in CI to catch drift in committed profiles.

---

## `pggosync conn init [name]`

Creates connection YAML file(s) with placeholder values for you to edit.

- With a name: creates `<name>.yaml`. Refuses to overwrite an existing connection, so a stray `init` can't clobber saved credentials.
- Without a name: creates the default `source`/`dest` pair (stepping to `source1`/`dest1`, … if taken).

Placeholders default to `localhost`, database `postgres`, sslmode `disable`, and port `5444` — except names starting with `dest` (or named `destination`/`local`), which get `5445`. These match the project's Docker test databases.

## `pggosync conn new`

Interactive form (same as the TUI's connection form) for creating a connection: name (must not already exist), host, port (validated 1–65535), database, user, password (masked input), and an sslmode picker. Aborting with Esc/Ctrl+C prints `Cancelled` and saves nothing.

## `pggosync conn list`

Lists all saved connections with masked connection strings (`postgres://user:***@host:port/db`). Passwords are never printed.

## `pggosync conn get <name>`

Prints one connection config as YAML with the password masked.

## `pggosync conn test <name>`

Actually opens a connection to the database and reports success or the underlying error. Use it to verify credentials before a long sync, or in a health-check script. Exits non-zero on failure.

---

## `pggosync config list`

Lists every sync config discoverable through the search directories, with each file's origin (`[project]`, `[user]`, `[include]`) and path. When the same name exists in multiple dirs, only the winning (earliest) one is shown.

## `pggosync config paths`

Prints the directories searched for configs and profiles, in precedence order. Include paths from `prefs.yaml` are listed last.

## `pggosync config paths add <path>`

Registers an extra base directory to search. The path must exist, be a directory, and contain a `configs/` and/or `profiles/` subdirectory; it is stored as an absolute path in `prefs.yaml`. Adding the same path twice is a no-op. Use this to plug in a shared config repo that lives outside your project (dotfiles, a mounted team share).

## `pggosync config validate <name-or-path>`

Two modes:

**Offline (no flags):** parses the YAML and checks structure — every group has tables, every table entry has a name, no entry sets both `truncate: true` and `preserve: true`, and every scrub rule uses a known rule ID (with valid params, e.g. `partial:0` is rejected). Reports group/table counts.

**Against the databases (`--source` + `--dest`, which must be passed together):** additionally resolves tasks exactly like `run` would — confirming tables exist in both databases, scrub columns exist and aren't PKs, and PK requirements hold — then prints the tables that *would* sync with their strategies. Nothing is written. Supporting flags let you validate under the same conditions you plan to run with:

| Flag | Purpose |
|---|---|
| `--group` / `-g` | Limit validation to specific groups (params work: `-g by_tenant:42`) |
| `--table` / `-t` | Limit validation to specific tables |
| `--exclude` / `-e` | Apply exclusions |
| `--truncate` / `-tr` | Validate as if truncating (relaxes the PK requirement) |
| `--preserve` / `-p` | Validate as if preserving |

This is cheaper than `run --dry-run` (no data is copied) but checks less — dry-run exercises the actual data movement.

---

## `pggosync tables`

```
pggosync tables --source <conn> --dest <conn>
```

Diffs table *presence* between the two databases (both flags required) and prints three sections: **Both**, **Source only**, **Destination only**, plus totals. Since a sync's default scope is "tables in both", this shows exactly what a full sync would cover and which tables would be silently skipped because they're missing on one side. It compares names only — not schemas or row contents.

---

## `pggosync version` (alias `v`)

Prints the version: the `git describe` string embedded by `make build`, falling back to the Go module version for `go install` builds.
