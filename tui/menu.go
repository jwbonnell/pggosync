package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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

// newMenuItemDelegate creates a list delegate highlighted in the project's primary colour.
// The delegate's own defaults no longer adapt to the background on their own (lipgloss v2
// dropped AdaptiveColor), so they are re-seeded from isDark before being recoloured.
func newMenuItemDelegate(s styles, isDark bool) menuItemDelegate {
	d := list.NewDefaultDelegate()
	d.Styles = list.NewDefaultItemStyles(isDark)
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.Foreground(s.c.primary).BorderForeground(s.c.primary)
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.Foreground(s.c.muted).BorderForeground(s.c.primary)
	return menuItemDelegate{d}
}

type menuModel struct {
	list      list.Model
	styles    styles
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

// menuPanel is the bordered box the menu list sits in.
func (s styles) menuPanel() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(s.c.primary).
		Padding(0, 1)
}

// logoPanel is the bordered box beside the menu, holding the logo art.
func (s styles) logoPanel() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(s.c.primary).
		Padding(1, 2).
		MarginLeft(menuLogoGap).
		Align(lipgloss.Center, lipgloss.Center)
}

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
func menuLogoContent(s styles) string {
	return lipgloss.JoinVertical(
		lipgloss.Center,
		lipgloss.NewStyle().Foreground(s.c.primary).Bold(true).Render(menuLogoArt),
		"",
		s.muted.Render("postgres → postgres data sync"),
	)
}

// newMenuModel creates the main menu with its three navigation items and an optional last-sync summary.
func newMenuModel(s styles, isDark bool, lastEntry *config.SyncHistoryEntry) menuModel {
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

	l := list.New(items, newMenuItemDelegate(s, isDark), 60, 20)
	l.Title = "pggosync"
	l.Styles = list.DefaultStyles(isDark)
	l.Styles.Title = s.title
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(true)

	return menuModel{list: l, styles: s, lastEntry: lastEntry}
}

// withStyles re-themes the menu after the terminal background is known. The list's own styles
// and its delegate bake in colours at construction, so they are rebuilt rather than reassigned.
func (m menuModel) withStyles(s styles, isDark bool) menuModel {
	m.styles = s
	m.list.SetDelegate(newMenuItemDelegate(s, isDark))
	m.list.Styles = list.DefaultStyles(isDark)
	m.list.Styles.Title = s.title
	return m
}

// Init satisfies tea.Model; the menu list needs no initial command.
func (m menuModel) Init() tea.Cmd {
	return nil
}

// Update handles resize, Enter/Space to navigate to a screen, and q to quit.
func (m menuModel) Update(msg tea.Msg) (menuModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := m.styles.doc.GetFrameSize()
		m.width = msg.Width
		m.height = msg.Height
		innerWidth := msg.Width - h
		if menuShowsLogo(innerWidth) {
			// Fixed-width menu box beside the logo; shorten by the border to fit the box.
			m.list.SetSize(menuListW, msg.Height-v-borderSize)
		} else {
			m.list.SetSize(innerWidth, msg.Height-v)
		}

	case tea.KeyPressMsg:
		switch msg.String() {
		// Bubble Tea v2 reports the space bar as "space"; " " never matches.
		case "enter", "space":
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
		content += "\n\n" + m.styles.lastSync.Render(formatLastSync(m.lastEntry))
	}

	h, _ := m.styles.doc.GetFrameSize()
	innerWidth := m.width - h
	if !menuShowsLogo(innerWidth) {
		return m.styles.doc.Render(content)
	}

	// Two bordered sections side by side. The menu box is a fixed width; the logo box
	// stretches to fill the rest of the row and to the menu box's height, with the logo
	// centered inside it. (lipgloss Width/Height set the box interior; the border adds 2.)
	menuPanel := m.styles.menuPanel().Width(menuBoxWidth - borderSize).Render(content)
	logoPanel := m.styles.logoPanel().
		Width(logoBoxOuter(innerWidth) - borderSize).
		Height(lipgloss.Height(menuPanel) - borderSize).
		Render(menuLogoContent(m.styles))
	row := lipgloss.JoinHorizontal(lipgloss.Top, menuPanel, logoPanel)
	return m.styles.doc.Render(row)
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
