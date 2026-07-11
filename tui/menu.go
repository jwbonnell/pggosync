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
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.Foreground(colorPrimary).BorderForeground(colorPrimary)
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.Foreground(colorMuted).BorderForeground(colorPrimary)
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
	descS := lipgloss.NewStyle().PaddingLeft(4).Foreground(colorMuted)
	if selected {
		titleS = titleS.Foreground(colorPrimary).Bold(true)
	}
	_, _ = io.WriteString(w, titleS.Render(i.title)+"\n"+descS.Render(i.description))
}

type menuModel struct {
	list      list.Model
	lastEntry *config.SyncHistoryEntry
	width     int
	height    int
}

const (
	borderSize = 2 // a rounded border adds 2 columns (and 2 rows) total

	menuLogoArtWidth = 48 // width of menuLogoArt's widest line
	menuLogoGap      = 6  // gap between the menu box and the logo box
	menuBoxWidth     = 62 // fixed total width of the bordered menu box when the logo is shown
	menuPadH         = 2  // menuPanelStyle Padding(0,1) horizontal
	logoPadH         = 4  // logoPanelStyle Padding(1,2) horizontal

	// menuListW is the list's content width inside the menu box (outer − border − padding).
	menuListW = menuBoxWidth - borderSize - menuPadH
)

var (
	menuPanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(0, 1)

	logoPanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(1, 2).
			MarginLeft(menuLogoGap).
			Align(lipgloss.Center, lipgloss.Center)
)

// logoBoxOuter is the total width the logo box should render at to fill the space to the
// right of the fixed-width menu box (excluding the left-margin gap). innerWidth is the width
// inside docStyle's margins.
func logoBoxOuter(innerWidth int) int {
	return innerWidth - menuBoxWidth - menuLogoGap
}

// menuShowsLogo reports whether there's room for a logo box wide enough to hold the art.
func menuShowsLogo(innerWidth int) bool {
	return logoBoxOuter(innerWidth) >= menuLogoArtWidth+borderSize+logoPadH
}

// menuLogoContent returns the green block-letter banner plus a tagline (unbordered).
func menuLogoContent() string {
	return lipgloss.JoinVertical(
		lipgloss.Center,
		lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render(menuLogoArt),
		"",
		mutedStyle.Render("postgres → postgres data sync"),
	)
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
		menuItem{
			title:       "Manage Profiles",
			description: "Save and launch named sync configurations",
			target:      profileScreen,
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
		m.width = msg.Width
		m.height = msg.Height
		innerWidth := msg.Width - h
		if menuShowsLogo(innerWidth) {
			// Fixed-width menu box beside the logo; shorten by the border to fit the box.
			m.list.SetSize(menuListW, msg.Height-v-borderSize)
		} else {
			m.list.SetSize(innerWidth, msg.Height-v)
		}

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
// On a wide-enough terminal, the pggosync logo is shown to the right of the menu.
func (m menuModel) View() string {
	content := strings.TrimRight(m.list.View(), "\n")
	if m.lastEntry != nil {
		content += "\n\n" + lastSyncStyle.Render(formatLastSync(m.lastEntry))
	}

	h, _ := docStyle.GetFrameSize()
	innerWidth := m.width - h
	if !menuShowsLogo(innerWidth) {
		return docStyle.Render(content)
	}

	// Two bordered sections side by side. The menu box is a fixed width; the logo box
	// stretches to fill the rest of the row and to the menu box's height, with the logo
	// centered inside it. (lipgloss Width/Height set the box interior; the border adds 2.)
	menuPanel := menuPanelStyle.Width(menuBoxWidth - borderSize).Render(content)
	logoPanel := logoPanelStyle.
		Width(logoBoxOuter(innerWidth) - borderSize).
		Height(lipgloss.Height(menuPanel) - borderSize).
		Render(menuLogoContent())
	row := lipgloss.JoinHorizontal(lipgloss.Top, menuPanel, logoPanel)
	return docStyle.Render(row)
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
