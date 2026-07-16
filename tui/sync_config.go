package tui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/jwbonnell/pggosync/config"
	"github.com/jwbonnell/pggosync/sync/data"
	"gopkg.in/yaml.v3"
)

type syncConfigPhase int

const (
	scPhaseMain syncConfigPhase = iota
	scPhaseAddGroup
	scPhaseAddTable
	scPhaseAddScrub
	scPhaseSave
	scPhaseDone
)

type syncConfigTable struct {
	name   string
	filter string
	scrub  []config.ScrubRule
}

type syncConfigGroup struct {
	name   string
	tables []syncConfigTable
}

type syncConfigBuilderModel struct {
	phase  syncConfigPhase
	form   *huh.Form
	styles styles
	err    string
	status string
	width  int
	height int

	description string
	excludeRaw  string
	savePath    string
	groups      []syncConfigGroup

	// add-group flow
	pendingGroupName string
	pendingTableName string
	pendingFilter    string
	addAnotherTable  bool
	addAnotherGroup  bool

	// scrub rule flow
	pendingScrubColumn string
	pendingScrubRule   string
	addAnotherScrub    bool
}

// newSyncConfigModel creates a blank sync config builder at the first phase.
func newSyncConfigModel(s styles) syncConfigBuilderModel {
	m := syncConfigBuilderModel{
		phase:  scPhaseMain,
		styles: s,
	}
	m.form = m.buildMainForm()
	return m
}

// withStyles re-themes the builder after the terminal background is known.
//
// A form bakes in its theme when built, so the one standing at construction has to be rebuilt to
// pick up the new palette. That is only safe because the background reply lands at startup, while
// the builder is still on its first, untouched form — a rebuild resets the bound fields, so doing
// it mid-flow would throw away the user's input. Every later form is built by advance(), which
// reads the already-updated styles.
func (m syncConfigBuilderModel) withStyles(s styles) syncConfigBuilderModel {
	m.styles = s
	if m.phase == scPhaseMain {
		m.form = m.buildMainForm()
	}
	return m
}

// newForm builds a themed form sized to the current terminal. Every builder below goes through
// it so that a form built mid-flow is laid out for the real terminal rather than huh's default
// width; see sizeForm.
func (m *syncConfigBuilderModel) newForm(groups ...*huh.Group) *huh.Form {
	return sizeForm(m.styles.newForm(groups...), m.width)
}

// buildMainForm creates the description and exclude-tables input form.
func (m *syncConfigBuilderModel) buildMainForm() *huh.Form {
	return m.newForm(
		huh.NewGroup(
			huh.NewInput().
				Key("description").
				Title("Description").
				Description("Human-readable description of this sync config").
				Value(&m.description),
			huh.NewText().
				Key("exclude").
				Title("Exclude tables").
				Description("One table per line (schema.table or table). Leave blank to skip.").
				Value(&m.excludeRaw),
		),
	)
}

// buildAddGroupForm creates the group-name input form and resets pendingGroupName.
func (m *syncConfigBuilderModel) buildAddGroupForm() *huh.Form {
	m.pendingGroupName = ""
	return m.newForm(
		huh.NewGroup(
			huh.NewInput().
				Key("groupname").
				Title("Group name").
				Description("Name for this group of tables").
				Value(&m.pendingGroupName),
		),
	)
}

// buildAddTableForm creates the table-name and filter input form for the current group.
func (m *syncConfigBuilderModel) buildAddTableForm() *huh.Form {
	m.pendingTableName = ""
	m.pendingFilter = ""
	return m.newForm(
		huh.NewGroup(
			huh.NewInput().
				Key("tablename").
				Title("Table name").
				Description("Format: schema.table or table").
				Value(&m.pendingTableName),
			huh.NewInput().
				Key("filter").
				Title("Filter (optional)").
				Description("SQL predicate, e.g. country_id = {1}").
				Value(&m.pendingFilter),
		),
	)
}

// buildAddScrubForm creates the scrub rule entry form for the current table.
func (m *syncConfigBuilderModel) buildAddScrubForm() *huh.Form {
	m.pendingScrubColumn = ""
	m.pendingScrubRule = ""
	m.addAnotherScrub = false

	ruleOptions := make([]huh.Option[string], len(data.SupportedRules))
	for i, r := range data.SupportedRules {
		ruleOptions[i] = huh.NewOption(r, r)
	}

	return m.newForm(
		huh.NewGroup(
			huh.NewInput().
				Key("scrubcol").
				Title("Column to scrub").
				Description("Column name in this table").
				Value(&m.pendingScrubColumn),
			huh.NewSelect[string]().
				Key("scrubrule").
				Title("Scrub rule").
				Description("How to transform the column value").
				Options(ruleOptions...).
				Value(&m.pendingScrubRule),
			huh.NewConfirm().
				Key("another_scrub").
				Title("Add another scrub rule to this table?").
				Value(&m.addAnotherScrub),
		),
	)
}

