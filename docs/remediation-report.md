# pggosync Remediation Report — Tiers 1–3

A consolidated **What / Why** for every fix delivered across the three tiers of the code
review. Ordered by tier, then by severity within each. Deferred items and remaining backlog
are listed at the end.

---

## Tier 1 — Critical (data-safety, silent-success, safety bypass)

### C1 — `Sync` reported success even when COMMIT failed
- **What:** Changed `Sync` to named returns `(res SyncResult, err error)`; the deferred cleanup now
  assigns `err` from `tx.Commit`/`tx.Rollback`, and "Sync complete." only prints after a *successful*
  commit.
- **Why:** With unnamed returns, `return res, nil` computed the return value *before* the deferred
  commit ran. A failing commit was only logged, never returned — so the caller saw `err == nil`,
  printed "Sync complete", and exited 0 while **nothing was committed**. Silent data loss.

### C2 — Shadowed `err :=` in the trigger blocks caused COMMIT instead of ROLLBACK
- **What:** Changed `err :=` to `err =` at the `DisableUserTriggers` / `RestoreUserTriggers` call sites.
- **Why:** The `:=` created a new variable, leaving the outer named `err` at `nil`. The deferred
  cleanup keyed off that outer `err`, so a trigger failure took the **commit** branch while the
  function returned an error — committing partial state (and potentially leaving triggers permanently
  disabled).

### C3 — `--table` always truncated, ignoring `--preserve`/`--truncate`/`--defer-constraints`
- **What:** `tableToTasks` and the no-arg "all tables" fallback now set
  `Truncate`/`Preserve`/`DeferConstraints` from the resolver flags instead of hardcoding
  `Truncate: true` (mirroring `groupToTasks`).
- **Why:** `run --table users --preserve` **truncated anyway** (silent data loss), and the
  confirmation banner printed `Truncate?: false` — actively lying about what would happen.

### C4 — Destination safety-check regex was bypassable
- **What:** Replaced the regex `IsLocalHost` with `net/url` host parsing, comparing `u.Hostname()`
  exactly against `localhost` / `127.0.0.1` / `::1`.
- **Why:** The unanchored regex let `postgres://u:p@localhost.evil.com/db` pass — a remote host
  treated as local, bypassing the `--no-safety` guard — while wrongly rejecting passwordless
  localhost and IPv6 `[::1]`.

### C5 — The TUI never enforced the safety check
- **What:** Added the same localhost guard in the wizard's `startSync`, honored unless the user
  enabled "Disable safety check?".
- **Why:** The `noSafety` toggle was collected, displayed, and saved to profiles — but never
  consulted. The wizard would sync to a production destination regardless.

### C6 — The sync-config TUI builder was completely broken
- **What:** Added `.Key(...)` to every huh field and a `captureForm` that reads values via
  `form.GetString/GetBool`.
- **Why:** Bubble Tea copies the model by value each `Update`, so huh's pointer-bound struct fields
  targeted a stale copy. The builder errored at "Group name cannot be empty" on step 1 and could
  never produce a non-empty YAML.

**Tier 1 tests:** `TestIsLocalHost` (incl. the exact spoof case), `TestTableToTasksHonorsFlags`.

---

## Tier 2 — High (correctness, robustness)

### H1 — Unquoted schema/table identifiers everywhere
- **What:** Added `db.Table.SQLName()` and `db.QuoteIdentifier()` (via `pgx.Identifier.Sanitize`);
  routed all SQL interpolation of table/schema/sequence/trigger/constraint/temp-table names through
  them. Kept `FullName()` for display, logging, and map keys.
- **Why:** Bare `schema.name` broke on mixed-case, reserved-word, or special-character names
  (`"Order"`, `public."my table"`) and was a second-order injection surface.

### H2 — `--table` filter parsing broke on single-quoted colons
- **What:** `splitTableArg` now tracks single **and** double quotes.
- **Why:** Colons inside a SQL string literal (e.g. `created_at > '2024-01-01 12:30:00'`) were
  treated as delimiters, truncating the filter and handing the remainder to the scrub-rule parser
  with a baffling error.

### H3 — `TRUNCATE … CASCADE` silently emptied unrelated tables → made opt-in
- **What:** New `--cascade` flag (wired through CLI, profiles, and TUI, plus `Task.Cascade`). Default
  truncate is now plain `TRUNCATE`, which errors on FK-referenced tables with a hint pointing to
  `--cascade` or `--defer-constraints`.
- **Why:** The implicit `CASCADE` also emptied tables not in the sync set — including ones being
  upserted with `--preserve` — and committed the loss. Now cascading data loss must be explicitly
  requested.

### H4 — `profile sync` dropped the profile's saved table selection
- **What:** Apply `profile.RawTableInput` to `args.Tables` (via `splitProfileTables`) when `--table`
  isn't passed.
