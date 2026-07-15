package tui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/jwbonnell/pggosync/config"
	"github.com/jwbonnell/pggosync/datasource"
	"github.com/jwbonnell/pggosync/opts"
	"github.com/jwbonnell/pggosync/sync"
	"github.com/jwbonnell/pggosync/sync/data"
)

type wizardPhase int

const (
	phasePickSource wizardPhase = iota
	phasePickDest
	phasePickSyncFile
	phasePickGroupsAndTables
	phasePickOptions
	phasePreviewLoading
	phasePreview
	phaseRunning
	phaseResults
	phaseSaveProfile
)

type tablePhase int

const (
	tableQueued tablePhase = iota
	tablePrefetching
	tablePrefetchReady
	tableWriting
	tableDone
	tableFailed
)

type tableProgress struct {
	phase   tablePhase
	rows    int64
	errMsg  string
	elapsed time.Duration
}

type syncOptions struct {
	truncate         bool
	cascade          bool
	preserve         bool
	deferConstraints bool
	disableTriggers  bool
	concurrency      int
	bufferSize       int // per-table prefetch buffer cap in MiB
	dryRun           bool
	noSafety         bool
}

type syncWizardModel struct {
	handler  *config.UserConfigHandler
	phase    wizardPhase
	form     *huh.Form
	width    int
	height   int
	err      string
	spinner  spinner.Model
	progress progress.Model

	// Phases 1 & 2: individual connections
	selectedSource string
	selectedDest   string

	// Phase 3
	syncConfigPath string
	syncConfig     config.SyncConfig

	// Phase 4
	selectedGroups []string
	rawTableInput  string

	// Phase 5
	options        syncOptions
	concurrencyStr string
	bufferSizeStr  string
	strategyStr    string // "upsert" | "truncate" | "preserve" — single-select so truncate and preserve can never both be chosen

	// Phase 6 (preview)
	tasks          []sync.Task
	preview        viewport.Model
	previewContent string
	cancelPreview  context.CancelFunc

	// Phase 7 (running)
	tableStates     []tableProgress
	tableIndex      map[string]int // full name → index in tasks
	tablesCompleted int
	totalRowsSynced int64
	syncErr         error
	syncDone        bool
	startTime       time.Time
	syncReader      *bufio.Reader
	cancelSync      context.CancelFunc
	syncResultCh    chan sync.SyncResult

	// Phase 8 (results)
	elapsed         time.Duration
	syncResult      sync.SyncResult
	savedProfileMsg string

	// Phase 9 (save profile)
	profileNameInput string

	// Detail panel state
	selectedTableIndex int
	showDetailPanel    bool
}

type syncLineMsg string
type syncDoneMsg struct {
	err    error
	result sync.SyncResult
}

// previewResultMsg carries the outcome of the async preview resolution back to the Update loop.
// On success, tasks and content are populated; on failure, err is set and returnPhase names the
// form phase to return to.
type previewResultMsg struct {
	tasks       []sync.Task
	content     string
	err         error
	returnPhase wizardPhase
}

func newSyncWizardModel(handler *config.UserConfigHandler) syncWizardModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(colorPrimary)

	m := syncWizardModel{
		handler:            handler,
		phase:              phasePickSource,
		spinner:            s,
		progress:           progress.New(progress.WithGradient(gradientStart, gradientEnd)),
		options:            syncOptions{concurrency: 1, bufferSize: 32},
		concurrencyStr:     "1",
		bufferSizeStr:      "32",
		selectedTableIndex: 0,
		showDetailPanel:    false,
	}
	m.form = m.buildPickConnectionForm("Source connection", "Which database is the sync source?", &m.selectedSource)
	return m
}

// Init satisfies tea.Model by initialising the first phase's form.
func (m syncWizardModel) Init() tea.Cmd {
	return m.form.Init()
}

// isRunning reports whether a sync is currently in flight, so the top-level model can route ctrl+c
// to this wizard (which cancels the sync's context) instead of quitting the process outright.
func (m syncWizardModel) isRunning() bool {
	return m.phase == phaseRunning
}

// isPreviewLoading reports whether the async preview is resolving, so the top-level model can route
// ctrl+c to this wizard (which cancels the preview) instead of quitting the process.
func (m syncWizardModel) isPreviewLoading() bool {
	return m.phase == phasePreviewLoading
}

// ── Form builders ──────────────────────────────────────────────────────────────

// buildPickConnectionForm creates a dropdown of all saved connections, writing the selection into target.
func (m *syncWizardModel) buildPickConnectionForm(title, desc string, target *string) *huh.Form {
	conns, _ := m.handler.ListConnections()
	options := make([]huh.Option[string], len(conns))
	for i, c := range conns {
		options[i] = huh.NewOption(c, c)
	}
	if len(options) == 0 {
		options = []huh.Option[string]{huh.NewOption("(none — run pggosync conn init first)", "")}
	}
	return newForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Key("conn").
				Title(title).
				Description(desc).
				Options(options...).
				Value(target),
		),
	)
}

// buildSyncFileForm creates the sync config path input form.
func (m *syncWizardModel) buildSyncFileForm() *huh.Form {
	return newForm(
		huh.NewGroup(
			huh.NewInput().
				Key("config").
				Title("Sync config").
				Description("Bare name (searched in configs/ dirs) or a path to a YAML file").
				Placeholder("default").
				Value(&m.syncConfigPath),
		),
	)
}

// buildGroupsForm creates a multi-select of all groups in the loaded sync config plus a free-text table input.
func (m *syncWizardModel) buildGroupsForm() *huh.Form {
	groupKeys := make([]huh.Option[string], 0, len(m.syncConfig.Groups))
	for name := range m.syncConfig.Groups {
		groupKeys = append(groupKeys, huh.NewOption(name, name))
	}
	if len(groupKeys) > 0 {
		return newForm(
			huh.NewGroup(
				huh.NewMultiSelect[string]().
					Key("groups").
					Title("Groups to sync").
					Description("Select groups (empty = all shared tables)").
					Options(groupKeys...).
					Value(&m.selectedGroups),
				huh.NewInput().
					Key("tables").
					Title("Additional tables").
					Description("Comma-separated, e.g. public.users,orders (optional)").
					Value(&m.rawTableInput),
			),
		)
	}
	return newForm(
		huh.NewGroup(
			huh.NewInput().
				Key("tables").
				Title("Tables to sync").
				Description("Comma-separated (empty = all shared tables)").
				Value(&m.rawTableInput),
		),
	)
}