// buildSaveForm creates the finish form: add another table?, add another group?, and save path.
func (m *syncConfigBuilderModel) buildSaveForm() *huh.Form {
	m.savePath = ""
	m.addAnotherGroup = false
	m.addAnotherTable = false
	return m.newForm(
		huh.NewGroup(
			huh.NewConfirm().
				Key("another_table").
				Title("Add another table to this group?").
				Value(&m.addAnotherTable),
			huh.NewConfirm().
				Key("another_group").
				Title("Add another group?").
				Description("Only relevant if you chose not to add another table above.").
				Value(&m.addAnotherGroup),
			huh.NewInput().
				Key("savepath").
				Title("Save path").
				Description("File path for the generated YAML (e.g. _configs/my-sync.yml)").
				Placeholder("_configs/my-sync.yml").
				Value(&m.savePath),
		),
	)
}

// Init satisfies tea.Model by initialising the current phase's form.
func (m syncConfigBuilderModel) Init() tea.Cmd {
	return m.form.Init()
}

// Update routes form completions to advance, aborts and Esc to goBack, and Enter on done to return to the menu.
func (m syncConfigBuilderModel) Update(msg tea.Msg) (syncConfigBuilderModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// sizeForm pins the form's width, which stops huh from adopting resizes itself.
		if m.form != nil {
			m.form.WithWidth(msg.Width)
		}

	case tea.KeyPressMsg:
		if m.phase == scPhaseDone {
			switch msg.String() {
			case "esc", "q", "enter":
				return m, func() tea.Msg { return switchScreenMsg{screen: menuScreen} }
			}
		} else if msg.String() == "esc" {
			return m.goBack()
		}
	}

	if m.phase == scPhaseDone {
		return m, nil
	}

	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f
	}

	if m.form.State == huh.StateAborted {
		return m.goBack()
	}

	if m.form.State == huh.StateCompleted {
		return m.advance()
	}

	return m, cmd
}

// goBack navigates to the previous phase, removing an empty pending group if the user backs out of table entry.
func (m syncConfigBuilderModel) goBack() (syncConfigBuilderModel, tea.Cmd) {
	switch m.phase {
	case scPhaseMain:
		return m, func() tea.Msg { return switchScreenMsg{screen: menuScreen} }
	case scPhaseAddGroup:
		m.phase = scPhaseMain
		m.form = m.buildMainForm()
	case scPhaseAddTable:
		// If the current group has no tables yet, remove it before going back.
		if len(m.groups) > 0 && len(m.groups[len(m.groups)-1].tables) == 0 {
			m.groups = m.groups[:len(m.groups)-1]
		}
		m.phase = scPhaseAddGroup
		m.form = m.buildAddGroupForm()
	case scPhaseAddScrub:
		// The table was already appended when entering the scrub phase; pop it so
		// re-entering the table form doesn't create a duplicate entry.
		if len(m.groups) > 0 {
			lastGroup := &m.groups[len(m.groups)-1]
			if len(lastGroup.tables) > 0 {
				lastGroup.tables = lastGroup.tables[:len(lastGroup.tables)-1]
			}
		}
		m.phase = scPhaseAddTable
		m.form = m.buildAddTableForm()
	case scPhaseSave:
		// Return to main so the user can review description/excludes; groups are kept.
		m.phase = scPhaseMain
		m.form = m.buildMainForm()
	default:
		return m, func() tea.Msg { return switchScreenMsg{screen: menuScreen} }
	}
	return m, m.form.Init()
}

// captureForm copies the just-completed form's values (keyed) into the model's fields for the
// current phase. huh binds each field via a pointer captured when the form was built, but bubbletea
// copies the model by value on every Update, so those pointers target a stale copy and the struct
// fields never receive input. The *huh.Form is a shared pointer, so reading by key is reliable.
// This mirrors captureForm in sync_wizard.go; see the project_tui_form_capture memory note.
func (m *syncConfigBuilderModel) captureForm() {
	switch m.phase {
	case scPhaseMain:
		m.description = m.form.GetString("description")
		m.excludeRaw = m.form.GetString("exclude")
	case scPhaseAddGroup:
		m.pendingGroupName = m.form.GetString("groupname")
	case scPhaseAddTable:
		m.pendingTableName = m.form.GetString("tablename")
		m.pendingFilter = m.form.GetString("filter")
	case scPhaseAddScrub:
		m.pendingScrubColumn = m.form.GetString("scrubcol")
		m.pendingScrubRule = m.form.GetString("scrubrule")
		m.addAnotherScrub = m.form.GetBool("another_scrub")
	case scPhaseSave:
		m.addAnotherTable = m.form.GetBool("another_table")
		m.addAnotherGroup = m.form.GetBool("another_group")
		m.savePath = m.form.GetString("savepath")
	}
}

