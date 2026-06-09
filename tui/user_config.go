package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/jwbonnell/pggosync/config"
)

type userConfigPhase int

const (
	ucPhaseList userConfigPhase = iota
	ucPhaseForm
	ucPhaseSetDefault
)

type connectionListItem struct {
	name          string
	defaultSource bool
	defaultDest   bool
}

func (i connectionListItem) Title() string {
	var tags []string
	if i.defaultSource {
		tags = append(tags, "default source")
	}
	if i.defaultDest {
		tags = append(tags, "default dest")
	}
	if len(tags) > 0 {
		return fmt.Sprintf("%s  [%s]", i.name, strings.Join(tags, ", "))
	}
	return i.name
}
func (i connectionListItem) Description() string { return "Connection config" }
func (i connectionListItem) FilterValue() string { return i.name }

type userConfigModel struct {
	handler *config.UserConfigHandler
	phase   userConfigPhase
	list    list.Model
	form    *huh.Form
	width   int
	height  int
	err     string
	status  string

	editingName string

	// flat form fields
	connName string
	host     string
	port     string
	database string
	user     string
	password string
	sslmode  string

	// set-default form fields
	defaultSource string
	defaultDest   string
}

func newUserConfigModel(handler *config.UserConfigHandler) userConfigModel {
	m := userConfigModel{
		handler: handler,
		phase:   ucPhaseList,
	}
	m.list = m.buildList()
	return m
}

func (m *userConfigModel) buildList() list.Model {
	conns, _ := m.handler.ListConnections()
	defaults, _ := m.handler.GetDefaults()

	items := make([]list.Item, 0, len(conns)+1)
	items = append(items, connectionListItem{name: "(+ New connection)"})
	for _, c := range conns {
		items = append(items, connectionListItem{
			name:          c,
			defaultSource: c == defaults.Source,
			defaultDest:   c == defaults.Dest,
		})
	}

	l := list.New(items, newMenuItemDelegate(), 60, 20)
	l.Title = "Connections"
	l.Styles.Title = titleStyle
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(true)
	return l
}

func (m *userConfigModel) buildConnectionForm(name string, existing *config.ConnectionConfig) *huh.Form {
	if existing != nil {
		m.connName = name
		m.host = existing.Host
		m.port = strconv.Itoa(existing.Port)
		m.database = existing.Database
		m.user = existing.User
		m.password = existing.Password
		m.sslmode = existing.SSLMode
		if m.sslmode == "" {
			m.sslmode = "disable"
		}
	} else {
		m.connName = ""
		m.host = "localhost"
		m.port = "5432"
		m.database = "postgres"
		m.user = ""
		m.password = ""
		m.sslmode = "disable"
	}
	nameField := huh.NewInput().
		Title("Connection name").
		Description("Identifier for this connection").
		Value(&m.connName)
	if existing != nil {
		nameField = nameField.Placeholder(name)
	}
	return huh.NewForm(
		huh.NewGroup(
			nameField,
			huh.NewInput().Title("Host").Value(&m.host),
			huh.NewInput().Title("Port").Value(&m.port).Validate(func(s string) error {
				p, err := strconv.Atoi(strings.TrimSpace(s))
				if err != nil || p < 1 || p > 65535 {
					return fmt.Errorf("must be a number between 1 and 65535")
				}
				return nil
			}),
			huh.NewInput().Title("Database").Value(&m.database),
			huh.NewInput().Title("User").Value(&m.user),
			huh.NewInput().Title("Password").EchoMode(huh.EchoModePassword).Value(&m.password),
			huh.NewSelect[string]().
				Title("SSL mode").
				Options(
					huh.NewOption("disable", "disable"),
					huh.NewOption("prefer", "prefer"),
					huh.NewOption("require", "require"),
					huh.NewOption("verify-full", "verify-full"),
				).
				Value(&m.sslmode),
		),
	)
}

func (m *userConfigModel) buildSetDefaultForm() *huh.Form {
	conns, _ := m.handler.ListConnections()
	options := make([]huh.Option[string], len(conns))
	for i, c := range conns {
		options[i] = huh.NewOption(c, c)
	}
	if d, err := m.handler.GetDefaults(); err == nil {
		m.defaultSource = d.Source
		m.defaultDest = d.Dest
	}
	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Default source").
				Options(options...).
				Value(&m.defaultSource),
			huh.NewSelect[string]().
				Title("Default destination").
				Options(options...).
				Value(&m.defaultDest),
		),
	)
}

func (m userConfigModel) Init() tea.Cmd {
	return nil
}

