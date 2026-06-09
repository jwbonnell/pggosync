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
)

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
	outputLines  []string
	runViewport  viewport.Model
	syncErr      error
	syncDone     bool
	startTime    time.Time
	syncReader   *bufio.Reader
	cancelSync   context.CancelFunc
	syncResultCh chan sync.SyncResult

	// Phase 8 (results)
	elapsed    time.Duration
	syncResult sync.SyncResult
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
		if m.phase == phaseRunning {
			m.runViewport.Width = msg.Width - 4
			m.runViewport.Height = msg.Height - 6
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
		m.outputLines = append(m.outputLines, string(msg))
		m.runViewport.SetContent(strings.Join(m.outputLines, ""))
		m.runViewport.GotoBottom()
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
	if m.phase == phaseRunning {
		var cmd tea.Cmd
		m.runViewport, cmd = m.runViewport.Update(msg)
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

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("  Source:      %s  (%s:%d/%s)\n", m.selectedSource, srcConn.Host, srcConn.Port, srcConn.Database))
	sb.WriteString(fmt.Sprintf("  Destination: %s  (%s:%d/%s)\n", m.selectedDest, dstConn.Host, dstConn.Port, dstConn.Database))
	sb.WriteString(fmt.Sprintf("  Sync config: %s\n", m.syncConfigPath))
	sb.WriteString("\n")
	sb.WriteString(strategyLine(m.options))
	sb.WriteString(fmt.Sprintf("  Concurrency: %d\n", m.options.concurrency))
	sb.WriteString(fmt.Sprintf("  Dry run:     %v\n", m.options.dryRun))
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
		sb.WriteString(fmt.Sprintf("    %-40s [%s]%s\n", t.FullName(), strategy, rowInfo))
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

// startSync launches the sync in a background goroutine, piping its output into the running viewport via readSyncLine.
func (m syncWizardModel) startSync() (syncWizardModel, tea.Cmd) {
	m.phase = phaseRunning
	m.outputLines = nil
	m.syncDone = false
	m.startTime = time.Now()

	vp := viewport.New(m.width-4, m.height-6)
	m.runViewport = vp

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

	case phaseRunning:
		label := "Syncing"
		if m.options.dryRun {
			label = "Dry run"
		}
		sb.WriteString(wizardTitleStyle.Render(fmt.Sprintf("%s %s...", m.spinner.View(), label)))
		sb.WriteString("\n")
		sb.WriteString(borderStyle.Render(m.runViewport.View()))
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
		sb.WriteString("\n")
		sb.WriteString(helpStyle.Render("r: run again   esc/q: main menu"))
	}

	return docStyle.Render(sb.String())
}
