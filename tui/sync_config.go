package tui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"
)

type syncConfigPhase int

const (
	scPhaseMain syncConfigPhase = iota
	scPhaseAddGroup
	scPhaseAddTable
	scPhaseSave
	scPhaseDone
)

type syncConfigGroup struct {
	name   string
	tables map[string]string // table -> filter
}

type syncConfigBuilderModel struct {
	phase  syncConfigPhase
	form   *huh.Form
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
}

func newSyncConfigModel() syncConfigBuilderModel {
	m := syncConfigBuilderModel{
		phase: scPhaseMain,
	}
	m.form = m.buildMainForm()
	return m
}

func (m *syncConfigBuilderModel) buildMainForm() *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Description").
				Description("Human-readable description of this sync config").
				Value(&m.description),
			huh.NewText().
				Title("Exclude tables").
				Description("One table per line (schema.table or table). Leave blank to skip.").
				Value(&m.excludeRaw),
		),
	)
}

func (m *syncConfigBuilderModel) buildAddGroupForm() *huh.Form {
	m.pendingGroupName = ""
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Group name").
				Description("Name for this group of tables").
				Value(&m.pendingGroupName),
		),
	)
}

func (m *syncConfigBuilderModel) buildAddTableForm() *huh.Form {
	m.pendingTableName = ""
	m.pendingFilter = ""
	m.addAnotherTable = false
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Table name").
				Description("Format: schema.table or table").
				Value(&m.pendingTableName),
			huh.NewInput().
				Title("Filter (optional)").
				Description("WHERE clause, e.g. WHERE country_id = {1}").
				Value(&m.pendingFilter),
			huh.NewConfirm().
				Title("Add another table to this group?").
				Value(&m.addAnotherTable),
		),
	)
}

func (m *syncConfigBuilderModel) buildSaveForm() *huh.Form {
	m.savePath = ""
	m.addAnotherGroup = false
	return huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Add another group?").
				Value(&m.addAnotherGroup),
			huh.NewInput().
				Title("Save path").
				Description("File path for the generated YAML (e.g. _configs/my-sync.yml)").
				Placeholder("_configs/my-sync.yml").
				Value(&m.savePath),
		),
	)
}

func (m syncConfigBuilderModel) Init() tea.Cmd {
	return m.form.Init()
}

func (m syncConfigBuilderModel) Update(msg tea.Msg) (syncConfigBuilderModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		if m.phase == scPhaseDone {
			switch msg.String() {
			case "esc", "q", "enter":
				return m, func() tea.Msg { return switchScreenMsg{screen: menuScreen} }
			}
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
		return m, func() tea.Msg { return switchScreenMsg{screen: menuScreen} }
	}

	if m.form.State == huh.StateCompleted {
		return m.advance()
	}

	return m, cmd
}

func (m syncConfigBuilderModel) advance() (syncConfigBuilderModel, tea.Cmd) {
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
		m.groups = append(m.groups, syncConfigGroup{name: name, tables: map[string]string{}})
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
		lastGroup := &m.groups[len(m.groups)-1]
		lastGroup.tables[tableName] = strings.TrimSpace(m.pendingFilter)
		if m.addAnotherTable {
			m.form = m.buildAddTableForm()
			return m, m.form.Init()
		}
		// Done adding tables to this group — ask about another group + save path
		m.phase = scPhaseSave
		m.form = m.buildSaveForm()
		return m, m.form.Init()

	case scPhaseSave:
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
	groups := map[string]map[string]string{}
	for _, g := range m.groups {
		if len(g.tables) > 0 {
			groups[g.name] = g.tables
		}
	}

	type syncConfigYAML struct {
		Description string                       `yaml:"description,omitempty"`
		Exclude     []string                     `yaml:"exclude,omitempty"`
		Groups      map[string]map[string]string `yaml:"groups,omitempty"`
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

func (m syncConfigBuilderModel) View() string {
	var sb strings.Builder

	helpS := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	switch m.phase {
	case scPhaseMain:
		sb.WriteString(wizardTitleStyle.Render("Build Sync Config — Step 1: Description & Exclusions"))
		sb.WriteString("\n")
		sb.WriteString(m.form.View())

	case scPhaseAddGroup:
		sb.WriteString(wizardTitleStyle.Render(fmt.Sprintf("Build Sync Config — Add Group (%d so far)", len(m.groups))))
		sb.WriteString("\n")
		if m.err != "" {
			sb.WriteString(errorStyle.Render(m.err) + "\n")
		}
		sb.WriteString(m.form.View())

	case scPhaseAddTable:
		lastGroup := m.groups[len(m.groups)-1]
		sb.WriteString(wizardTitleStyle.Render(fmt.Sprintf("Build Sync Config — Group %q: Add Table (%d so far)", lastGroup.name, len(lastGroup.tables))))
		sb.WriteString("\n")
		if m.err != "" {
			sb.WriteString(errorStyle.Render(m.err) + "\n")
		}
		sb.WriteString(m.form.View())

	case scPhaseSave:
		sb.WriteString(wizardTitleStyle.Render("Build Sync Config — Finish"))
		sb.WriteString("\n")
		if m.err != "" {
			sb.WriteString(errorStyle.Render(m.err) + "\n")
		}
		sb.WriteString(m.form.View())

	case scPhaseDone:
		sb.WriteString(successStyle.Render("Sync config saved!"))
		sb.WriteString("\n")
		sb.WriteString(m.status + "\n\n")
		sb.WriteString(summaryView(m))
		sb.WriteString("\n" + helpS.Render("press any key to return to the menu"))
	}

	return docStyle.Render(sb.String())
}

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
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(sb.String())
}