- **Why:** A TUI-built profile with explicit tables ran as "all shared tables" — its saved intent was
  silently ignored.

### H5 — Connection leak on post-connect setup failure
- **What:** Both datasource constructors close the connection when `StatusCheck`/`GetTables` fails,
  and return `nil` consistently on error.
- **Why:** Failures after a successful `pgx.Connect` leaked a backend each time; the wizard
  preview/retry path could exhaust `max_connections`.

### H6 — `concurrency <= 0` deadlocked or panicked
- **What:** Clamp `concurrency` to `>= 1` inside `Sync`.
- **Why:** `make(chan, 0)` deadlocked the prefetch launcher; a negative value panicked `make`. The
  CLI clamped, but the TUI could still pass `0`/`-1`.

### H7 — Empty group params substituted an empty string into SQL
- **What:** `ParseGroupArg` returns `nil` (not `[""]`) for no params; added `UnresolvedPlaceholders`,
  and `groupToTasks` now errors clearly when a filter has an unfilled `{N}`.
- **Why:** `strings.Split("", ",")` yields `[""]`, so `--group g` replaced `{1}` with an empty
  string, producing malformed/incorrect WHERE clauses.

**Tier 2 bonus:** fixed the `.ym`→`.yml` config-path typo that was silently failing the **entire
`cmd/tests` integration suite**; fixed a copy-paste error string in `RestoreUserTriggers`.

**Tier 2 tests:** `SQLName`/`QuoteIdentifier`, single-quoted-colon filter cases, `ParseGroupArg` nil
params, `UnresolvedPlaceholders`, cascade-threading in the resolver test; updated integration +
datasource tests for the new `Truncate` signature and `--cascade`.

---

## Tier 3 — Medium (quality, developer experience)

### M1 — Column quoting was a 3-word denylist
- **What:** `GetColumns` and `GetPrimaryKeys` now sanitize every identifier via `db.QuoteIdentifier`;
  removed the `reservedColumnNames` denylist.
- **Why:** The case-sensitive `order/limit/offset` list missed most reserved and all mixed-case
  columns. Quoting PK columns too keeps the `ON CONFLICT` exclusion comparison in `TableSync`
  consistent.

### M3 — Screens rendered collapsed on first display
- **What:** Feed the current `WindowSizeMsg` to each submodel on screen switch, and to the wizard
  before building the profile-launch preview.
- **Why:** Non-menu screens kept `width/height == 0` until a manual terminal resize — e.g. the
  preview built `viewport.New(-4, -6)`.

### M4 — Upsert reported the wrong row count
- **What:** `InsertFromTempTable` returns `RowsAffected()`; `TableSync` reports that instead of the
  staging-COPY count.
- **Why:** The summary/TUI showed the temp-table load count, mislabeling `DO NOTHING` conflicts and
  updates as inserts.

### M5 — Phantom `prefs` connection + hidden dotted names
- **What:** `ListConnections` skips `prefs.yaml` and uses `HasSuffix`/`TrimSuffix` instead of
  `SplitN(".", 2)`.
- **Why:** `prefs.yaml` (same directory) surfaced as a bogus `prefs` connection, and any connection
  name containing a dot was invisible.

### M6 — Corrupt `history.json` silently wiped; no atomic writes
- **What:** Added `atomicWriteFile` (temp file + rename) for connections, prefs, profiles, and
  history; a corrupt history file is renamed to `.corrupt` instead of overwritten.
- **Why:** A crash mid-write could truncate a config file, and a single unreadable history file
  caused the next save to discard all history.

### M7 — Prefetch goroutines leaked / did wasted COPYs on early error
- **What:** Derived a cancellable `prefetchCtx` (cancelled on `Sync` return) and a `select` on the
  semaphore send.
- **Why:** Every early error return left the launcher and N goroutines reading entire tables into
  buffers nobody would drain, with no way to stop them.

### M9 — Editing+renaming a connection orphaned the old one; no delete
- **What:** Added `DeleteConnection`; the TUI now removes the old file on rename and has a `d` delete
  key on the connection list.
- **Why:** Renaming left both old and new entries, and connections (unlike profiles) had no delete
  action at all.

### M10 — Prompt / exit polish
- **What:** Confirmation prompt is case-insensitive (`y`/`n`/`more`), re-prompts on invalid input, and
  hints at `--skip-confirmation` on EOF; `requireConnections` exits 1 on a real IO error (not 0);
  usage help writes to stderr; ctrl+c during a TUI sync now cancels the sync context cleanly (via
  `isRunning`).
- **Why:** A typo aborted the whole run; real IO failures reported success exit codes to scripts;
  help polluted stdout; and ctrl+c tore down the process mid-write instead of cancelling.

