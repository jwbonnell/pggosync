# TUI Walkthrough

Part of the [pggosync Guide](../GUIDE.md). See also: [Command Reference](commands.md) · [Usage Examples](usage.md).

Launch the TUI by running `pggosync` with **no arguments**. It runs full-screen (alt-screen mode) and returns your terminal untouched on exit. `Ctrl+C` quits from anywhere — except while a sync is running, where it cancels the sync first (see [Running](#running)).

The TUI and CLI operate on the same files. Connections created here are usable with `-s`/`-d` on the command line; profiles saved here run with `pggosync profile sync`; sync configs built here work with `--config`.

---

## Main menu

Four entries, navigated with the arrow keys (or `j`/`k`), selected with `Enter` or `Space`; `q` quits:

| Entry | Purpose |
|---|---|
| **Run Sync** | Guided five-step wizard that configures and executes a sync |
| **Manage Connections** | Create, edit, and delete connection credential files |
| **Build Sync Config** | Interactively compose a sync config YAML file |
| **Manage Profiles** | Launch or delete saved profiles |

Below the menu, a **Last sync** line summarises the most recent TUI-run sync (relative time, `source → dest`, table/row counts, or the failure message). It is fed by `history.json` in the user config dir, which records the last 20 TUI-run syncs — dry runs and CLI runs are not recorded. On wide terminals a logo panel fills the right side; on narrow ones it disappears.

---

## Run Sync wizard

Five form steps, then preview → run → results. `Esc` always steps back one screen (from step 1, back to the menu). Validation errors appear in red above the form and keep you on the step.

**Step 1 — Source Connection.** Pick from your saved connections. If none exist, the single option `(none — run pggosync conn init first)` tells you what's missing.

**Step 2 — Destination Connection.** Same picker for the destination.

**Step 3 — Sync Config File.** Free-text field accepting a bare name (searched through the config dirs, e.g. `default`) or a path to a YAML file. Resolution and parsing happen when you submit; failures show the error and let you retype.

**Step 4 — Groups & Tables.** A multi-select of every group in the loaded config (Space toggles), plus an **Additional tables** field for comma-separated table specs (e.g. `public.users,sales.orders`). Leave both empty to sync all shared tables, minus the config's `exclude` list, which is applied exactly as in the CLI. If the config defines no groups, only the tables field is shown.

> **Limitation:** the group picker selects groups by name only — there is no way to supply positional params. Parameterised groups (filters containing `{1}`, …) will fail at the preview step with an "unfilled placeholder" error; run those from the CLI with `--group name:params`.

**Step 5 — Options.** A **Sync strategy** selector — Upsert / Truncate / Preserve — makes the strategy a single choice, so the mutually exclusive truncate and preserve can never both be selected. Below it, toggles and pickers mirroring the remaining CLI flags: cascade truncate, defer FK constraints, disable user triggers, concurrency (1/2/4/8), dry run, and disable safety check. The same semantics and caveats as the flags apply — see the [Command Reference](commands.md#strategy-flags--how-rows-are-written). (A hand-written profile that sets both `truncate` and `preserve` is caught at the preview step and routed back here to pick one.)

### Preview

Submitting the options connects to both databases and fully resolves the plan — the same validation `run` does (tables exist, PKs present, scrub columns valid). You see source/destination details, the effective strategy, and every table with:

- its strategy (`upsert` / `truncate` / `preserve`),
- how many destination rows will be deleted (truncate tables),
- an estimated source row count for the filter,
- any scrub rules.

The list scrolls if long. `Enter` or `y` starts the sync; `Esc` or `b` returns to options. The safety check is enforced here exactly as in the CLI: a non-localhost destination aborts to the results screen unless "Disable safety check?" was enabled.

### Running

Live progress: a spinner with elapsed time, a progress bar (`N / M tables`), and a per-table status list:

| Indicator | Meaning |
|---|---|
| `·` | queued |
| `↓` | prefetching from source |
| `●` | writing to destination |
| `✓` | done (with row count) |
| `✗` | failed (with error) |

A `🔒` badge marks tables with scrub rules. Keys: `j`/`k` move the selection; `d` or `Enter` toggles a **detail panel** (terminals ≥100 columns wide) showing the selected table's status, strategy, row counts, elapsed time, filter, scrub rules, and full error text. A running total of rows and rows/sec appears below the list.

`q` or `Ctrl+C` **cancels the sync**: the context is cancelled and the destination transaction rolls back — the destination is left exactly as it was.

### Results

A summary header (`Sync complete`, `Dry run complete`, or `Sync failed: …`) with total time, and a per-table breakdown of strategy and rows synced (or the failure). Successful non-dry runs are appended to the sync history. Keys:

- `r` — restart the wizard from step 1
- `p` — **save as profile**: prompts for a name (pre-filled `source-dest`) and writes the entire wizard configuration to the user profiles dir, ready for `pggosync profile sync <name>` or the Manage Profiles screen
- `Esc`/`q` — back to the main menu

---

## Manage Connections

A list of all saved connections with masked connection strings, topped by a `(+ New connection)` entry.

| Key | Action |
|---|---|
| `Enter` / `Space` | Edit the selected connection (or create, on the `+` entry) |
| `n` | New connection |
| `d` | **Delete the selected connection — immediately, no confirmation** |
| `Esc` / `q` | Back to the menu |

The form has fields for name, host, port (validated 1–65535), database, user, password (masked as you type), and an sslmode picker (`disable`/`prefer`/`require`/`verify-full`). Creating rejects a name that already exists; editing pre-fills current values, and changing the name renames the file (the old one is removed). This is the same form used by `pggosync conn new`.

---

## Build Sync Config

A looped wizard that assembles a sync config YAML without hand-writing it:

1. **Description & Exclusions** — free-text description plus a multi-line exclude list (one `schema.table` or `table` per line).
2. **Add Group** — name the group.
3. **Add Table** — table name (`schema.table` or bare `table`) and an optional SQL filter (placeholders like `country_id = {1}` are fine — they stay in the file for later param substitution).
4. **Scrub rules** — optionally add column/rule pairs for the table just added; the rule is picked from the supported list. Loop with "Add another scrub rule?".
5. **Finish form** — "Add another table to this group?" loops to 3; otherwise "Add another group?" loops to 2; otherwise enter a **save path** and the YAML is written there.

`Esc` steps backwards, cleanly discarding half-entered groups/tables. The done screen shows a summary of what was written; any key returns to the menu.

> The save path is used verbatim (suggested placeholder: `_configs/my-sync.yml`). To make the config resolvable by *bare name*, save it into `./.pggosync/configs/` or `$XDG_CONFIG_DIR/pggosync/configs/` — otherwise reference it by path.

---

## Manage Profiles

Lists every discoverable profile (project, user, and include dirs) as `name — source → dest · config`.

| Key | Action |
|---|---|
| `Enter` / `Space` | **Launch** the profile: the Run Sync wizard is pre-populated from it and jumps straight to the [Preview](#preview) screen — one keypress from syncing, with a chance to review or step back into the options first |
| `d` | Delete the selected profile (no confirmation). Only removes files from the *user* profiles dir; project-local profile files in the repo are left alone |
| `Esc` / `q` | Back to the menu |

Profiles are created from the wizard's results screen (`p`) or by dropping YAML files into a `profiles/` search dir — see the [main guide](../GUIDE.md#profiles-the-how-of-a-sync-you-run-repeatedly).