// buildOptionsForm creates the full sync-options form (strategy, constraints, triggers, concurrency, dry-run, safety).
// Strategy is a single select so the mutually exclusive truncate/preserve can never both be chosen.
func (m *syncWizardModel) buildOptionsForm() *huh.Form {
	m.concurrencyStr = fmt.Sprintf("%d", m.options.concurrency)
	m.bufferSizeStr = fmt.Sprintf("%d", m.options.bufferSize)
	m.strategyStr = "upsert"
	if m.options.truncate {
		m.strategyStr = "truncate"
	} else if m.options.preserve {
		m.strategyStr = "preserve"
	}
	return newForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Key("strategy").
				Title("Sync strategy").
				Description("How rows are written to the destination.").
				Options(
					huh.NewOption("Upsert — update existing rows, insert new ones", "upsert"),
					huh.NewOption("Truncate — clear destination tables first", "truncate"),
					huh.NewOption("Preserve — insert new rows only, keep existing", "preserve"),
				).
				Value(&m.strategyStr),
			huh.NewConfirm().
				Key("cascade").
				Title("Cascade truncate?").
				Description("TRUNCATE ... CASCADE also empties tables with an FK to the target. Only used with the Truncate strategy.").
				Value(&m.options.cascade),
			huh.NewConfirm().
				Key("defer").
				Title("Defer FK constraints?").
				Description("Allows out-of-order inserts on the destination.").
				Value(&m.options.deferConstraints),
			huh.NewConfirm().
				Key("triggers").
				Title("Disable user triggers?").
				Description("Disables user triggers on the destination during sync.").
				Value(&m.options.disableTriggers),
			huh.NewSelect[string]().
				Key("concurrency").
				Title("Concurrency").
				Description("Source tables to pre-fetch in parallel.").
				Options(
					huh.NewOption("1", "1"),
					huh.NewOption("2", "2"),
					huh.NewOption("4", "4"),
					huh.NewOption("8", "8"),
				).
				Value(&m.concurrencyStr),
			huh.NewSelect[string]().
				Key("buffersize").
				Title("Buffer size").
				Description("Per-table prefetch cap in MiB (peak memory ≈ concurrency × this).").
				Options(
					huh.NewOption("8 MiB", "8"),
					huh.NewOption("16 MiB", "16"),
					huh.NewOption("32 MiB", "32"),
					huh.NewOption("64 MiB", "64"),
					huh.NewOption("128 MiB", "128"),
				).
				Value(&m.bufferSizeStr),
			huh.NewConfirm().
				Key("dryrun").
				Title("Dry run?").
				Description("Simulate without committing changes.").
				Value(&m.options.dryRun),
			huh.NewConfirm().
				Key("nosafety").
				Title("Disable safety check?").
				Description("Allow syncing to non-localhost destinations.").
				Value(&m.options.noSafety),
		),
	)
}

// ── Update ─────────────────────────────────────────────────────────────────────

// Update handles all wizard phases: form completion, live sync output streaming, spinner ticks, and key bindings.
func (m syncWizardModel) Update(msg tea.Msg) (syncWizardModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Size the progress bar to the terminal, leaving room for the docStyle margin
		// and the "N / M tables" label; clamp so it never collapses or runs off-screen.
		barWidth := msg.Width - 30
		if barWidth < 20 {
			barWidth = 20
		}
		if barWidth > 80 {
			barWidth = 80
		}
		m.progress.Width = barWidth
		if m.phase == phasePreview {
			m.preview.Width = msg.Width - 4
			m.preview.Height = msg.Height - 6
		}

	case tea.KeyMsg:
		switch m.phase {
		case phasePreviewLoading:
			// Cancel the in-flight resolution and drop back to the options form.
			if msg.String() == "esc" || msg.String() == "ctrl+c" {
				if m.cancelPreview != nil {
					m.cancelPreview()
					m.cancelPreview = nil
				}
				m.phase = phasePickOptions
				m.form = m.buildOptionsForm()
				return m, m.form.Init()
			}
			return m, nil
		case phasePreview:
			return m.handlePreviewKey(msg)
		case phaseResults:
			switch msg.String() {
			case "r":
				next := newSyncWizardModel(m.handler)
				return next, next.Init()
			case "esc", "q":
				return m, func() tea.Msg { return switchScreenMsg{screen: menuScreen} }
			case "p":
				m.phase = phaseSaveProfile
				m.profileNameInput = m.selectedSource + "-" + m.selectedDest
				m.form = m.buildSaveProfileForm()
				return m, m.form.Init()
			}
		case phaseRunning:
			if msg.String() == "q" || msg.String() == "ctrl+c" {
				if m.cancelSync != nil {
					m.cancelSync()
				}
				return m, nil
			}
			switch msg.String() {
			case "j", "down":
				if m.selectedTableIndex < len(m.tasks)-1 {
					m.selectedTableIndex++
				}
				return m, nil
			case "k", "up":
				if m.selectedTableIndex > 0 {
					m.selectedTableIndex--
				}
				return m, nil
			case "d", "enter":
				if len(m.tasks) > 0 {
					m.showDetailPanel = !m.showDetailPanel
				}
				return m, nil
			}
		default:
			// Form phases: intercept esc before huh consumes it.
			if msg.String() == "esc" {
				return m.goBack()
			}
		}

	case syncLineMsg:
		m.parseSyncLine(strings.TrimRight(string(msg), "\n"))
		return m, readSyncLine(m.syncReader, m.syncResultCh)

	case previewResultMsg:
		// Ignore results that arrive after the user cancelled/navigated away — the goroutine
		// can't be killed, only its context cancelled, so a stale message can still land.
		if m.phase != phasePreviewLoading {
			return m, nil
		}
		m.cancelPreview = nil
		if msg.err != nil {
			if errors.Is(msg.err, context.Canceled) {
				m.phase = phasePickOptions
				m.form = m.buildOptionsForm()
				return m, m.form.Init()
			}
			m.err = msg.err.Error()
			switch msg.returnPhase {
			case phasePickSource:
				m.phase = phasePickSource
				m.form = m.buildPickConnectionForm("Source connection", "Which database is the sync source?", &m.selectedSource)
			case phasePickDest:
				m.phase = phasePickDest
				m.form = m.buildPickConnectionForm("Destination connection", "Which database is the sync destination?", &m.selectedDest)
			case phasePickSyncFile:
				m.phase = phasePickSyncFile
				m.form = m.buildSyncFileForm()
			default:
				m.phase = phasePickOptions
				m.form = m.buildOptionsForm()
			}
			return m, m.form.Init()
		}
		m.tasks = msg.tasks
		m.err = ""
		m.previewContent = msg.content
		vp := viewport.New(m.width-4, m.height-6)
		vp.SetContent(m.previewContent)
		m.preview = vp
		m.phase = phasePreview
		return m, nil

	case syncDoneMsg:
		m.syncDone = true
		m.syncErr = msg.err
		m.syncResult = msg.result
		m.elapsed = time.Since(m.startTime)
		m.phase = phaseResults
		if !m.options.dryRun {
			_ = m.handler.SaveSyncHistory(m.buildHistoryEntry(msg.err, msg.result))
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	if m.phase == phasePreview {
		var cmd tea.Cmd
		m.preview, cmd = m.preview.Update(msg)
		return m, cmd
	}

	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f
	}
	if m.form.State == huh.StateCompleted {
		return m.advancePhase()
	}
	if m.form.State == huh.StateAborted {
		return m.goBack()
	}
	return m, cmd
}

