package tui

import (
	"bufio"
	"context"
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
	phasePickConfig wizardPhase = iota
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

	// Phase 1
	selectedConfig string
	configs        []string

	// Phase 2
	syncConfigPath string
	syncConfig     config.SyncConfig

	// Phase 3
	selectedGroups []string
	rawTableInput  string

	// Phase 4
	options        syncOptions
	concurrencyStr string

	// Phase 5 (preview)
	tasks          []sync.Task
	preview        viewport.Model
	previewContent string
	confirmed      bool

	// Phase 6 (running)
	outputLines []string
	runViewport viewport.Model
	syncErr     error
	syncDone    bool
	startTime   time.Time
	syncReader  *bufio.Reader

	// Phase 7 (results)
	elapsed time.Duration
}

type syncLineMsg string
type syncDoneMsg struct{ err error }

func newSyncWizardModel(handler *config.UserConfigHandler) syncWizardModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	m := syncWizardModel{
		handler:        handler,
		phase:          phasePickConfig,
		spinner:        s,
		options:        syncOptions{concurrency: 1},
		concurrencyStr: "1",
	}
	m.form = m.buildPhase1Form(nil)
	return m
}

func (m syncWizardModel) Init() tea.Cmd {
	configs, _ := m.handler.ListConfigs()
	m.configs = configs
	return m.form.Init()
}

// ── Form builders ──────────────────────────────────────────────────────────────

func (m *syncWizardModel) buildPhase1Form(configs []string) *huh.Form {
	if configs == nil {
		configs, _ = m.handler.ListConfigs()
	}
	m.configs = configs

	options := make([]huh.Option[string], len(configs))
	for i, c := range configs {
		options[i] = huh.NewOption(c, c)
	}
	if len(options) == 0 {
		options = []huh.Option[string]{huh.NewOption("(no configs — run pggosync init first)", "")}
	}

	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Connection config").
				Description("Which saved connection config should be used?").
				Options(options...).
				Value(&m.selectedConfig),
		),
	)
}

func (m *syncWizardModel) buildPhase2Form() *huh.Form {
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

func (m *syncWizardModel) buildPhase3Form() *huh.Form {
	groupKeys := make([]huh.Option[string], 0, len(m.syncConfig.Groups))
	for name := range m.syncConfig.Groups {
		groupKeys = append(groupKeys, huh.NewOption(name, name))
	}

	if len(groupKeys) > 0 {
		return huh.NewForm(
			huh.NewGroup(
				huh.NewMultiSelect[string]().
					Title("Groups to sync").
					Description("Select groups from the sync config (leave empty to sync all shared tables)").
					Options(groupKeys...).
					Value(&m.selectedGroups),
				huh.NewInput().
					Title("Additional tables").
					Description("Comma-separated extra tables, e.g. public.users,orders (optional)").
					Value(&m.rawTableInput),
			),
		)
	}

	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Tables to sync").
				Description("Comma-separated tables, e.g. public.users,orders (empty = all shared tables)").
				Value(&m.rawTableInput),
		),
	)
}

func (m *syncWizardModel) buildPhase4Form() *huh.Form {
	concurrencyOptions := []huh.Option[string]{
		huh.NewOption("1", "1"),
		huh.NewOption("2", "2"),
		huh.NewOption("4", "4"),
		huh.NewOption("8", "8"),
	}
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
				Description("Number of source tables to pre-fetch in parallel.").
				Options(concurrencyOptions...).
				Value(&m.concurrencyStr),
			huh.NewConfirm().
				Title("Dry run?").
				Description("Simulate the sync without committing any changes.").
				Value(&m.options.dryRun),
			huh.NewConfirm().
				Title("Disable safety check?").
				Description("Allow syncing to non-localhost destinations.").
				Value(&m.options.noSafety),
		),
	)
}

