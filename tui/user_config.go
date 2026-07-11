package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/jwbonnell/pggosync/config"
)

type userConfigPhase int

const (
	ucPhaseList userConfigPhase = iota
	ucPhaseForm
)

type connectionListItem struct {
	name       string
	connString string
}

func (i connectionListItem) Title() string       { return i.name }
func (i connectionListItem) Description() string { return i.connString }
func (i connectionListItem) FilterValue() string { return i.name }

// maskedConnString formats a connection as a postgres:// URL with the password replaced by ***.
func maskedConnString(c config.ConnectionConfig) string {
	userInfo := c.User
	if c.Password != "" {
		userInfo += ":***"
	}
	s := fmt.Sprintf("postgres://%s@%s:%d/%s", userInfo, c.Host, c.Port, c.Database)
	if c.SSLMode != "" && c.SSLMode != "disable" {
		s += "?sslmode=" + c.SSLMode
	}
	return s
}

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
	formValues  *connectionFormValues
}

// newUserConfigModel creates the connection management screen with the connection list pre-loaded.
func newUserConfigModel(handler *config.UserConfigHandler) userConfigModel {
	m := userConfigModel{
		handler: handler,
		phase:   ucPhaseList,
	}
	m.list = m.buildList()
	return m
}

// buildList fetches all saved connections and constructs a list that includes a "(+ New connection)" entry at the top.
func (m *userConfigModel) buildList() list.Model {
	conns, _ := m.handler.ListConnections()

	items := make([]list.Item, 0, len(conns)+1)
	items = append(items, connectionListItem{name: "(+ New connection)"})
	for _, c := range conns {
		item := connectionListItem{name: c}
		if conn, err := m.handler.GetConnection(c); err == nil {
			item.connString = maskedConnString(conn)
		}
		items = append(items, item)
	}

	l := list.New(items, newMenuItemDelegate(), 60, 20)
	l.Title = "Connections"
	l.Styles.Title = titleStyle
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(true)
	return l
}

// buildConnectionForm creates the Huh form for creating or editing a connection; pre-populates fields when existing is non-nil.
func (m *userConfigModel) buildConnectionForm(name string, existing *config.ConnectionConfig) *huh.Form {
	m.formValues = newConnectionFormValues(name, existing)
	placeholder := ""
	if existing != nil {
		placeholder = name
	}
	return newConnectionForm(m.formValues, placeholder)
}

// Init satisfies tea.Model; the connection list needs no initial command.
func (m userConfigModel) Init() tea.Cmd {
	return nil
}

// Update routes messages between the list phase (browse) and form phase (create/edit).
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
			}
		} else if msg.String() == "esc" {
			// In form phases, esc returns to the connection list.
			m.phase = ucPhaseList
			m.list = m.buildList()
			return m, nil
		}
	}

	if m.phase == ucPhaseForm {
		form, cmd := m.form.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.form = f
		}
		if m.form.State == huh.StateCompleted {
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

// handleListSelect opens a blank form for the new-connection entry or a pre-populated form for an existing one.
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

// openNewForm resets editingName and switches to a blank connection form.
func (m userConfigModel) openNewForm() (userConfigModel, tea.Cmd) {
	m.editingName = ""
	m.phase = ucPhaseForm
	m.form = m.buildConnectionForm("", nil)
	return m, m.form.Init()
}

// saveConnection validates form fields, persists the connection config, and returns to the list.
func (m userConfigModel) saveConnection() (userConfigModel, tea.Cmd) {
	name, conn := m.formValues.connection()
	if name == "" && m.editingName != "" {
		name = m.editingName
	}
	if name == "" {
		m.err = "Connection name cannot be empty"
		m.phase = ucPhaseForm
		m.form = m.buildConnectionForm("", nil)
		return m, m.form.Init()
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

func (m userConfigModel) View() string {
	var sb strings.Builder

	switch m.phase {
	case ucPhaseList:
		sb.WriteString(m.list.View())
		if m.err != "" {
			sb.WriteString("\n" + errorStyle.Render(m.err))
		}
		if m.status != "" {
			sb.WriteString("\n" + successStyle.Render(m.status))
		}
		sb.WriteString("\n" + mutedStyle.Render("enter: edit   n: new   esc: back"))

	case ucPhaseForm:
		sb.WriteString(wizardTitleStyle.Render("Connection Config"))
		if m.err != "" {
			sb.WriteString("\n" + errorStyle.Render(m.err))
		}
		sb.WriteString("\n" + m.form.View())

	}

	return docStyle.Render(sb.String())
}