// handlePreviewKey processes keystrokes on the preview screen: Enter/y starts the sync, Esc/b goes back to options.
func (m syncWizardModel) handlePreviewKey(msg tea.KeyMsg) (syncWizardModel, tea.Cmd) {
	switch msg.String() {
	case "enter", "y":
		return m.startSync()
	case "esc", "b":
		m.phase = phasePickOptions
		m.form = m.buildOptionsForm()
		return m, m.form.Init()
	}
	var cmd tea.Cmd
	m.preview, cmd = m.preview.Update(msg)
	return m, cmd
}

// goBack navigates to the previous wizard phase, rebuilding the appropriate form.
func (m syncWizardModel) goBack() (syncWizardModel, tea.Cmd) {
	switch m.phase {
	case phaseSaveProfile:
		m.phase = phaseResults
		return m, nil
	case phasePickSource:
		return m, func() tea.Msg { return switchScreenMsg{screen: menuScreen} }
	case phasePickDest:
		m.phase = phasePickSource
		m.form = m.buildPickConnectionForm("Source connection", "Which database is the sync source?", &m.selectedSource)
	case phasePickSyncFile:
		m.phase = phasePickDest
		m.form = m.buildPickConnectionForm("Destination connection", "Which database is the sync destination?", &m.selectedDest)
	case phasePickGroupsAndTables:
		m.phase = phasePickSyncFile
		m.form = m.buildSyncFileForm()
	case phasePickOptions:
		m.phase = phasePickGroupsAndTables
		m.form = m.buildGroupsForm()
	default:
		return m, func() tea.Msg { return switchScreenMsg{screen: menuScreen} }
	}
	return m, m.form.Init()
}

// advancePhase moves to the next phase after a form completes, loading the sync config at the file-pick step.
// captureForm copies the just-completed form's values out of the huh form and into
// the model's fields, keyed by the current phase. This is necessary because huh binds
// each field via a pointer captured when the form was built; the model is copied by
// value on every Update (bubbletea) so those bound pointers target a stale copy and the
// struct fields never receive the user's input. The *huh.Form itself is a shared pointer,
// so reading through it (by key) is the reliable source of truth.
func (m *syncWizardModel) captureForm() {
	switch m.phase {
	case phasePickSource:
		m.selectedSource = m.form.GetString("conn")
	case phasePickDest:
		m.selectedDest = m.form.GetString("conn")
	case phasePickSyncFile:
		m.syncConfigPath = m.form.GetString("config")
	case phasePickGroupsAndTables:
		if groups, ok := m.form.Get("groups").([]string); ok {
			m.selectedGroups = groups
		}
		m.rawTableInput = m.form.GetString("tables")
	case phasePickOptions:
		m.strategyStr = m.form.GetString("strategy")
		m.options.truncate = m.strategyStr == "truncate"
		m.options.preserve = m.strategyStr == "preserve"
		m.options.cascade = m.form.GetBool("cascade")
		m.options.deferConstraints = m.form.GetBool("defer")
		m.options.disableTriggers = m.form.GetBool("triggers")
		m.concurrencyStr = m.form.GetString("concurrency")
		m.bufferSizeStr = m.form.GetString("buffersize")
		m.options.dryRun = m.form.GetBool("dryrun")
		m.options.noSafety = m.form.GetBool("nosafety")
	case phaseSaveProfile:
		m.profileNameInput = m.form.GetString("profile")
	}
}