func (m userConfigModel) Update(msg tea.Msg) (userConfigModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		h, v := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)

	case tea.KeyMsg:
		if m.phase == ucPhaseList {
			switch msg.String() {
			case "esc", "q":
				return m, func() tea.Msg { return switchScreenMsg{screen: menuScreen} }
			case "enter", " ":
				return m.handleListSelect()
			case "n":
				return m.openNewForm()
			case "d":
				return m.openSetDefaultForm()
			}
		} else if msg.String() == "esc" {
			// In form phases, esc returns to the connection list.
			m.phase = ucPhaseList
			m.list = m.buildList()
			return m, nil
		}
	}

	if m.phase == ucPhaseForm || m.phase == ucPhaseSetDefault {
		form, cmd := m.form.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.form = f
		}
		if m.form.State == huh.StateCompleted {
			if m.phase == ucPhaseSetDefault {
				return m.saveDefaults()
			}
			return m.saveConnection()
		}
		if m.form.State == huh.StateAborted {
			m.phase = ucPhaseList
			m.list = m.buildList()
			return m, nil
		}
		return m, cmd
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m userConfigModel) handleListSelect() (userConfigModel, tea.Cmd) {
	item, ok := m.list.SelectedItem().(connectionListItem)
	if !ok {
		return m, nil
	}
	if item.name == "(+ New connection)" {
		return m.openNewForm()
	}
	existing, err := m.handler.GetConnection(item.name)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.editingName = item.name
	m.phase = ucPhaseForm
	m.form = m.buildConnectionForm(item.name, &existing)
	return m, m.form.Init()
}

func (m userConfigModel) openNewForm() (userConfigModel, tea.Cmd) {
	m.editingName = ""
	m.phase = ucPhaseForm
	m.form = m.buildConnectionForm("", nil)
	return m, m.form.Init()
}

func (m userConfigModel) openSetDefaultForm() (userConfigModel, tea.Cmd) {
	m.phase = ucPhaseSetDefault
	m.form = m.buildSetDefaultForm()
	return m, m.form.Init()
}

func (m userConfigModel) saveConnection() (userConfigModel, tea.Cmd) {
	name := strings.TrimSpace(m.connName)
	if name == "" && m.editingName != "" {
		name = m.editingName
	}
	if name == "" {
		m.err = "Connection name cannot be empty"
		m.phase = ucPhaseForm
		m.form = m.buildConnectionForm("", nil)
		return m, m.form.Init()
	}
	port, _ := strconv.Atoi(strings.TrimSpace(m.port))
	conn := config.ConnectionConfig{
		Host:     m.host,
		Port:     port,
		Database: m.database,
		User:     m.user,
		Password: m.password,
		SSLMode:  m.sslmode,
	}
	if err := m.handler.SaveConnection(name, conn); err != nil {
		m.err = err.Error()
		m.phase = ucPhaseList
		m.list = m.buildList()
		return m, nil
	}
	m.err = ""
	m.status = fmt.Sprintf("Saved connection %q", name)
	m.phase = ucPhaseList
	m.list = m.buildList()
	return m, nil
}

func (m userConfigModel) saveDefaults() (userConfigModel, tea.Cmd) {
	if err := m.handler.SetDefaults(m.defaultSource, m.defaultDest); err != nil {
		m.err = err.Error()
	} else {
		m.status = fmt.Sprintf("Defaults set — source: %s  dest: %s", m.defaultSource, m.defaultDest)
	}
	m.err = ""
	m.phase = ucPhaseList
	m.list = m.buildList()
	return m, nil
}

func (m userConfigModel) View() string {
	var sb strings.Builder
	helpS := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	switch m.phase {
	case ucPhaseList:
		sb.WriteString(m.list.View())
		if m.err != "" {
			sb.WriteString("\n" + errorStyle.Render(m.err))
		}
		if m.status != "" {
			sb.WriteString("\n" + successStyle.Render(m.status))
		}
		sb.WriteString("\n" + helpS.Render("enter: edit   n: new   d: set defaults   esc: back"))

	case ucPhaseForm:
		sb.WriteString(wizardTitleStyle.Render("Connection Config"))
		if m.err != "" {
			sb.WriteString("\n" + errorStyle.Render(m.err))
		}
		sb.WriteString("\n" + m.form.View())

	case ucPhaseSetDefault:
		sb.WriteString(wizardTitleStyle.Render("Set Default Connections"))
		sb.WriteString("\n" + m.form.View())
	}

	return docStyle.Render(sb.String())
}
