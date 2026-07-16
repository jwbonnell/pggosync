package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
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
	styles  styles
	isDark  bool
	width   int
	height  int
	err     string
	status  string

	editingName string
	formValues  *connectionFormValues
}

// newUserConfigModel creates the connection management screen with the connection list pre-loaded.
func newUserConfigModel(s styles, isDark bool, handler *config.UserConfigHandler) userConfigModel {
	m := userConfigModel{
		handler: handler,
		phase:   ucPhaseList,
		styles:  s,
		isDark:  isDark,
	}
	m.list = m.buildList()
	return m
}

// withStyles re-themes the screen after the terminal background is known. The list bakes in its
// colours at construction, so it is rebuilt.
//
// The form is deliberately left alone. It only exists once the user opens one, which is long
// after the background reply lands at startup — by then buildConnectionForm picks up the new
// styles on its own. Rebuilding it here would reset formValues and discard whatever had been
// typed, which is a far worse failure than a stale palette.
func (m userConfigModel) withStyles(s styles, isDark bool) userConfigModel {
	m.styles = s
	m.isDark = isDark
	m.list.SetDelegate(newMenuItemDelegate(s, isDark))
	m.list.Styles = list.DefaultStyles(isDark)
	m.list.Styles.Title = s.title
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

	l := list.New(items, newMenuItemDelegate(m.styles, m.isDark), 60, 20)
	l.Title = "Connections"
	l.Styles = list.DefaultStyles(m.isDark)
	l.Styles.Title = m.styles.title
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(true)
	return l
}

// buildConnectionForm creates the Huh form for creating or editing a connection; pre-populates fields when existing is non-nil.
func (m *userConfigModel) buildConnectionForm(name string, existing *config.ConnectionConfig) *huh.Form {
	m.formValues = newConnectionFormValues(name, existing)
	placeholder := ""
	var nameValidate func(string) error
	if existing != nil {
		placeholder = name
	} else {
		// Creating a new connection: reject a name that already exists.
		nameValidate = func(candidate string) error {
			exists, err := m.handler.ConnectionExists(candidate)
			if err != nil {
				return err
			}
			if exists {
				return fmt.Errorf("connection %q already exists", candidate)
			}
			return nil
		}
	}
	return sizeForm(newConnectionForm(m.styles, m.formValues, placeholder, nameValidate), m.width)
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
		h, v := m.styles.doc.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
		// sizeForm pins the form's width, which stops huh from adopting resizes itself.
		if m.form != nil {
			m.form.WithWidth(msg.Width)
		}

	case tea.KeyPressMsg:
		if m.phase == ucPhaseList {
			switch msg.String() {
			case "esc", "q":
				return m, func() tea.Msg { return switchScreenMsg{screen: menuScreen} }
			// Bubble Tea v2 reports the space bar as "space"; " " never matches.
			case "enter", "space":
				return m.handleListSelect()
			case "n":
				return m.openNewForm()
			case "d":
				return m.deleteSelected()
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

// deleteSelected removes the highlighted connection (ignoring the "(+ New connection)" entry) and refreshes the list.
func (m userConfigModel) deleteSelected() (userConfigModel, tea.Cmd) {
	item, ok := m.list.SelectedItem().(connectionListItem)
	if !ok || item.name == "(+ New connection)" {
		return m, nil
	}
	if err := m.handler.DeleteConnection(item.name); err != nil {
		m.err = fmt.Sprintf("could not delete %q: %v", item.name, err)
		return m, nil
	}
	m.err = ""
	m.status = fmt.Sprintf("Deleted connection %q", item.name)
	m.list = m.buildList()
	return m, nil
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
	// If the user renamed an existing connection, remove the old file so it isn't orphaned.
	if m.editingName != "" && m.editingName != name {
		if err := m.handler.DeleteConnection(m.editingName); err != nil {
			m.err = fmt.Sprintf("saved %q but could not remove old %q: %v", name, m.editingName, err)
		}
	}
	m.editingName = ""
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
			sb.WriteString("\n" + m.styles.err.Render(m.err))
		}
		if m.status != "" {
			sb.WriteString("\n" + m.styles.success.Render(m.status))
		}
		sb.WriteString("\n" + m.styles.muted.Render("enter: edit   n: new   d: delete   esc: back"))

	case ucPhaseForm:
		sb.WriteString(m.styles.wizardTitle.Render("Connection Config"))
		if m.err != "" {
			sb.WriteString("\n" + m.styles.err.Render(m.err))
		}
		sb.WriteString("\n" + m.form.View())

	}

	return m.styles.doc.Render(sb.String())
}