func (m syncWizardModel) advancePhase() (syncWizardModel, tea.Cmd) {
	m.captureForm()
	switch m.phase {
	case phasePickSource:
		m.phase = phasePickDest
		m.form = m.buildPickConnectionForm("Destination connection", "Which database is the sync destination?", &m.selectedDest)
		return m, m.form.Init()

	case phasePickDest:
		m.phase = phasePickSyncFile
		m.form = m.buildSyncFileForm()
		return m, m.form.Init()

	case phasePickSyncFile:
		configPath, err := m.handler.ResolveSyncConfigPath(m.syncConfigPath)
		if err != nil {
			m.err = err.Error()
			m.form = m.buildSyncFileForm()
			return m, m.form.Init()
		}
		sc, err := config.GetSyncConfig(configPath)
		if err != nil {
			m.err = err.Error()
			m.form = m.buildSyncFileForm()
			return m, m.form.Init()
		}
		m.syncConfig = sc
		m.err = ""
		m.phase = phasePickGroupsAndTables
		m.form = m.buildGroupsForm()
		return m, m.form.Init()

	case phasePickGroupsAndTables:
		m.phase = phasePickOptions
		m.form = m.buildOptionsForm()
		return m, m.form.Init()

	case phasePickOptions:
		if _, err := fmt.Sscanf(m.concurrencyStr, "%d", &m.options.concurrency); err != nil {
			m.options.concurrency = 1
		}
		if _, err := fmt.Sscanf(m.bufferSizeStr, "%d", &m.options.bufferSize); err != nil || m.options.bufferSize < 1 {
			m.options.bufferSize = 32
		}
		return m.startPreview()

	case phaseSaveProfile:
		if err := m.handler.SaveProfile(m.toProfile(m.profileNameInput)); err != nil {
			m.err = err.Error()
		} else {
			m.savedProfileMsg = fmt.Sprintf("Saved as profile %q", m.profileNameInput)
		}
		m.phase = phaseResults
		return m, nil
	}
	return m, nil
}

// buildTableArgs splits the comma-separated rawTableInput into individual table arguments.
func (m syncWizardModel) buildTableArgs() []string {
	var args []string
	for _, p := range strings.Split(m.rawTableInput, ",") {
		if p = strings.TrimSpace(p); p != "" {
			args = append(args, p)
		}
	}
	return args
}

// startPreview switches to the loading phase and kicks off the preview resolution off the event
// loop. The actual DB work (connecting, resolving tasks, counting rows) runs in previewCmd so the
// UI stays responsive; the result comes back as a previewResultMsg.
func (m syncWizardModel) startPreview() (syncWizardModel, tea.Cmd) {
	m.phase = phasePreviewLoading
	m.err = ""
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelPreview = cancel
	return m, tea.Batch(m.spinner.Tick, m.previewCmd(ctx))
}

// previewCmd resolves tasks against both databases and renders the summary, returning a
// previewResultMsg. It runs in a goroutine, so it must not mutate the model — everything it needs
// is captured by value. ctx cancels the resolve/count queries when the user aborts the loading
// screen.
func (m syncWizardModel) previewCmd(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		// Honor the sync config's exclude list, mirroring executeSync (cmd/run.go).
		excludedTables, err := opts.ProcessExcludedArgs(m.syncConfig.Exclude)
		if err != nil {
			return previewResultMsg{err: fmt.Errorf("invalid exclude entry in sync config: %w", err), returnPhase: phasePickSyncFile}
		}

		srcConn, err := m.handler.GetConnection(m.selectedSource)
		if err != nil {
			return previewResultMsg{err: err, returnPhase: phasePickSource}
		}
		dstConn, err := m.handler.GetConnection(m.selectedDest)
		if err != nil {
			return previewResultMsg{err: err, returnPhase: phasePickDest}
		}

		source, dest, err := setupWizardDatasources(&srcConn, &dstConn)
		if err != nil {
			return previewResultMsg{err: err, returnPhase: phasePickSource}
		}
		defer func() {
			_ = source.DB.Close(context.Background())
			_ = dest.DB.Close(context.Background())
		}()

		resolver := sync.NewTaskResolver(source, dest, m.syncConfig.Groups,
			m.options.truncate, m.options.cascade, m.options.preserve, m.options.deferConstraints, m.options.disableTriggers, excludedTables)
		tasks, err := resolver.Resolve(ctx, m.selectedGroups, m.buildTableArgs())
		if err != nil {
			return previewResultMsg{err: err, returnPhase: phasePickOptions}
		}

		// Fetch source row counts sequentially (connection is already open). Bail on cancellation
		// so an aborted loading screen doesn't keep counting large tables.
		for i := range tasks {
			count, err := source.GetRowCountFiltered(ctx, tasks[i].SQLName(), tasks[i].Filter)
			if err != nil {
				if ctx.Err() != nil {
					return previewResultMsg{err: ctx.Err(), returnPhase: phasePickOptions}
				}
				continue
			}
			tasks[i].SourceRowCount = count
		}

		content := renderPreviewContent(tasks, m.selectedSource, m.selectedDest, srcConn, dstConn, m.syncConfigPath, m.options)
		return previewResultMsg{tasks: tasks, content: content}
	}
}

// renderPreviewContent builds the confirmation summary shown in the preview viewport. Kept pure
// (no DB access, no model) so it can be unit-tested against a slice of resolved tasks.
func renderPreviewContent(tasks []sync.Task, srcName, dstName string, srcConn, dstConn config.ConnectionConfig, syncConfigPath string, options syncOptions) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("  Source:      %s  (%s:%d/%s)\n", srcName, srcConn.Host, srcConn.Port, srcConn.Database))
	sb.WriteString(fmt.Sprintf("  Destination: %s  (%s:%d/%s)\n", dstName, dstConn.Host, dstConn.Port, dstConn.Database))
	sb.WriteString(fmt.Sprintf("  Sync config: %s\n", syncConfigPath))
	sb.WriteString("\n")
	sb.WriteString(strategyLine(options))
	sb.WriteString(fmt.Sprintf("  Concurrency: %d\n", options.concurrency))
	sb.WriteString(fmt.Sprintf("  Dry run:     %v\n", options.dryRun))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  Tables (%d):\n", len(tasks)))
	for _, t := range tasks {
		strategy := "upsert"
		if t.Truncate && !t.Preserve {
			strategy = "truncate"
		} else if t.Preserve {
			strategy = "preserve"
		}
		rowInfo := ""
		if t.Truncate && !t.Preserve {
			rowInfo = fmt.Sprintf("  (%s dest rows will be deleted)", sync.FormatCount(t.DestRowCount))
		}
		srcInfo := ""
		if t.SourceRowCount > 0 {
			srcInfo = fmt.Sprintf("  ~%s rows", sync.FormatCount(t.SourceRowCount))
		}
		scrubInfo := ""
		if len(t.ScrubRules) > 0 {
			labels := make([]string, len(t.ScrubRules))
			for j, r := range t.ScrubRules {
				labels[j] = fmt.Sprintf("%s=%s", r.Column, data.RuleLabel(r.Rule))
			}
			scrubInfo = fmt.Sprintf("  [%s]", strings.Join(labels, ", "))
		}
		sb.WriteString(fmt.Sprintf("    %-40s [%s]%s%s%s\n", t.FullName(), strategy, rowInfo, srcInfo, scrubInfo))
	}
	sb.WriteString("\n  Press enter to start sync, esc to go back.\n")
	return sb.String()
}