**Tier 3 tests:** `TestAtomicWriteFile`, `ListConnections` skip-prefs/keep-dotted.

---

## Post-review follow-ups (delivered after Tiers 1–3)

### M8 — bounded streaming with backpressure
- **What:** `SafeBuffer` is now a bounded blocking buffer: `Write` blocks once the unread length
  reaches the cap and resumes as the reader drains it, and a new `Close()` unblocks a writer parked
  at the cap. `Sync` threads a `bufferCap` (bytes) and, on return, cancels the prefetch context,
  closes every buffer, and waits for all prefetch goroutines. Exposed as `--buffer-size` (MiB,
  default 32), wired through the CLI, profiles, and the TUI.
- **Why:** Each task previously buffered its whole table into an unbounded `bytes.Buffer`, so peak
  memory was `concurrency × table size` — an OOM ceiling on large tables. Peak prefetch memory is
  bounded on the order of `concurrency × --buffer-size`, independent of table size (real RSS runs
  several times higher — `bytes.Buffer` growth plus pgx/COPY driver buffers). `Close()` is required
  because a goroutine parked in the bounded `Write` is outside ctx-aware pgx code, so context
  cancellation alone would leak it on an early-error return. Deadlock-free because the write loop
  `Close()`s each buffer as soon as its drain returns — before moving to the next task — which frees
  that prefetch goroutine's semaphore slot even when the drain returned early, so the launcher can
  always start the next task's prefetch and the loop can never block on a buffer whose producer never
  got a slot.
- **Tests:** `TestSafeBuffer_Bounded`, `TestSafeBuffer_CloseUnblocksWriter` (run under `-race`).

### `--verify` — post-sync row-count check
- **What:** New `sync.Verify` (`sync/verify.go`) runs after `Sync` commits and re-counts each table on
  both databases: truncate tables must match the source exactly, upsert/preserve tables must hold
  `dest >= source` (the destination keeps rows outside the synced slice). A mismatch is a non-zero
  CLI exit (and a red banner on the TUI results screen). Skipped on `--dry-run`. Exposed as `--verify`
  and wired through the CLI, profiles (`Verify`), and the TUI options + results, matching the other
  flags.
- **Why:** Nothing confirmed the sync actually landed every row; a silently short COPY, an
  interfering destination trigger, or a filter mistake would commit and report success. `--verify`
  gives scripts and CI a cheap post-flight assertion.
- **Scope note:** It is deliberately a *row-count* check, not a value/checksum comparison — scrub
  rules make source and destination values differ by design, and non-deterministic rules (random_*)
  would never match, so counts are the only meaningful cross-database invariant. Because it re-queries
  the live source, concurrent writes there can cause a spurious truncate mismatch (best-effort, not a
  transactional guarantee).
- **Tests:** `TestVerifyVerdict` (strategy-aware comparison), `TestVerifyResultOK`; verified
  end-to-end against Docker (truncate exact match, upsert `dest >= source`, and a forced
  trigger-suppressed mismatch → non-zero exit).

### `--output json` — machine-readable summary
- **What:** `run` and `profile sync` accept `--output json` (`cmd/jsonoutput.go`), which prints one
  summary object to stdout — per-table strategy/rows/error, the `--verify` results, `success`,
  `error`, `elapsed_ms` — and routes all human progress to stderr so stdout stays parseable. Emitted
  even on failure (with `error` set); the exit code still reflects success. Requires
  `--skip-confirmation`; rejects any format other than `text`/`json`.
- **Why:** Scripts and CI had no structured way to consume a run's outcome — they had to scrape
  human progress lines. JSON pairs with `--verify` to give a pipeline a single parseable pass/fail
  artifact.
- **Scope note:** Deliberately a per-invocation flag — not persisted in profiles and not surfaced in
  the TUI, where a JSON dump has no meaning.
- **Tests:** `TestPrintJSONSummary_*` (success+verify, sync error, verify failure); verified
  end-to-end against Docker (clean stdout under `jq`, stderr-routed progress, non-zero exit on
  failure, and both input guards).

## Deferred (by decision)

- **M2 — async TUI preview.** The preview currently opens connections and runs a `COUNT(*)` per table
  synchronously inside `Update`, freezing the UI. Proper fix is a `tea.Cmd`; deferred as it needs
  manual TUI verification.

## Remaining backlog / feature suggestions

1. Finish schema sync (`sync/schemasync.go` is a stub) — `--schema-only` / DDL diff.
2. FK-aware task ordering — topological sort so truncate/insert order is correct without always
   needing `--defer-constraints`.
3. Shell completion + `config lint` — validate a sync config against a live schema.
4. Global `--where` / column include-exclude, and named masking presets (GDPR-style scrub bundles).
