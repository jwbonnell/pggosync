package tui

import (
	"fmt"
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
)

type configListItem struct {
	name   string
	active bool
}

func (i configListItem) Title() string {
	if i.active {
		return i.name + " (active)"
	}
	return i.name
}
func (i configListItem) Description() string {
	if i.active {
		return "Currently selected connection config"
	}
	return "Connection config"
}
func (i configListItem) FilterValue() string { return i.name }

type userConfigModel struct {
	handler *config.UserConfigHandler
	phase   userConfigPhase
	list    list.Model
	form    *huh.Form
	width   int
	height  int
	err     string
	status  string

	// form fields
	editingName string
	formConn    formDBConnections
}

type formDBConnections struct {
	srcHost string
	srcPort string
	srcDB   string
	srcUser string
	srcPass string
	dstHost string
	dstPort string
	dstDB   string
	dstUser string
	dstPass string
	cfgName string
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
	configs, _ := m.handler.ListConfigs()
	activeConfig, _ := m.handler.GetDefault()

	items := make([]list.Item, 0, len(configs)+1)
	items = append(items, configListItem{name: "(+ New config)", active: false})
	for _, c := range configs {
		items = append(items, configListItem{name: c, active: c == activeConfig})
	}

	l := list.New(items, newMenuItemDelegate(), 60, 20)
	l.Title = "Connection Configs"
	l.Styles.Title = titleStyle
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(true)
	return l
}

func (m *userConfigModel) buildForm(name string, existing *config.UserConfig) *huh.Form {
	conn := formDBConnections{
		cfgName: name,
	}
	if existing != nil {
		conn.srcHost = existing.Source.Host
		conn.srcPort = existing.Source.Port
		conn.srcDB = existing.Source.Database
		conn.srcUser = existing.Source.User
		conn.srcPass = existing.Source.Password
		conn.dstHost = existing.Destination.Host
		conn.dstPort = existing.Destination.Port
		conn.dstDB = existing.Destination.Database
		conn.dstUser = existing.Destination.User
		conn.dstPass = existing.Destination.Password
	} else {
		conn.srcHost = "localhost"
		conn.srcPort = "5432"
		conn.srcDB = "postgres"
		conn.srcUser = "source_user"
		conn.srcPass = ""
		conn.dstHost = "localhost"
		conn.dstPort = "5433"
		conn.dstDB = "postgres"
		conn.dstUser = "dest_user"
		conn.dstPass = ""
	}
	m.formConn = conn

	nameField := huh.NewInput().
		Title("Config name").
		Description("Identifier for this connection config").
		Value(&m.formConn.cfgName)
	if existing != nil {
		nameField = nameField.Placeholder(name)
	}

	return huh.NewForm(
		huh.NewGroup(
			nameField,
		).Title("Config"),
		huh.NewGroup(
			huh.NewInput().Title("Source host").Value(&m.formConn.srcHost),
			huh.NewInput().Title("Source port").Value(&m.formConn.srcPort),
			huh.NewInput().Title("Source database").Value(&m.formConn.srcDB),
			huh.NewInput().Title("Source user").Value(&m.formConn.srcUser),
			huh.NewInput().Title("Source password").EchoMode(huh.EchoModePassword).Value(&m.formConn.srcPass),
		).Title("Source Database"),
		huh.NewGroup(
			huh.NewInput().Title("Destination host").Value(&m.formConn.dstHost),
			huh.NewInput().Title("Destination port").Value(&m.formConn.dstPort),
			huh.NewInput().Title("Destination database").Value(&m.formConn.dstDB),
			huh.NewInput().Title("Destination user").Value(&m.formConn.dstUser),
			huh.NewInput().Title("Destination password").EchoMode(huh.EchoModePassword).Value(&m.formConn.dstPass),
		).Title("Destination Database"),
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
			case "s":
				return m.setDefaultFromList()
			}
		}
	}

	if m.phase == ucPhaseForm {
		form, cmd := m.form.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.form = f
		}
		if m.form.State == huh.StateCompleted {
			return m.saveForm()
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
	item, ok := m.list.SelectedItem().(configListItem)
	if !ok {
		return m, nil
	}
	if item.name == "(+ New config)" {
		return m.openNewForm()
	}
	// Edit existing
	existing, err := m.handler.GetConfig(item.name)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.editingName = item.name
	m.phase = ucPhaseForm
	m.form = m.buildForm(item.name, &existing)
	return m, m.form.Init()
}

func (m userConfigModel) openNewForm() (userConfigModel, tea.Cmd) {
	m.editingName = ""
	m.phase = ucPhaseForm
	m.form = m.buildForm("", nil)
	return m, m.form.Init()
}

func (m userConfigModel) setDefaultFromList() (userConfigModel, tea.Cmd) {
	item, ok := m.list.SelectedItem().(configListItem)
	if !ok || item.name == "(+ New config)" {
		return m, nil
	}
	if err := m.handler.SetDefault(item.name); err != nil {
		m.err = err.Error()
	} else {
		m.status = fmt.Sprintf("Active config set to %q", item.name)
	}
	m.list = m.buildList()
	return m, nil
}

func (m userConfigModel) saveForm() (userConfigModel, tea.Cmd) {
	name := strings.TrimSpace(m.formConn.cfgName)
	if name == "" && m.editingName != "" {
		name = m.editingName
	}
	if name == "" {
		m.err = "Config name cannot be empty"
		m.phase = ucPhaseForm
		m.form = m.buildForm("", nil)
		return m, m.form.Init()
	}

	uc := config.UserConfig{
		Source: config.DBConnection{
			Host:     m.formConn.srcHost,
			Port:     m.formConn.srcPort,
			Database: m.formConn.srcDB,
			User:     m.formConn.srcUser,
			Password: m.formConn.srcPass,
		},
		Destination: config.DBConnection{
			Host:     m.formConn.dstHost,
			Port:     m.formConn.dstPort,
			Database: m.formConn.dstDB,
			User:     m.formConn.dstUser,
			Password: m.formConn.dstPass,
		},
	}
	if err := m.handler.SaveConfig(name, uc); err != nil {
		m.err = err.Error()
		m.phase = ucPhaseList
		m.list = m.buildList()
		return m, nil
	}

	m.err = ""
	m.status = fmt.Sprintf("Saved config %q", name)
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
		sb.WriteString("\n" + helpS.Render("enter: edit   n: new   s: set active   esc: back"))
	case ucPhaseForm:
		sb.WriteString(wizardTitleStyle.Render("Connection Config"))
		if m.err != "" {
			sb.WriteString("\n" + errorStyle.Render(m.err))
		}
		sb.WriteString("\n" + m.form.View())
	}

	return docStyle.Render(sb.String())
}
