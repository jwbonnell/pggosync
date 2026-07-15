# Usage Examples & Flag Combinations

Part of the [pggosync Guide](../GUIDE.md). See also: [Command Reference](commands.md) · [TUI Walkthrough](tui.md).

---

## First-time setup

```bash
# 1. Create placeholder connection files, then edit in real credentials
pggosync conn init            # creates "source" and "dest"
$EDITOR ~/.config/pggosync/source.yaml
# …or do it interactively:
pggosync conn new

# 2. Verify both sides are reachable
pggosync conn test source
pggosync conn test dest

# 3. See what a full sync would cover
pggosync tables -s source -d dest

# 4. Write a sync config (see the main guide) and check it
pggosync config validate ./my-sync.yml
pggosync config validate -s source -d dest ./my-sync.yml
```

---

## Recipes

### Pull one tenant's data into a local database

```bash
pggosync run -s prod-replica -d local -c app-slice -g by_tenant:42
```

`by_tenant`'s filters contain `{1}`; `:42` fills it. The default upsert strategy means you can re-run this any time to refresh — existing rows are updated in place, new rows added.

### Full local refresh — destination becomes an exact copy of the slice

```bash
pggosync run -s prod-replica -d local -c app-slice \
  --truncate --defer-constraints
```

`--truncate` clears each table so deleted-on-source rows don't linger locally. `--defer-constraints` makes ordering irrelevant: tables are cleared with `DELETE FROM` and FK checks wait until commit, so parents and children can load in any order. This is the most common "reset my dev database" combination.

### Truncate a table that other tables reference (without deferring)

```bash
pggosync run -s src -d dest -c app-slice -g catalog --truncate --cascade
```

Plain `TRUNCATE` refuses to clear a table that others point FKs at. `--cascade` upgrades it to `TRUNCATE … CASCADE` — but be aware this **also empties the referencing tables**, even ones not in your group. Prefer `--truncate --defer-constraints` when you're syncing the referencing tables in the same run anyway.

### Top up seed/lookup data without touching local edits

```bash
pggosync run -s staging -d local -c app-slice -g lookups --preserve
```

`--preserve` (`ON CONFLICT DO NOTHING`) inserts rows you're missing and leaves every existing row exactly as it is.

### Anonymised copy for a demo

```bash
# Rules in the sync config's scrub: blocks apply automatically; or inline:
pggosync run -s prod-replica -d local -c app-slice \
  --table 'public.users::email=random_email,phone=null,name=partial:2' \
  --table 'public.payments:created_at > now() - interval '"'"'90 days'"'"':card_last4=static:0000'
```

Scrubbing happens inside the source-side `COPY` query — raw values never leave the source. Scrub columns must exist on both sides and cannot be primary-key columns (upsert conflict detection would break); both are validated before any data moves.

### One ad-hoc table, filtered

```bash
pggosync run -s src -d dest -c app-slice \
  -t 'sales.orders:tenant_id = 42 AND status = '"'"'open'"'"''
```

`--config` is still required (it supplies the exclude list), but `--table` bypasses groups entirely. Quoted SQL literals may contain colons safely.

### Unattended / scripted runs

```bash
pggosync profile sync --skip-confirmation --quiet nightly-staging
```

`--skip-confirmation` is mandatory for non-interactive use (the prompt would otherwise fail on EOF); `--quiet` keeps logs to failures and the final result. Profile validation (`pggosync profile validate nightly-staging`) makes a good pre-flight step in the same script.

For CI, add `--output json` to capture a machine-readable result:

```bash
pggosync profile sync --skip-confirmation --output json --verify nightly-staging > result.json
echo "exit=$?"                      # non-zero if the sync or verification failed
jq '.success, .tables[].rows' result.json
```

`--output json` prints one summary object to stdout (per-table rows, the `--verify` outcome, `success`, `elapsed_ms`) and pushes progress to stderr, so redirecting stdout gives you a clean, parseable file. The object is written even on failure (with `error` populated) while the exit code still signals it. It requires `--skip-confirmation`.

### Rehearse a risky sync

```bash
pggosync run -s src -d dest -c app-slice -g big_group --truncate --dry-run
```

Everything executes — clearing, copying, sequence updates — inside the transaction, then rolls back. If the dry run passes, the real run will almost certainly commit. Use `config validate -s … -d …` instead when you only need the cheap structural checks.

### Speed up a many-table sync

```bash
pggosync run -s src -d dest -c app-slice --truncate --defer-constraints \
  --concurrency 4 --skip-confirmation
```