// advance moves the wizard forward: validates input, appends groups/tables, and transitions to the next phase.
func (m syncConfigBuilderModel) advance() (syncConfigBuilderModel, tea.Cmd) {
	m.captureForm()
	switch m.phase {
	case scPhaseMain:
		// Start first group
		m.phase = scPhaseAddGroup
		m.form = m.buildAddGroupForm()
		return m, m.form.Init()

	case scPhaseAddGroup:
		name := strings.TrimSpace(m.pendingGroupName)
		if name == "" {
			m.err = "Group name cannot be empty"
			m.form = m.buildAddGroupForm()
			return m, m.form.Init()
		}
		m.groups = append(m.groups, syncConfigGroup{name: name})
		m.err = ""
		m.phase = scPhaseAddTable
		m.form = m.buildAddTableForm()
		return m, m.form.Init()

	case scPhaseAddTable:
		tableName := strings.TrimSpace(m.pendingTableName)
		if tableName == "" {
			m.err = "Table name cannot be empty"
			m.form = m.buildAddTableForm()
			return m, m.form.Init()
		}
		m.err = ""
		// Append table placeholder (scrub rules added in scrub phase)
		lastGroup := &m.groups[len(m.groups)-1]
		lastGroup.tables = append(lastGroup.tables, syncConfigTable{
			name:   tableName,
			filter: strings.TrimSpace(m.pendingFilter),
		})
		m.phase = scPhaseAddScrub
		m.form = m.buildAddScrubForm()
		return m, m.form.Init()

	case scPhaseAddScrub:
		col := strings.TrimSpace(m.pendingScrubColumn)
		rule := strings.TrimSpace(m.pendingScrubRule)
		if col != "" && rule != "" {
			lastGroup := &m.groups[len(m.groups)-1]
			lastTable := &lastGroup.tables[len(lastGroup.tables)-1]
			lastTable.scrub = append(lastTable.scrub, config.ScrubRule{
				Column: col,
				Rule:   rule,
			})
		}
		if m.addAnotherScrub {
			m.form = m.buildAddScrubForm()
			return m, m.form.Init()
		}
		// Done with scrub — ask about another table
		m.phase = scPhaseSave
		m.form = m.buildSaveForm()
		return m, m.form.Init()

	case scPhaseSave:
		if m.addAnotherTable {
			m.phase = scPhaseAddTable
			m.form = m.buildAddTableForm()
			return m, m.form.Init()
		}
		if m.addAnotherGroup {
			m.phase = scPhaseAddGroup
			m.form = m.buildAddGroupForm()
			return m, m.form.Init()
		}
		// Write YAML
		return m.writeYAML()
	}

	return m, nil
}

// writeYAML marshals the collected config to YAML and writes it to the user-supplied save path.
func (m syncConfigBuilderModel) writeYAML() (syncConfigBuilderModel, tea.Cmd) {
	savePath := strings.TrimSpace(m.savePath)
	if savePath == "" {
		m.err = "Save path cannot be empty"
		m.phase = scPhaseSave
		m.form = m.buildSaveForm()
		return m, m.form.Init()
	}

	// Build exclude list
	var exclude []string
	for _, line := range strings.Split(m.excludeRaw, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			exclude = append(exclude, line)
		}
	}

	// Build groups map
	groups := map[string]config.Group{}
	for _, g := range m.groups {
		if len(g.tables) > 0 {
			var entries []config.TableEntry
			for _, t := range g.tables {
				entries = append(entries, config.TableEntry{
					Table:  t.name,
					Filter: t.filter,
					Scrub:  t.scrub,
				})
			}
			groups[g.name] = config.Group{Tables: entries}
		}
	}

	type syncConfigYAML struct {
		Description string                  `yaml:"description,omitempty"`
		Exclude     []string                `yaml:"exclude,omitempty"`
		Groups      map[string]config.Group `yaml:"groups,omitempty"`
	}

	out := syncConfigYAML{
		Description: m.description,
		Exclude:     exclude,
		Groups:      groups,
	}

	data, err := yaml.Marshal(out)
	if err != nil {
		m.err = fmt.Sprintf("YAML marshal error: %v", err)
		m.phase = scPhaseSave
		m.form = m.buildSaveForm()
		return m, m.form.Init()
	}

	if err := os.WriteFile(savePath, data, 0644); err != nil {
		m.err = fmt.Sprintf("Could not write file: %v", err)
		m.phase = scPhaseSave
		m.form = m.buildSaveForm()
		return m, m.form.Init()
	}

	m.status = fmt.Sprintf("Saved to %s", savePath)
	m.err = ""
	m.phase = scPhaseDone
	return m, nil
}

