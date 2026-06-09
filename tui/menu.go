package tui

import (
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			Padding(0, 1)

	docStyle = lipgloss.NewStyle().Margin(1, 2)
)

type menuItem struct {
	title       string
	description string
	target      screen
}

func (i menuItem) Title() string       { return i.title }
func (i menuItem) Description() string { return i.description }
func (i menuItem) FilterValue() string { return i.title }

type menuItemDelegate struct {
	list.DefaultDelegate
}

// newMenuItemDelegate creates a list delegate with the project's pink highlight colour.
func newMenuItemDelegate() menuItemDelegate {
	d := list.NewDefaultDelegate()
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.Foreground(lipgloss.Color("205")).BorderForeground(lipgloss.Color("205"))
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.Foreground(lipgloss.Color("240")).BorderForeground(lipgloss.Color("205"))
	return menuItemDelegate{d}
}

type menuDelegate struct{}

func (d menuDelegate) Height() int                             { return 2 }
func (d menuDelegate) Spacing() int                            { return 1 }
func (d menuDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d menuDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(menuItem)
	if !ok {
		return
	}
	selected := index == m.Index()
	titleS := lipgloss.NewStyle().PaddingLeft(2)
	descS := lipgloss.NewStyle().PaddingLeft(4).Foreground(lipgloss.Color("240"))
	if selected {
		titleS = titleS.Foreground(lipgloss.Color("205")).Bold(true)
	}
	_, _ = io.WriteString(w, titleS.Render(i.title)+"\n"+descS.Render(i.description))
}

type menuModel struct {
	list list.Model
}

// newMenuModel creates the main menu with its three navigation items.
func newMenuModel() menuModel {
	items := []list.Item{
		menuItem{
			title:       "Run Sync",
			description: "Guided setup to configure and run a database sync",
			target:      syncWizardScreen,
		},
		menuItem{
			title:       "Manage Connections",
			description: "Create, view, and switch connection credential configs",
			target:      userConfigScreen,
		},
		menuItem{
			title:       "Build Sync Config",
			description: "Interactively compose a sync config YAML file",
			target:      syncConfigScreen,
		},
	}

	l := list.New(items, newMenuItemDelegate(), 60, 20)
	l.Title = "pggosync"
	l.Styles.Title = titleStyle
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(true)

	return menuModel{list: l}
}

// Init satisfies tea.Model; the menu list needs no initial command.
func (m menuModel) Init() tea.Cmd {
	return nil
}

// Update handles resize, Enter/Space to navigate to a screen, and q to quit.
func (m menuModel) Update(msg tea.Msg) (menuModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)

	case tea.KeyMsg:
		switch msg.String() {
		case "enter", " ":
			if item, ok := m.list.SelectedItem().(menuItem); ok {
				return m, func() tea.Msg {
					return switchScreenMsg{screen: item.target}
				}
			}
		case "q":
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// View renders the menu list with document-style margins.
func (m menuModel) View() string {
	return docStyle.Render(strings.TrimRight(m.list.View(), "\n"))
}
