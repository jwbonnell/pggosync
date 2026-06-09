package tui

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jwbonnell/pggosync/config"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			Padding(0, 1)

	docStyle = lipgloss.NewStyle().Margin(1, 2)

	lastSyncStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
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
	list      list.Model
	lastEntry *config.SyncHistoryEntry
}

// newMenuModel creates the main menu with its three navigation items and an optional last-sync summary.
func newMenuModel(lastEntry *config.SyncHistoryEntry) menuModel {
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

	return menuModel{list: l, lastEntry: lastEntry}
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

// View renders the menu list with document-style margins and an optional last-sync summary line.
func (m menuModel) View() string {
	content := strings.TrimRight(m.list.View(), "\n")
	if m.lastEntry != nil {
		content += "\n\n" + lastSyncStyle.Render(formatLastSync(m.lastEntry))
	}
	return docStyle.Render(content)
}

func formatLastSync(e *config.SyncHistoryEntry) string {
	status := fmt.Sprintf("%d tables · %s rows", len(e.Tables), formatRowCount(e.TotalRows))
	if e.Error != "" {
		status = "failed: " + e.Error
	}
	return fmt.Sprintf("Last sync: %s · %s → %s · %s", formatElapsed(e.Timestamp), e.Source, e.Dest, status)
}

func formatElapsed(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func formatRowCount(n int64) string {
	switch {
	case n < 1_000:
		return fmt.Sprintf("%d", n)
	case n < 1_000_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
}