// View renders the current wizard phase with a title header, error/status messages, and the active form.
func (m syncConfigBuilderModel) View() string {
	var sb strings.Builder

	switch m.phase {
	case scPhaseMain:
		sb.WriteString(m.styles.wizardTitle.Render("Build Sync Config — Step 1: Description & Exclusions"))
		sb.WriteString("\n")
		sb.WriteString(m.form.View())

	case scPhaseAddGroup:
		sb.WriteString(m.styles.wizardTitle.Render(fmt.Sprintf("Build Sync Config — Add Group (%d so far)", len(m.groups))))
		sb.WriteString("\n")
		if m.err != "" {
			sb.WriteString(m.styles.err.Render(m.err) + "\n")
		}
		sb.WriteString(m.form.View())

	case scPhaseAddTable:
		lastGroup := m.groups[len(m.groups)-1]
		sb.WriteString(m.styles.wizardTitle.Render(fmt.Sprintf("Build Sync Config — Group %q: Add Table (%d so far)", lastGroup.name, len(lastGroup.tables))))
		sb.WriteString("\n")
		if m.err != "" {
			sb.WriteString(m.styles.err.Render(m.err) + "\n")
		}
		sb.WriteString(m.form.View())

	case scPhaseAddScrub:
		lastGroup := m.groups[len(m.groups)-1]
		lastTable := lastGroup.tables[len(lastGroup.tables)-1]
		scrubInfo := ""
		if len(lastTable.scrub) > 0 {
			labels := make([]string, len(lastTable.scrub))
			for j, r := range lastTable.scrub {
				labels[j] = fmt.Sprintf("%s=%s", r.Column, data.RuleLabel(r.Rule))
			}
			scrubInfo = fmt.Sprintf("  [%s]", strings.Join(labels, ", "))
		}
		title := fmt.Sprintf("Build Sync Config — Scrub %s%s", lastTable.name, scrubInfo)
		sb.WriteString(m.styles.wizardTitle.Render(title))
		sb.WriteString("\n")
		if m.err != "" {
			sb.WriteString(m.styles.err.Render(m.err) + "\n")
		}
		sb.WriteString(m.form.View())

	case scPhaseSave:
		sb.WriteString(m.styles.wizardTitle.Render("Build Sync Config — Finish"))
		sb.WriteString("\n")
		if m.err != "" {
			sb.WriteString(m.styles.err.Render(m.err) + "\n")
		}
		sb.WriteString(m.form.View())

	case scPhaseDone:
		sb.WriteString(m.styles.success.Render("Sync config saved!"))
		sb.WriteString("\n")
		sb.WriteString(m.status + "\n\n")
		sb.WriteString(summaryView(m))
		sb.WriteString("\n" + m.styles.muted.Render("press any key to return to the menu"))
	}

	return m.styles.doc.Render(sb.String())
}

// summaryView formats the completed config as a compact YAML-like summary for the done screen.
func summaryView(m syncConfigBuilderModel) string {
	var sb strings.Builder
	if m.description != "" {
		sb.WriteString(fmt.Sprintf("description: %s\n", m.description))
	}
	if m.excludeRaw != "" {
		sb.WriteString("exclude:\n")
		for _, l := range strings.Split(m.excludeRaw, "\n") {
			l = strings.TrimSpace(l)
			if l != "" {
				sb.WriteString(fmt.Sprintf("  - %s\n", l))
			}
		}
	}
	for _, g := range m.groups {
		sb.WriteString(fmt.Sprintf("groups.%s: %d table(s)\n", g.name, len(g.tables)))
		for _, t := range g.tables {
			if len(t.scrub) > 0 {
				labels := make([]string, len(t.scrub))
				for j, r := range t.scrub {
					labels[j] = fmt.Sprintf("%s=%s", r.Column, data.RuleLabel(r.Rule))
				}
				sb.WriteString(fmt.Sprintf("    %s [%s]\n", t.name, strings.Join(labels, ", ")))
			}
		}
	}
	return m.styles.muted.Render(sb.String())
}