// strategyLine formats the active sync strategy flags as a single display line for the preview screen.
func strategyLine(o syncOptions) string {
	var parts []string
	if o.truncate {
		parts = append(parts, "truncate")
	}
	if o.preserve {
		parts = append(parts, "preserve")
	}
	if o.deferConstraints {
		parts = append(parts, "defer-constraints")
	}
	if o.disableTriggers {
		parts = append(parts, "disable-triggers")
	}
	if len(parts) == 0 {
		parts = []string{"upsert"}
	}
	return fmt.Sprintf("  Strategy:    %s\n", strings.Join(parts, ", "))
}

// startSync launches the sync in a background goroutine, piping its output into the running state via readSyncLine.
func (m syncWizardModel) startSync() (syncWizardModel, tea.Cmd) {
	m.phase = phaseRunning
	m.syncDone = false
	m.tablesCompleted = 0
	m.totalRowsSynced = 0
	m.startTime = time.Now()
	m.selectedTableIndex = 0
	m.showDetailPanel = false

	m.tableStates = make([]tableProgress, len(m.tasks))
	m.tableIndex = make(map[string]int, len(m.tasks))
	for i, t := range m.tasks {
		m.tableIndex[t.FullName()] = i
	}

	srcConn, err := m.handler.GetConnection(m.selectedSource)
	if err != nil {
		m.syncErr = err
		m.phase = phaseResults
		return m, nil
	}
	dstConn, err := m.handler.GetConnection(m.selectedDest)
	if err != nil {
		m.syncErr = err
		m.phase = phaseResults
		return m, nil
	}

	source, dest, err := setupWizardDatasources(&srcConn, &dstConn)
	if err != nil {
		m.syncErr = err
		m.phase = phaseResults
		return m, nil
	}

	// Mirror the CLI safety guard (cmd/run.go): refuse a non-loopback destination unless the
	// user explicitly disabled safety in the options form.
	if !m.options.noSafety && !dest.IsLocalHost(context.Background()) {
		_ = source.DB.Close(context.Background())
		_ = dest.DB.Close(context.Background())
		m.syncErr = fmt.Errorf("destination host %q is not localhost or 127.0.0.1 — enable \"Disable safety check?\" to override", dstConn.Host)
		m.phase = phaseResults
		return m, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancelSync = cancel

	resultCh := make(chan sync.SyncResult, 1)
	m.syncResultCh = resultCh

	pr, pw := io.Pipe()
	reader := bufio.NewReader(pr)
	m.syncReader = reader

	go func() {
		defer cancel()
		defer func() {
			_ = source.DB.Close(context.Background())
			_ = dest.DB.Close(context.Background())
		}()
		result, err := sync.Sync(
			ctx,
			m.options.deferConstraints,
			m.options.disableTriggers,
			false,
			m.options.dryRun,
			m.options.concurrency,
			m.options.bufferSize<<20,
			m.tasks,
			source,
			dest,
			pw,
		)
		resultCh <- result
		_ = pw.CloseWithError(err)
	}()

	return m, tea.Batch(m.spinner.Tick, readSyncLine(reader, resultCh))
}

// readSyncLine returns a Bubble Tea command that reads one line from the sync output pipe and posts it as a syncLineMsg,
// or posts a syncDoneMsg when the pipe closes.
func readSyncLine(r *bufio.Reader, resultCh <-chan sync.SyncResult) tea.Cmd {
	return func() tea.Msg {
		line, err := r.ReadString('\n')
		if len(line) > 0 {
			return syncLineMsg(line)
		}
		if err != nil {
			result := <-resultCh
			return syncDoneMsg{err: unwrapPipeErr(err), result: result}
		}
		return readSyncLine(r, resultCh)()
	}
}

// unwrapPipeErr converts normal pipe-close errors (EOF, ErrClosedPipe) to nil so they aren't shown as failures.
func unwrapPipeErr(err error) error {
	if err == io.EOF || errors.Is(err, io.ErrClosedPipe) {
		return nil
	}
	return err
}

// setupWizardDatasources connects to both databases; closes dest if source fails to avoid a connection leak.
func setupWizardDatasources(src, dst *config.ConnectionConfig) (*datasource.ReaderDataSource, *datasource.ReadWriteDatasource, error) {
	dest, err := datasource.NewReadWriteDataSource("destination", url.URL{
		Scheme:   "postgres",
		Host:     fmt.Sprintf("%s:%d", dst.Host, dst.Port),
		User:     url.UserPassword(dst.User, dst.Password),
		Path:     dst.Database,
		RawQuery: sslmodeQuery(dst.SSLMode),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("destination: %w", err)
	}
	source, err := datasource.NewReadDataSource("source", url.URL{
		Scheme:   "postgres",
		Host:     fmt.Sprintf("%s:%d", src.Host, src.Port),
		User:     url.UserPassword(src.User, src.Password),
		Path:     src.Database,
		RawQuery: sslmodeQuery(src.SSLMode),
	})
	if err != nil {
		_ = dest.DB.Close(context.Background())
		return nil, nil, fmt.Errorf("source: %w", err)
	}
	return source, dest, nil
}

// sslmodeQuery returns a URL query string for the SSL mode, or an empty string when mode is unset.
func sslmodeQuery(mode string) string {
	if mode == "" {
		return ""
	}
	return "sslmode=" + url.QueryEscape(mode)
}

// parseSyncLine updates per-table state based on a line emitted by sync.Sync().
func (m *syncWizardModel) parseSyncLine(line string) {
	switch {
	case strings.HasPrefix(line, "Prefetching "):
		name := strings.TrimSuffix(strings.TrimPrefix(line, "Prefetching "), "...")
		if i, ok := m.tableIndex[name]; ok {
			m.tableStates[i].phase = tablePrefetching
		}
	case strings.HasPrefix(line, "Prefetch ready "):
		name := strings.TrimPrefix(line, "Prefetch ready ")
		if i, ok := m.tableIndex[name]; ok {
			m.tableStates[i].phase = tablePrefetchReady
		}
	case strings.HasPrefix(line, "Syncing "):
		name := strings.TrimSuffix(strings.TrimPrefix(line, "Syncing "), "...")
		if i, ok := m.tableIndex[name]; ok {
			m.tableStates[i].phase = tableWriting
		}
	case strings.HasPrefix(line, "Done "):
		if name, rows, ok := parseDoneLine(line); ok {
			if i, ok2 := m.tableIndex[name]; ok2 {
				m.tableStates[i].phase = tableDone
				m.tableStates[i].rows = rows
				m.tableStates[i].elapsed = time.Since(m.startTime)
				m.tablesCompleted++
				m.totalRowsSynced += rows
			}
		}
	case strings.HasPrefix(line, "Task failed "):
		if name, errMsg, ok := parseFailedLine(line); ok {
			if i, ok2 := m.tableIndex[name]; ok2 {
				m.tableStates[i].phase = tableFailed
				m.tableStates[i].errMsg = errMsg
				m.tablesCompleted++
			}
		}
	}
}

// parseDoneLine parses "Done public.users (1,234 rows)" into table name and row count.
func parseDoneLine(line string) (table string, rows int64, ok bool) {
	// Format: "Done <table> (<count> rows)"
	rest := strings.TrimPrefix(line, "Done ")
	open := strings.LastIndex(rest, " (")
	if open < 0 {
		return "", 0, false
	}
	table = rest[:open]
	inside := strings.TrimSuffix(rest[open+2:], " rows)")
	countStr := strings.ReplaceAll(inside, ",", "")
	n, err := fmt.Sscanf(countStr, "%d", &rows)
	return table, rows, n == 1 && err == nil
}

// parseFailedLine parses "Task failed public.users: <error>" into table name and error message.
func parseFailedLine(line string) (table, errMsg string, ok bool) {
	rest := strings.TrimPrefix(line, "Task failed ")
	colon := strings.Index(rest, ": ")
	if colon < 0 {
		return "", "", false
	}
	return rest[:colon], rest[colon+2:], true
}

// buildSaveProfileForm creates a single-input form asking for the profile name.
func (m *syncWizardModel) buildSaveProfileForm() *huh.Form {
	return newForm(
		huh.NewGroup(
			huh.NewInput().
				Key("profile").
				Title("Profile name").
				Description("Save this configuration for quick re-launch").
				Value(&m.profileNameInput),
		),
	)
}

// toProfile converts the current wizard state into a SyncProfile for persistence.
func (m syncWizardModel) toProfile(name string) config.SyncProfile {
	return config.SyncProfile{
		Name:             name,
		Source:           m.selectedSource,
		Dest:             m.selectedDest,
		ConfigFile:       m.syncConfigPath,
		Groups:           m.selectedGroups,
		RawTableInput:    m.rawTableInput,
		Truncate:         m.options.truncate,
		Cascade:          m.options.cascade,
		Preserve:         m.options.preserve,
		DeferConstraints: m.options.deferConstraints,
		DisableTriggers:  m.options.disableTriggers,
		Concurrency:      m.options.concurrency,
		BufferSize:       m.options.bufferSize,
		DryRun:           m.options.dryRun,
		NoSafety:         m.options.noSafety,
		CreatedAt:        time.Now(),
	}
}

// newSyncWizardModelFromProfile builds a wizard pre-populated from a saved profile, ready to call buildPreview().
func newSyncWizardModelFromProfile(handler *config.UserConfigHandler, p config.SyncProfile) syncWizardModel {
	m := newSyncWizardModel(handler)
	m.selectedSource = p.Source
	m.selectedDest = p.Dest
	m.syncConfigPath = p.ConfigFile
	m.selectedGroups = p.Groups
	m.rawTableInput = p.RawTableInput
	m.options = syncOptions{
		truncate:         p.Truncate,
		cascade:          p.Cascade,
		preserve:         p.Preserve,
		deferConstraints: p.DeferConstraints,
		disableTriggers:  p.DisableTriggers,
		concurrency:      p.Concurrency,
		bufferSize:       p.BufferSize,
		dryRun:           p.DryRun,
		noSafety:         p.NoSafety,
	}
	// Legacy profiles saved before --buffer-size have 0 here; fall back to the 32 MiB default so the
	// buffer stays bounded and the options picker lands on a real choice.
	if m.options.bufferSize < 1 {
		m.options.bufferSize = 32
	}
	if path, err := handler.ResolveSyncConfigPath(p.ConfigFile); err == nil {
		if sc, err := config.GetSyncConfig(path); err == nil {
			m.syncConfig = sc
		}
	}
	m.phase = phasePickOptions
	return m
}

// buildHistoryEntry converts the completed sync result into a config.SyncHistoryEntry for persistence.
func (m syncWizardModel) buildHistoryEntry(syncErr error, result sync.SyncResult) config.SyncHistoryEntry {
	tables := make([]config.TableHistoryEntry, len(result.Tables))
	var totalRows int64
	for i, tr := range result.Tables {
		e := config.TableHistoryEntry{
			Table:    tr.Table,
			Rows:     tr.Rows,
			Strategy: tr.Strategy,
		}
		if tr.Err != nil {
			e.Error = tr.Err.Error()
		}
		tables[i] = e
		totalRows += tr.Rows
	}
	entry := config.SyncHistoryEntry{
		Timestamp:  time.Now(),
		Source:     m.selectedSource,
		Dest:       m.selectedDest,
		ConfigFile: m.syncConfigPath,
		Tables:     tables,
		TotalRows:  totalRows,
		DryRun:     m.options.dryRun,
	}
	if syncErr != nil {
		entry.Error = syncErr.Error()
	}
	return entry
}

// ── View ───────────────────────────────────────────────────────────────────────

// renderDetailPanel renders a bordered detail box for the currently selected table.
func (m syncWizardModel) renderDetailPanel() string {
	if m.selectedTableIndex < 0 || m.selectedTableIndex >= len(m.tasks) {
		return ""
	}

	task := m.tasks[m.selectedTableIndex]
	st := m.tableStates[m.selectedTableIndex]

	var sb strings.Builder
	sb.WriteString(detailTitleStyle.Render(task.FullName()))
	sb.WriteString("\n\n")

	statusLabel := "queued"
	switch st.phase {
	case tableDone:
		statusLabel = "done"
	case tableFailed:
		statusLabel = "failed"
	case tableWriting:
		statusLabel = "writing"
	case tablePrefetching, tablePrefetchReady:
		statusLabel = "prefetching"
	}
	sb.WriteString(detailRow("Status", statusStyle(st.phase).Render(statusLabel)))

	strategy := "upsert"
	if task.Truncate && !task.Preserve {
		strategy = "truncate"
	} else if task.Preserve {
		strategy = "preserve"
	}
	sb.WriteString(detailRow("Strategy", strategy))

	if task.SourceRowCount > 0 {
		sb.WriteString(detailRow("Source rows", sync.FormatCount(task.SourceRowCount)))
	}
	if task.DestRowCount > 0 {
		sb.WriteString(detailRow("Dest rows (before)", sync.FormatCount(task.DestRowCount)))
	}
	if st.rows > 0 {
		sb.WriteString(detailRow("Rows synced", sync.FormatCount(st.rows)))
	}
	if st.elapsed > 0 {
		sb.WriteString(detailRow("Elapsed", st.elapsed.Round(time.Millisecond).String()))
	}
	if task.Filter != "" {
		sb.WriteString(detailRow("Filter", task.Filter))
	}
	if len(task.ScrubRules) > 0 {
		sb.WriteString(detailKeyStyle.Render("Scrub rules:\n"))
		for _, r := range task.ScrubRules {
			sb.WriteString(fmt.Sprintf("    %s → %s\n", detailValueStyle.Render(r.Column), scrubStyle.Render(data.RuleLabel(r.Rule))))
		}
	}
	if st.errMsg != "" {
		sb.WriteString("\n")
		sb.WriteString(detailKeyStyle.Render("Error:\n"))
		sb.WriteString(errorStyle.Render(st.errMsg))
	}

	return detailBorderStyle.Render(sb.String())
}

// detailRow formats a key-value pair for the detail panel.
func detailRow(key, value string) string {
	return fmt.Sprintf("%s  %s\n", detailKeyStyle.Render(key+":"), detailValueStyle.Render(value))
}

// ── View ───────────────────────────────────────────────────────────────────────

// View renders each wizard phase: connection picks, config file, groups/tables, options, preview, running output, and results.
func (m syncWizardModel) View() string {
	var sb strings.Builder

	switch m.phase {
	case phasePickSource:
		sb.WriteString(wizardTitleStyle.Render("Run Sync — Step 1 of 5: Source Connection"))
		sb.WriteString("\n")
		if m.err != "" {
			sb.WriteString(errorStyle.Render("Error: "+m.err) + "\n")
		}
		sb.WriteString(m.form.View())

	case phasePickDest:
		sb.WriteString(wizardTitleStyle.Render("Run Sync — Step 2 of 5: Destination Connection"))
		sb.WriteString("\n")
		if m.err != "" {
			sb.WriteString(errorStyle.Render("Error: "+m.err) + "\n")
		}
		sb.WriteString(m.form.View())

	case phasePickSyncFile:
		sb.WriteString(wizardTitleStyle.Render("Run Sync — Step 3 of 5: Sync Config File"))
		sb.WriteString("\n")
		if m.err != "" {
			sb.WriteString(errorStyle.Render("Error: "+m.err) + "\n")
		}
		sb.WriteString(m.form.View())

	case phasePickGroupsAndTables:
		sb.WriteString(wizardTitleStyle.Render("Run Sync — Step 4 of 5: Groups & Tables"))
		sb.WriteString("\n")
		sb.WriteString(m.form.View())

	case phasePickOptions:
		sb.WriteString(wizardTitleStyle.Render("Run Sync — Step 5 of 5: Options"))
		sb.WriteString("\n")
		if m.err != "" {
			sb.WriteString(errorStyle.Render("Error: "+m.err) + "\n")
		}
		sb.WriteString(m.form.View())

	case phasePreviewLoading:
		sb.WriteString(wizardTitleStyle.Render("Run Sync — Preview"))
		sb.WriteString("\n\n")
		sb.WriteString(fmt.Sprintf("  %s Resolving tables and counting rows…\n", m.spinner.View()))
		sb.WriteString("\n  esc to cancel\n")

	case phasePreview:
		sb.WriteString(wizardTitleStyle.Render("Run Sync — Preview"))
		sb.WriteString("\n")
		sb.WriteString(borderStyle.Render(m.preview.View()))

	case phaseSaveProfile:
		sb.WriteString(wizardTitleStyle.Render("Save Profile"))
		sb.WriteString("\n")
		if m.err != "" {
			sb.WriteString(errorStyle.Render("Error: "+m.err) + "\n")
		}
		sb.WriteString(m.form.View())

	case phaseRunning:
		label := "Syncing"
		if m.options.dryRun {
			label = "Dry run"
		}
		elapsed := time.Since(m.startTime).Round(time.Second)
		header := wizardTitleStyle.Render(fmt.Sprintf("%s %s...  %s", m.spinner.View(), label, elapsed))
		sb.WriteString(header)
		sb.WriteString("\n\n")

		// Progress bar
		total := len(m.tasks)
		percent := 0.0
		if total > 0 {
			percent = float64(m.tablesCompleted) / float64(total)
		}
		progressLine := fmt.Sprintf("  %d / %d tables  %s\n", m.tablesCompleted, total, m.progress.ViewAs(percent))

		// Per-table status list (cap to available height)
		maxRows := m.height - 10
		if m.showDetailPanel && m.width >= 100 {
			maxRows = m.height - 12
		}
		if maxRows < 4 {
			maxRows = 4
		}
		// Scroll the window so the selected row stays visible.
		offset := 0
		if len(m.tasks) > maxRows {
			if m.selectedTableIndex >= maxRows {
				offset = m.selectedTableIndex - maxRows + 1
			}
			if maxOffset := len(m.tasks) - maxRows; offset > maxOffset {
				offset = maxOffset
			}
		}
		end := offset + maxRows
		if end > len(m.tasks) {
			end = len(m.tasks)
		}

		var tableList strings.Builder
		if offset > 0 {
			tableList.WriteString(fmt.Sprintf("  %s\n", helpStyle.Render(fmt.Sprintf("... %d more above", offset))))
		}
		for i := offset; i < end; i++ {
			t := m.tasks[i]
			st := m.tableStates[i]
			var indicatorChar, detail string
			indicatorStyle := statusStyle(st.phase)
			failed := false
			switch st.phase {
			case tableDone:
				indicatorChar = "✓"
				detail = fmt.Sprintf("%s rows", sync.FormatCount(st.rows))
				if t.SourceRowCount > 0 {
					detail += fmt.Sprintf("  (~%s est.)", sync.FormatCount(t.SourceRowCount))
				}
			case tableFailed:
				indicatorChar = "✗"
				detail = "FAILED: " + st.errMsg
				failed = true
			case tableWriting:
				indicatorChar = "●"
				detail = "writing..."
			case tablePrefetching, tablePrefetchReady:
				indicatorChar = "↓"
				detail = "prefetching..."
			default:
				indicatorChar = "·"
				detail = "queued"
			}
			scrubBadge := ""
			if len(t.ScrubRules) > 0 {
				scrubBadge = " 🔒"
			}
			if i == m.selectedTableIndex {
				// Plain text under a single background style; nested ANSI resets would cut the highlight short.
				line := fmt.Sprintf("  %s  %-40s%s  %s", indicatorChar, t.FullName(), scrubBadge, detail)
				tableList.WriteString(selectedRowStyle.Render(line) + "\n")
			} else {
				detailStyled := detail
				if failed {
					detailStyled = errorStyle.Render(detail)
				}
				badgeStyled := scrubBadge
				if scrubBadge != "" {
					badgeStyled = scrubStyle.Render(scrubBadge)
				}
				tableList.WriteString(fmt.Sprintf("  %s  %-40s%s  %s\n", indicatorStyle.Render(indicatorChar), t.FullName(), badgeStyled, detailStyled))
			}
		}
		if end < len(m.tasks) {
			tableList.WriteString(fmt.Sprintf("  %s\n", helpStyle.Render(fmt.Sprintf("... and %d more", len(m.tasks)-end))))
		}

		// Footer stats
		var statsLine string
		if m.totalRowsSynced > 0 && elapsed.Seconds() >= 1 {
			rps := float64(m.totalRowsSynced) / elapsed.Seconds()
			statsLine = fmt.Sprintf("  %s total rows · %s/sec\n", sync.FormatCount(m.totalRowsSynced), sync.FormatCount(int64(rps)))
		} else if m.totalRowsSynced > 0 {
			statsLine = fmt.Sprintf("  %s total rows\n", sync.FormatCount(m.totalRowsSynced))
		}

		helpText := helpStyle.Render("j/k: navigate  d: detail  q: cancel sync")

		// Compose layout
		if m.showDetailPanel && m.width >= 100 && len(m.tasks) > 0 {
			detailPanel := m.renderDetailPanel()
			listWidth := m.width - lipgloss.Width(detailPanel) - 4
			if listWidth < 40 {
				listWidth = 40
			}

			leftPanel := progressLine + "\n" + tableList.String() + "\n" + statsLine
			leftPanel = lipgloss.NewStyle().Width(listWidth).Render(leftPanel)

			joined := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, detailPanel)
			sb.WriteString(joined)
			sb.WriteString("\n")
			sb.WriteString(helpText)
		} else {
			sb.WriteString(progressLine)
			sb.WriteString("\n")
			sb.WriteString(tableList.String())
			if m.totalRowsSynced > 0 {
				sb.WriteString("\n")
				sb.WriteString(statsLine)
			}
			sb.WriteString("\n")
			sb.WriteString(helpText)
		}

	case phaseResults:
		if m.syncErr != nil {
			sb.WriteString(errorStyle.Render(fmt.Sprintf("Sync failed: %v", m.syncErr)))
		} else if m.options.dryRun {
			sb.WriteString(successStyle.Render("Dry run complete"))
		} else {
			sb.WriteString(successStyle.Render("Sync complete"))
		}
		sb.WriteString(fmt.Sprintf("  (%d tables, %s)\n\n", len(m.tasks), m.elapsed.Round(time.Millisecond)))
		if len(m.syncResult.Tables) > 0 {
			var stats strings.Builder
			stats.WriteString(fmt.Sprintf("  %-40s  %-9s  %s\n", "Table", "Strategy", "Rows"))
			stats.WriteString(fmt.Sprintf("  %-40s  %-9s  %s\n", strings.Repeat("─", 40), strings.Repeat("─", 9), strings.Repeat("─", 10)))
			for _, tr := range m.syncResult.Tables {
				if tr.Err != nil {
					stats.WriteString(fmt.Sprintf("  %-40s  %-9s  %s\n", tr.Table, tr.Strategy, errorStyle.Render("FAILED: "+tr.Err.Error())))
				} else {
					stats.WriteString(fmt.Sprintf("  %-40s  %-9s  %s\n", tr.Table, tr.Strategy, sync.FormatCount(tr.Rows)))
				}
			}
			sb.WriteString(borderStyle.Render(stats.String()))
		}
		if m.savedProfileMsg != "" {
			sb.WriteString(successStyle.Render(m.savedProfileMsg) + "\n")
		}
		sb.WriteString("\n")
		sb.WriteString(helpStyle.Render("r: run again   p: save as profile   esc/q: main menu"))
	}

	return docStyle.Render(sb.String())
}