`--concurrency 4` prefetches up to four source tables in parallel while the destination writes sequentially, hiding source round-trip latency. Diminishing returns past the point where prefetch outpaces the write loop. Each prefetch streams into a bounded buffer (`--buffer-size`, default 32 MiB) that applies backpressure when full, so peak memory stays bounded on the order of `concurrency × --buffer-size` (real RSS runs several times higher) — raising concurrency raises the memory ceiling proportionally, but it never grows with table size.

### Confirm the sync landed every row

```bash
pggosync run -s prod-replica -d local -c app-slice --truncate --defer-constraints \
  --verify --skip-confirmation
```

`--verify` re-counts each table on both databases *after the commit* and exits non-zero on a
mismatch — a cheap post-flight check for scripts and CI. Truncate tables must match the source
exactly; upsert/preserve tables must hold at least as many rows on the destination (they keep rows
outside the synced slice). It is a **row-count** check only: it deliberately does not compare column
values, because scrubbed columns differ between source and destination by design. It is skipped
under `--dry-run` (nothing is committed to check).

### Refresh a shared (remote) staging database

```bash
pggosync run -s prod-replica -d staging -c app-slice --truncate --no-safety
```

`--no-safety` is required because `staging` isn't localhost. Double-check `-s`/`-d` before running — the safety check exists precisely to catch a swapped direction, and you are turning it off.

---

## Flags that conflict, ignore each other, or are required together

| Combination | What happens |
|---|---|
| `--truncate` + `--preserve` | **Error — the run is refused.** They select opposite strategies ("replace everything" vs "change nothing existing"). Enforced everywhere: `run`, `profile sync` (including values merged in from the profile), `config validate`, the TUI (whose strategy selector can't even express both), and the task resolver itself as a backstop. Per-table `truncate:`/`preserve:` overrides in the sync config are the supported way to mix strategies in one run — but a single table resolving to both (e.g. global `--truncate` plus a per-table `preserve: true`) is also an error; disambiguate with an explicit `truncate: false` on the entry. |
| `--cascade` without `--truncate` | Ignored. Cascade only modifies the `TRUNCATE` statement. |
| `--cascade` + `--defer-constraints` | Cascade is ignored: with deferred constraints the truncate path clears via `DELETE FROM`, and no `TRUNCATE` is issued. |
| `--source`/`--dest` on `config validate` | Must be passed **together**; one without the other is an error. Omit both for the offline check. |
| `--group`/`--table` params on `profile sync` | Passing any `--group` replaces the profile's whole group list; any `--table` replaces its stored tables. Flags must precede the profile name. |
| Parameterised group without params | `--group by_tenant` when the group's filters contain `{1}` is rejected before the sync starts ("unfilled placeholder"). |
| `--exclude` vs explicit `--table` | Explicitly requesting an excluded table is an error, not a silent skip. |
| `--exclude` vs `--group` | Exclusions do **not** currently filter tables listed inside a selected group — a group syncs all of its tables. Exclusions apply to the "all shared tables" default scope and to explicit `--table` args. |
| `--quiet` without `--skip-confirmation` | Fine interactively, but note `--quiet` doesn't suppress the confirmation prompt — scripts need `--skip-confirmation` regardless. |
| `--concurrency 0` (or negative) | Clamped to 1. |
| `--verify` + `--dry-run` | Verification is skipped — a dry run rolls back, so there is nothing committed to check. A note is printed. |
| `--verify` on an upsert/preserve run | Passes as long as the destination holds **≥** the source row count (it keeps rows outside the synced slice); only truncate tables are checked for an exact match. |
| `--output json` without `--skip-confirmation` | **Error — refused.** The confirmation prompt can't share stdout with the JSON; add `--skip-confirmation`. |
| `--output json` + `--quiet` | Fine; `--quiet` only affects the human progress on stderr — the JSON on stdout is unchanged. |
| Upsert/preserve on a PK-less table | Rejected during resolution with the table names listed; use `--truncate` for those tables (or a per-table `truncate: true` in the config). |

### Choosing a strategy at a glance

| You want | Use |
|---|---|
| Destination slice identical to source slice | `--truncate` (+ `--defer-constraints` for FK-heavy schemas) |
| Update existing + add new rows | default (upsert) |
| Add new rows only, never modify | `--preserve` |
| Table has no primary key | `--truncate` (only option) |
| FK ordering problems / self-referencing FKs | add `--defer-constraints` |
| Destination triggers causing side effects | add `--disable-triggers` |