// ── Update ─────────────────────────────────────────────────────────────────────

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
			// no key handling during sync
		}

	case syncLineMsg:
		m.outputLines = append(m.outputLines, string(msg))
		m.runViewport.SetContent(strings.Join(m.outputLines, ""))
		m.runViewport.GotoBottom()
		return m, readSyncLine(m.syncReader)

	case syncDoneMsg:
		m.syncDone = true
		m.syncErr = msg.err
		m.elapsed = time.Since(m.startTime)
		m.phase = phaseResults
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

	// Delegate to huh form
	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f
	}

	if m.form.State == huh.StateCompleted {
		return m.advancePhase()
	}
	if m.form.State == huh.StateAborted {
		return m, func() tea.Msg { return switchScreenMsg{screen: menuScreen} }
	}

	return m, cmd
}

func (m syncWizardModel) handlePreviewKey(msg tea.KeyMsg) (syncWizardModel, tea.Cmd) {
	switch msg.String() {
	case "enter", "y":
		return m.startSync()
	case "esc", "b":
		m.phase = phasePickOptions
		m.form = m.buildPhase4Form()
		return m, m.form.Init()
	}
	var cmd tea.Cmd
	m.preview, cmd = m.preview.Update(msg)
	return m, cmd
}

func (m syncWizardModel) advancePhase() (syncWizardModel, tea.Cmd) {
	switch m.phase {
	case phasePickConfig:
		m.phase = phasePickSyncFile
		m.form = m.buildPhase2Form()
		return m, m.form.Init()

	case phasePickSyncFile:
		sc, err := config.GetSyncConfig(m.syncConfigPath)
		if err != nil {
			m.err = err.Error()
			m.phase = phasePickSyncFile
			m.form = m.buildPhase2Form()
			return m, m.form.Init()
		}
		m.syncConfig = sc
		m.err = ""
		m.phase = phasePickGroupsAndTables
		m.form = m.buildPhase3Form()
		return m, m.form.Init()

	case phasePickGroupsAndTables:
		m.phase = phasePickOptions
		m.form = m.buildPhase4Form()
		return m, m.form.Init()

	case phasePickOptions:
		if c, err := fmt.Sscanf(m.concurrencyStr, "%d", &m.options.concurrency); c != 1 || err != nil {
			m.options.concurrency = 1
		}
		return m.buildPreview()
	}
	return m, nil
}

func (m syncWizardModel) buildTableArgs() []string {
	if m.rawTableInput == "" {
		return nil
	}
	parts := strings.Split(m.rawTableInput, ",")
	var args []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			args = append(args, p)
		}
	}
	return args
}

func (m syncWizardModel) buildPreview() (syncWizardModel, tea.Cmd) {
	uc, err := m.handler.GetConfig(m.selectedConfig)
	if err != nil {
		m.err = err.Error()
		m.phase = phasePickConfig
		m.form = m.buildPhase1Form(m.configs)
		return m, m.form.Init()
	}

	source, dest, err := setupWizardDatasources(&uc)
	if err != nil {
		m.err = err.Error()
		m.phase = phasePickConfig
		m.form = m.buildPhase1Form(m.configs)
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
		m.form = m.buildPhase4Form()
		return m, m.form.Init()
	}
	m.tasks = tasks
	m.err = ""

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("  Config:      %s\n", m.selectedConfig))
	sb.WriteString(fmt.Sprintf("  Source:      %s:%s/%s\n", uc.Source.Host, uc.Source.Port, uc.Source.Database))
	sb.WriteString(fmt.Sprintf("  Destination: %s:%s/%s\n", uc.Destination.Host, uc.Destination.Port, uc.Destination.Database))
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

