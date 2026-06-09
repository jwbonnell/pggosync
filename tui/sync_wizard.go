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

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/jwbonnell/pggosync/config"
	"github.com/jwbonnell/pggosync/datasource"
	"github.com/jwbonnell/pggosync/sync"
)

type wizardPhase int

const (
	phasePickSource wizardPhase = iota
	phasePickDest
	phasePickSyncFile
	phasePickGroupsAndTables
	phasePickOptions
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
	preserve         bool
	deferConstraints bool
	disableTriggers  bool
	concurrency      int
	dryRun           bool
	noSafety         bool
}

type syncWizardModel struct {
	handler *config.UserConfigHandler
	phase   wizardPhase
	form    *huh.Form
	width   int
	height  int
	err     string
	spinner spinner.Model

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

	// Phase 6 (preview)
	tasks          []sync.Task
	preview        viewport.Model
	previewContent string

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
}

type syncLineMsg string
type syncDoneMsg struct {
	err    error
	result sync.SyncResult
}

func newSyncWizardModel(handler *config.UserConfigHandler) syncWizardModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	m := syncWizardModel{
		handler:        handler,
		phase:          phasePickSource,
		spinner:        s,
		options:        syncOptions{concurrency: 1},
		concurrencyStr: "1",
	}
	m.form = m.buildPickConnectionForm("Source connection", "Which database is the sync source?", &m.selectedSource)
	return m
}

// Init satisfies tea.Model by initialising the first phase's form.
func (m syncWizardModel) Init() tea.Cmd {
	return m.form.Init()
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
		options = []huh.Option[string]{huh.NewOption("(none — run pggosync init first)", "")}
	}
	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Description(desc).
				Options(options...).
				Value(target),
		),
	)
}

// buildSyncFileForm creates the sync config path input form.
func (m *syncWizardModel) buildSyncFileForm() *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Sync config path").
				Description("Path to the sync config YAML file").
				Placeholder("_configs/default.yml").
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
		return huh.NewForm(
			huh.NewGroup(
				huh.NewMultiSelect[string]().
					Title("Groups to sync").
					Description("Select groups (empty = all shared tables)").
					Options(groupKeys...).
					Value(&m.selectedGroups),
				huh.NewInput().
					Title("Additional tables").
					Description("Comma-separated, e.g. public.users,orders (optional)").
					Value(&m.rawTableInput),
			),
		)
	}
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Tables to sync").
				Description("Comma-separated (empty = all shared tables)").
				Value(&m.rawTableInput),
		),
	)
}

