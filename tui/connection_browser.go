package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/jwbonnell/pggosync/config"
)

type connBrowserMode int

const (
	connBrowseList connBrowserMode = iota
	connBrowseDetail
)

// connectionBrowserModel is a standalone two-screen browser for `pggosync conn list`:
// a list of connections that, on select, is replaced by a read-only detail view.
// Escape from the detail view returns to the list.
type connectionBrowserModel struct {
	handler *config.UserConfigHandler
	mode    connBrowserMode
	list    list.Model
	detail  string
}

// newConnectionBrowserModel builds the browser with the connection list pre-loaded.
func newConnectionBrowserModel(handler *config.UserConfigHandler) connectionBrowserModel {
	conns, _ := handler.ListConnections()
	items := make([]list.Item, 0, len(conns))
	for _, c := range conns {
		item := connectionListItem{name: c}
		if conn, err := handler.GetConnection(c); err == nil {
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

	return connectionBrowserModel{handler: handler, mode: connBrowseList, list: l}
}

// Init satisfies tea.Model; the list needs no initial command.
func (m connectionBrowserModel) Init() tea.Cmd { return nil }

// Update routes keys per mode: the list browses/selects, the detail view returns on esc.
func (m connectionBrowserModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)

	case tea.KeyMsg:
		switch m.mode {
		case connBrowseList:
			switch msg.String() {
			case "esc", "q", "ctrl+c":
				return m, tea.Quit
			case "enter", " ":
				if item, ok := m.list.SelectedItem().(connectionListItem); ok {
					m.detail = m.renderDetail(item.name)
					m.mode = connBrowseDetail
				}
				return m, nil
			}
		case connBrowseDetail:
			switch msg.String() {
			case "esc", "backspace":
				m.mode = connBrowseList
				return m, nil
			case "q", "ctrl+c":
				return m, tea.Quit
			}
			return m, nil
		}
	}

	if m.mode == connBrowseList {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}
	return m, nil
}

// View renders whichever screen is active.
func (m connectionBrowserModel) View() string {
	if m.mode == connBrowseDetail {
		return docStyle.Render(m.detail)
	}
	return docStyle.Render(m.list.View())
}

// renderDetail builds the bordered read-only detail panel for a single connection,
// with the password masked, plus a footer of key bindings.
func (m connectionBrowserModel) renderDetail(name string) string {
	conn, err := m.handler.GetConnection(name)
	if err != nil {
		return detailBorderStyle.Render(errorStyle.Render(fmt.Sprintf("Could not load %q: %v", name, err)))
	}

	password := "(none)"
	if conn.Password != "" {
		password = "***"
	}

	var b strings.Builder
	b.WriteString(detailTitleStyle.Render(name))
	b.WriteString("\n\n")
	for _, row := range [][2]string{
		{"Host", conn.Host},
		{"Port", fmt.Sprintf("%d", conn.Port)},
		{"Database", conn.Database},
		{"User", conn.User},
		{"Password", password},
		{"SSL mode", conn.SSLMode},
	} {
		key := detailKeyStyle.Render(fmt.Sprintf("%-9s", row[0]+":"))
		b.WriteString(fmt.Sprintf("%s  %s\n", key, detailValueStyle.Render(row[1])))
	}

	panel := detailBorderStyle.Render(b.String())
	return panel + "\n" + helpStyle.Render("esc: back to list  ·  q: quit")
}

// RunConnectionBrowser runs the connection browser standalone (outside the full TUI) in
// alt-screen mode, so the detail view replaces the list and esc returns to it.
func RunConnectionBrowser(handler *config.UserConfigHandler) error {
	p := tea.NewProgram(newConnectionBrowserModel(handler), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