func (m syncWizardModel) startSync() (syncWizardModel, tea.Cmd) {
	m.phase = phaseRunning
	m.outputLines = nil
	m.syncDone = false
	m.startTime = time.Now()

	vp := viewport.New(m.width-4, m.height-6)
	m.runViewport = vp

	pr, pw := io.Pipe()
	reader := bufio.NewReader(pr)
	m.syncReader = reader

	uc, err := m.handler.GetConfig(m.selectedConfig)
	if err != nil {
		_ = pw.Close()
		return m, nil
	}
	source, dest, err := setupWizardDatasources(&uc)
	if err != nil {
		_ = pw.Close()
		return m, nil
	}

	go func() {
		defer func() {
			_ = source.DB.Close(context.Background())
			_ = dest.DB.Close(context.Background())
		}()
		err := sync.Sync(
			context.Background(),
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
		_ = pw.CloseWithError(err)
	}()

	return m, tea.Batch(m.spinner.Tick, readSyncLine(reader))
}

func readSyncLine(r *bufio.Reader) tea.Cmd {
	return func() tea.Msg {
		line, err := r.ReadString('\n')
		if len(line) > 0 {
			// Return the line; if the pipe also closed, we'll detect it on the next read.
			return syncLineMsg(line)
		}
		if err != nil {
			return syncDoneMsg{err: unwrapPipeErr(err)}
		}
		return readSyncLine(r)()
	}
}

func unwrapPipeErr(err error) error {
	if err == io.EOF || err == io.ErrClosedPipe {
		return nil
	}
	return err
}

func setupWizardDatasources(c *config.UserConfig) (*datasource.ReaderDataSource, *datasource.ReadWriteDatasource, error) {
	dest, err := datasource.NewReadWriteDataSource("destination", url.URL{
		Scheme: "postgres",
		Host:   fmt.Sprintf("%s:%s", c.Destination.Host, c.Destination.Port),
		User:   url.UserPassword(c.Destination.User, c.Destination.Password),
		Path:   c.Destination.Database,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("destination: %w", err)
	}
	source, err := datasource.NewReadDataSource("source", url.URL{
		Scheme: "postgres",
		Host:   fmt.Sprintf("%s:%s", c.Source.Host, c.Source.Port),
		User:   url.UserPassword(c.Source.User, c.Source.Password),
		Path:   c.Source.Database,
	})
	if err != nil {
		_ = dest.DB.Close(context.Background())
		return nil, nil, fmt.Errorf("source: %w", err)
	}
	return source, dest, nil
}

// ── View ───────────────────────────────────────────────────────────────────────

var (
	wizardTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).MarginBottom(1)
	errorStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).MarginTop(1)
	helpStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).MarginTop(1)
	successStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	borderStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1).Margin(1, 2)
)

func (m syncWizardModel) View() string {
	var sb strings.Builder

	switch m.phase {
	case phasePickConfig:
		sb.WriteString(wizardTitleStyle.Render("Run Sync — Step 1 of 4: Connection Config"))
		sb.WriteString("\n")
		if m.err != "" {
			sb.WriteString(errorStyle.Render("Error: "+m.err) + "\n")
		}
		sb.WriteString(m.form.View())

	case phasePickSyncFile:
		sb.WriteString(wizardTitleStyle.Render("Run Sync — Step 2 of 4: Sync Config File"))
		sb.WriteString("\n")
		if m.err != "" {
			sb.WriteString(errorStyle.Render("Error: "+m.err) + "\n")
		}
		sb.WriteString(m.form.View())

	case phasePickGroupsAndTables:
		sb.WriteString(wizardTitleStyle.Render("Run Sync — Step 3 of 4: Groups & Tables"))
		sb.WriteString("\n")
		sb.WriteString(m.form.View())

	case phasePickOptions:
		sb.WriteString(wizardTitleStyle.Render("Run Sync — Step 4 of 4: Options"))
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

	case phaseResults:
		if m.syncErr != nil {
			sb.WriteString(errorStyle.Render(fmt.Sprintf("Sync failed: %v", m.syncErr)))
		} else if m.options.dryRun {
			sb.WriteString(successStyle.Render("Dry run complete"))
		} else {
			sb.WriteString(successStyle.Render("Sync complete"))
		}
		sb.WriteString(fmt.Sprintf("  (%d tables, %s)\n\n", len(m.tasks), m.elapsed.Round(time.Millisecond)))
		sb.WriteString(borderStyle.Render(strings.Join(m.outputLines, "")))
		sb.WriteString("\n")
		sb.WriteString(helpStyle.Render("r: run again   esc/q: main menu"))
	}

	return docStyle.Render(sb.String())
}