// buildOptionsForm creates the full sync-options form (truncate, preserve, constraints, triggers, concurrency, dry-run, safety).
func (m *syncWizardModel) buildOptionsForm() *huh.Form {
	m.concurrencyStr = fmt.Sprintf("%d", m.options.concurrency)
	return huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Truncate destination tables?").
				Description("Clears destination tables before syncing. Mutually exclusive with Preserve.").
				Value(&m.options.truncate),
			huh.NewConfirm().
				Title("Preserve existing data?").
				Description("INSERT ... ON CONFLICT DO NOTHING. Ignored when Truncate is enabled.").
				Value(&m.options.preserve),
			huh.NewConfirm().
				Title("Defer FK constraints?").
				Description("Allows out-of-order inserts on the destination.").
				Value(&m.options.deferConstraints),
			huh.NewConfirm().
				Title("Disable user triggers?").
				Description("Disables user triggers on the destination during sync.").
				Value(&m.options.disableTriggers),
			huh.NewSelect[string]().
				Title("Concurrency").
				Description("Source tables to pre-fetch in parallel.").
				Options(
					huh.NewOption("1", "1"),
					huh.NewOption("2", "2"),
					huh.NewOption("4", "4"),
					huh.NewOption("8", "8"),
				).
				Value(&m.concurrencyStr),
			huh.NewConfirm().
				Title("Dry run?").
				Description("Simulate without committing changes.").
				Value(&m.options.dryRun),
			huh.NewConfirm().
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
		if m.phase == phasePreview {
			m.preview.Width = msg.Width - 4
			m.preview.Height = msg.Height - 6
		}

	case tea.KeyMsg:
		switch m.phase {
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
func (m syncWizardModel) advancePhase() (syncWizardModel, tea.Cmd) {
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
		sc, err := config.GetSyncConfig(m.syncConfigPath)
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
		return m.buildPreview()

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

// buildPreview resolves tasks against both databases and renders a summary viewport for the user to confirm.
func (m syncWizardModel) buildPreview() (syncWizardModel, tea.Cmd) {
	srcConn, err := m.handler.GetConnection(m.selectedSource)
	if err != nil {
		m.err = err.Error()
		m.phase = phasePickSource
		m.form = m.buildPickConnectionForm("Source connection", "Which database is the sync source?", &m.selectedSource)
		return m, m.form.Init()
	}
	dstConn, err := m.handler.GetConnection(m.selectedDest)
	if err != nil {
		m.err = err.Error()
		m.phase = phasePickDest
		m.form = m.buildPickConnectionForm("Destination connection", "Which database is the sync destination?", &m.selectedDest)
		return m, m.form.Init()
	}

	source, dest, err := setupWizardDatasources(&srcConn, &dstConn)
	if err != nil {
		m.err = err.Error()
		m.phase = phasePickSource
		m.form = m.buildPickConnectionForm("Source connection", "Which database is the sync source?", &m.selectedSource)
		return m, m.form.Init()
	}
	defer func() {
		_ = source.DB.Close(context.Background())
		_ = dest.DB.Close(context.Background())
	}()

	resolver := sync.NewTaskResolver(source, dest, m.syncConfig.Groups,
		m.options.truncate, m.options.preserve, m.options.deferConstraints, m.options.disableTriggers, nil)
	tasks, err := resolver.Resolve(context.Background(), m.selectedGroups, m.buildTableArgs())
	if err != nil {
		m.err = err.Error()
		m.phase = phasePickOptions
		m.form = m.buildOptionsForm()
		return m, m.form.Init()
	}
	m.tasks = tasks
	m.err = ""

	// Fetch source row counts sequentially (connection is already open).
	for i := range m.tasks {
		count, err := source.GetRowCountFiltered(context.Background(), m.tasks[i].FullName(), m.tasks[i].Filter)
		if err == nil {
			m.tasks[i].SourceRowCount = count
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("  Source:      %s  (%s:%d/%s)\n", m.selectedSource, srcConn.Host, srcConn.Port, srcConn.Database))
	sb.WriteString(fmt.Sprintf("  Destination: %s  (%s:%d/%s)\n", m.selectedDest, dstConn.Host, dstConn.Port, dstConn.Database))
	sb.WriteString(fmt.Sprintf("  Sync config: %s\n", m.syncConfigPath))
	sb.WriteString("\n")
	sb.WriteString(strategyLine(m.options))
	sb.WriteString(fmt.Sprintf("  Concurrency: %d\n", m.options.concurrency))
	sb.WriteString(fmt.Sprintf("  Dry run:     %v\n", m.options.dryRun))
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  Tables (%d):\n", len(m.tasks)))
	for _, t := range m.tasks {
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
		sb.WriteString(fmt.Sprintf("    %-40s [%s]%s%s\n", t.FullName(), strategy, rowInfo, srcInfo))
	}
	sb.WriteString("\n  Press enter to start sync, esc to go back.\n")
	m.previewContent = sb.String()

	vp := viewport.New(m.width-4, m.height-6)
	vp.SetContent(m.previewContent)
	m.preview = vp
	m.phase = phasePreview
	return m, nil
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
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
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
		Preserve:         m.options.preserve,
		DeferConstraints: m.options.deferConstraints,
		DisableTriggers:  m.options.disableTriggers,
		Concurrency:      m.options.concurrency,
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
		preserve:         p.Preserve,
		deferConstraints: p.DeferConstraints,
		disableTriggers:  p.DisableTriggers,
		concurrency:      p.Concurrency,
		dryRun:           p.DryRun,
		noSafety:         p.NoSafety,
	}
	if sc, err := config.GetSyncConfig(p.ConfigFile); err == nil {
		m.syncConfig = sc
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

var (
	wizardTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).MarginBottom(1)
	errorStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).MarginTop(1)
	helpStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).MarginTop(1)
	successStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	borderStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1).Margin(1, 2)
)

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
		sb.WriteString(wizardTitleStyle.Render(fmt.Sprintf("%s %s...  %s", m.spinner.View(), label, elapsed)))
		sb.WriteString("\n\n")

		// Progress bar
		total := len(m.tasks)
		barWidth := 20
		filled := 0
		if total > 0 {
			filled = m.tablesCompleted * barWidth / total
		}
		bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
		sb.WriteString(fmt.Sprintf("  %d / %d tables  %s\n\n", m.tablesCompleted, total, bar))

		// Per-table status list (cap to available height)
		maxRows := m.height - 10
		if maxRows < 4 {
			maxRows = 4
		}
		shown := m.tasks
		if len(shown) > maxRows {
			shown = shown[:maxRows]
		}
		for i, t := range shown {
			var indicator, detail string
			st := m.tableStates[i]
			switch st.phase {
			case tableDone:
				indicator = successStyle.Render("✓")
				detail = fmt.Sprintf("%s rows", sync.FormatCount(st.rows))
				if t.SourceRowCount > 0 {
					detail += fmt.Sprintf("  (~%s est.)", sync.FormatCount(t.SourceRowCount))
				}
			case tableFailed:
				indicator = errorStyle.Render("✗")
				detail = errorStyle.Render("FAILED: " + st.errMsg)
			case tableWriting:
				indicator = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render("●")
				detail = "writing..."
			case tablePrefetching, tablePrefetchReady:
				indicator = lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Render("↓")
				detail = "prefetching..."
			default:
				indicator = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("·")
				detail = "queued"
			}
			sb.WriteString(fmt.Sprintf("  %s  %-40s  %s\n", indicator, t.FullName(), detail))
		}
		if len(m.tasks) > maxRows {
			sb.WriteString(fmt.Sprintf("  %s\n", helpStyle.Render(fmt.Sprintf("... and %d more", len(m.tasks)-maxRows))))
		}

		// Footer stats
		sb.WriteString("\n")
		if m.totalRowsSynced > 0 && elapsed.Seconds() >= 1 {
			rps := float64(m.totalRowsSynced) / elapsed.Seconds()
			sb.WriteString(fmt.Sprintf("  %s total rows · %s/sec\n", sync.FormatCount(m.totalRowsSynced), sync.FormatCount(int64(rps))))
		} else if m.totalRowsSynced > 0 {
			sb.WriteString(fmt.Sprintf("  %s total rows\n", sync.FormatCount(m.totalRowsSynced)))
		}
		sb.WriteString("\n")
		sb.WriteString(helpStyle.Render("q: cancel sync"))

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
