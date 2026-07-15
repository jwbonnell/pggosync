# pggosync Guide

pggosync copies data between two PostgreSQL databases: it reads rows from a **source** database (never writing to it) and applies them to a **destination** database inside a single transaction. It is designed for the "give me realistic data locally" workflow — pulling a filtered, optionally anonymised slice of a shared or production-like database into a local one.

This guide covers the design of the configuration system and how the pieces are meant to fit together. The detail-heavy material is split into sub-guides:

- **[Command Reference](guide/commands.md)** — every command, every flag, and the problem each flag solves
- **[Usage Examples & Flag Combinations](guide/usage.md)** — worked examples, typical flag pairings, and combinations that conflict or are ignored
- **[TUI Walkthrough](guide/tui.md)** — a screen-by-screen tour of the interactive terminal UI

---

## Design: the four kinds of configuration

pggosync deliberately splits configuration into four separate layers. Each layer answers a different question and changes at a different rate, which is why they are not one big file:

| Layer | Answers | Stored at | Changes |
|---|---|---|---|
| **Connections** | *Where are the databases and how do I log in?* | `$XDG_CONFIG_DIR/pggosync/<name>.yaml` | Rarely; per machine/user |
| **Sync configs** | *Which tables, filtered and scrubbed how?* | `--config <name-or-path>`, searched in `configs/` dirs | Per project; shareable |
| **Profiles** | *Which exact combination of the above do I run repeatedly?* | one YAML per profile in `profiles/` dirs | Per workflow |
| **Prefs** | *General settings (extra search paths)* | `$XDG_CONFIG_DIR/pggosync/prefs.yaml` | Rarely |

### Connections: credentials, kept out of everything else

A connection is one YAML file per named database:

```yaml
# ~/.config/pggosync/prod-replica.yaml
host: db.internal.example.com
port: 5432
database: app
user: readonly
password: secret
sslmode: require   # optional; omitted/empty means no sslmode in the URL
```

Connections are stored **separately from sync configs and profiles on purpose**:

- **Reuse.** The same `prod-replica` connection can be the source for ten different sync configs and profiles. Change the password once, everything keeps working.
- **Shareability without leaking secrets.** Sync configs and profiles reference connections *by name only*, so they can be committed to a repo (`./.pggosync/`) and shared with a team. Each teammate defines their own `prod-replica` and `local` connections with their own credentials.
- **Machine independence.** A profile that says `source: prod-replica` works on any machine where a connection named `prod-replica` exists, regardless of how that machine reaches the database.

Connections are managed with `pggosync conn` (`init`, `new`, `list`, `get`, `test`) or the TUI's **Manage Connections** screen. Connection files never move between machines through pggosync; they are always local, created per user, with `0600` permissions.

### Sync configs: the *what* of a sync

A sync config is a YAML file describing table selections — named **groups** of tables with optional SQL `WHERE` filters, per-table scrub (anonymisation) rules, per-table strategy overrides, and a global exclude list:

```yaml
description: "Staging refresh"

exclude:               # never synced, regardless of group selection
  - public.audit_log

groups:
  by_tenant:           # parameterised: run with --group by_tenant:42
    tables:
      - table: public.orders
        filter: "tenant_id = {1}"
      - table: public.order_items
        filter: "order_id IN (SELECT id FROM orders WHERE tenant_id = {1})"
        truncate: true          # per-table override of the global flag
        scrub:
          - column: customer_note
            rule: redact
```

The sync config intentionally contains **no credentials and no host names**. It describes data shape, not location — the same config can pull from prod to staging, staging to local, or between two Docker containers, depending on which connections you pair it with at run time.

Key concepts:

- **Groups** bundle related tables so a consistent slice (parent + children) syncs together. `{1}`, `{2}`, … placeholders in filters are substituted from `--group name:param1,param2`, letting one group definition serve any tenant/country/id.
- **Filters** are raw SQL predicates appended to the source `COPY` query — anything valid in a `WHERE` clause works, including subqueries.
- **Scrub rules** map a column to an anonymisation rule (`hash`, `redact`, `null`, `random_int`, `random_float`, `random_email`, `partial[:n]`, `static[:value]`). The rule becomes a SQL expression inside the source-side `COPY TO STDOUT` query, so **raw values never leave the source database**.
- **`exclude`** lists tables that must never sync (audit logs, session tables). It is merged with any `--exclude` flags.
- **Per-table `truncate`/`preserve`** override the global `--truncate`/`--preserve` flags for that one table. Truncate and preserve are mutually exclusive — as flags (passing both is an error) and per table (an entry resolving to both is rejected; disambiguate with an explicit `truncate: false` or `preserve: false`).

Sync configs are managed with `pggosync config` (`list`, `validate`, `paths`) and can be authored by hand or via the TUI's **Build Sync Config** screen.

### Profiles: the *how* of a sync you run repeatedly

A profile is a saved bundle of everything a sync run needs: which connections, which sync config, which groups/tables, and which flags:

```yaml
# ./.pggosync/profiles/nightly-staging.yaml — the profile's name is the filename stem
source: prod-replica       # connection name
dest: local                # connection name
config_file: default       # sync config name or path, resolved like --config
groups: [by_tenant]
truncate: true
defer_constraints: true
concurrency: 4
```

Profiles exist to solve "I keep typing the same nine flags." They **reference** the other two layers by name rather than duplicating them:

- profile → connection names (`source`, `dest`) → resolved against the connections directory at run time
- profile → sync config name (`config_file`) → resolved through the normal config search path

So the relationship is: **a profile is a pointer to two connections + one sync config + a set of flag values.** Editing a sync config immediately affects every profile that references it; renaming a connection breaks profiles that point at it (`pggosync profile validate` catches this).

Run one with `pggosync profile sync <name>`. Any flag you pass explicitly overrides the profile's stored value for that run — the profile provides *defaults*, not mandates. The TUI can save a profile after a successful run (press `p` on the results screen) and launch profiles from the **Manage Profiles** screen.

A legacy `profiles.json` (from older versions) is automatically split into per-file YAML profiles on first load and renamed to `profiles.json.bak`.

### Prefs, and the other files in the user config dir

`$XDG_CONFIG_DIR/pggosync/` (typically `~/.config/pggosync/`) holds:

| File / dir | Purpose |
|---|---|
| `<name>.yaml` | One file per named connection |
| `prefs.yaml` | General settings; currently `include.paths` (extra search directories) |
| `configs/` | User-level sync configs (searchable by bare name) |
| `profiles/` | User-level profiles (where the TUI saves them) |
| `history.json` | Rolling record of the last 20 TUI-run syncs (feeds the menu's "Last sync" line) |
| `profiles.json.bak` | Backup left behind by the legacy-profile migration |

---

## Name resolution and search order

`--config`, `config_file` in a profile, and profile names all accept **either a literal path or a bare name**. Anything containing a `/` or ending in `.yaml`/`.yml` is treated as a path and used verbatim. A bare name is searched as `<dir>/<name>.yaml` (then `.yml`) across the search directories **in order**:

1. `./.pggosync/configs/` or `./.pggosync/profiles/` — **project-local**. Commit these to share with a team.
2. `$XDG_CONFIG_DIR/pggosync/configs/` or `.../profiles/` — **user-level**.
3. Any **include paths** from `prefs.yaml`, in the order they were added. Each include path is a base directory expected to contain a `configs/` and/or `profiles/` subdirectory. Manage them with `pggosync config paths` / `pggosync config paths add <path>`.

The first match wins. In listings (`config list`, `profile list`), earlier directories *shadow* later ones when the same bare name exists in more than one place — the project dir beats the user dir, which beats include paths. There is currently no warning when a name is shadowed, and the precedence is not configurable (tracked as a TODO in `config/resolve.go`).

**Intended usage:** put team-shared, repo-versioned configs in `./.pggosync/`; put personal ones in the user dir; use include paths for a shared directory outside the repo (a dotfiles checkout, a mounted team share).

---

## How a sync actually runs (mental model)

Understanding the execution model explains most flag behavior:

1. **Resolve.** Group/table arguments expand into tasks. Table existence is confirmed on *both* databases, columns/PKs/sequences are loaded, scrub columns are validated (must exist, must not be a PK), strategy conflicts (truncate + preserve on the same task) are rejected, and upsert-mode tables without a destination primary key are rejected up front.
2. **One destination transaction.** Everything — optional constraint deferral, optional trigger disabling, all table writes, sequence updates — happens in a single transaction. Any task failure (or `--dry-run`) rolls the whole thing back; the destination is never left half-synced.
3. **Concurrent reads, sequential writes.** Source tables are pre-fetched concurrently (up to `--concurrency` at once) into bounded in-memory buffers (`--buffer-size`, default 32 MiB each) that apply backpressure when full — peak memory stays independent of table size, on the order of `concurrency × --buffer-size` (higher in practice); the destination is written one table at a time within the transaction.
4. **Two write strategies.** *Truncate*: clear the table (`TRUNCATE`, or `DELETE FROM` under `--defer-constraints`) then `COPY` straight in. *Upsert/preserve* (default): `COPY` into a temp table, then `INSERT … ON CONFLICT DO UPDATE` (or `DO NOTHING` with `--preserve`) — this is why a destination primary key is required.
5. **Sequences follow.** After each table, sequences owned by it are set to the source's values, so inserts on the destination don't collide after the sync.
6. **Safety check.** Before any of this, the destination host must be `localhost` or `127.0.0.1` unless `--no-safety` is passed. The check compares the URL host exactly, so `localhost.evil.com` does not pass. This exists because the tool's whole premise is *pulling data down* — a fat-fingered direction swap should not be able to write into a shared database.

---

## Where to go next

- [Command Reference](guide/commands.md) — `run`, `tables`, `conn`, `profile`, `config`, `version`, and every flag
- [Usage Examples & Flag Combinations](guide/usage.md) — recipes and the compatibility matrix
- [TUI Walkthrough](guide/tui.md) — the interactive interface, screen by screen
