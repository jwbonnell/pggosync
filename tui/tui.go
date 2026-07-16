package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/jwbonnell/pggosync/config"
)

type screen int

const (
	menuScreen screen = iota
	syncWizardScreen
	userConfigScreen
	syncConfigScreen
	profileScreen
)

type model struct {
	screen     screen
	menu       menuModel
	syncWizard syncWizardModel
	userConfig userConfigModel
	syncConfig syncConfigBuilderModel
	profiles   profileModel
	handler    *config.UserConfigHandler
	styles     styles
	isDark     bool
	width      int
	height     int
}

type switchScreenMsg struct {
	screen screen
}

// newModel builds the root model with the main menu as the initial screen.
//
// The palette starts on its dark variants: Init asks the terminal for its actual background and
// the styles are rebuilt when it answers. Dark is the better guess for the brief moment before
// that (and for terminals that never answer), since it matches lipgloss's own fallback.
func newModel(handler *config.UserConfigHandler) model {
	s := newStyles(true)
	return model{
		screen:     menuScreen,
		menu:       newMenuModel(s, true, lastSyncEntry(handler)),
		syncWizard: newSyncWizardModel(s, handler),
		userConfig: newUserConfigModel(s, true, handler),
		syncConfig: newSyncConfigModel(s),
		profiles:   newProfileModel(s, true, handler),
		handler:    handler,
		styles:     s,
		isDark:     true,
	}
}

// Run starts the Bubble Tea program. Alt-screen mode is no longer a program option in Bubble Tea
// v2 — it is declared per-frame in View.
func Run(handler *config.UserConfigHandler) error {
	p := tea.NewProgram(newModel(handler))
	_, err := p.Run()
	return err
}

// lastSyncEntry loads the most recent history entry, returning nil if none exists or on error.
func lastSyncEntry(handler *config.UserConfigHandler) *config.SyncHistoryEntry {
	h, err := handler.LoadSyncHistory()
	if err != nil || len(h.Entries) == 0 {
		return nil
	}
	e := h.Entries[len(h.Entries)-1]
	return &e
}

// Init delegates to the menu's Init so the list renders immediately, and asks the terminal for
// its background colour so the palette can be resolved (see the tea.BackgroundColorMsg case in
// Update). Bubble Tea does not query the background on its own.
func (m model) Init() tea.Cmd {
	return tea.Batch(m.menu.Init(), tea.RequestBackgroundColor)
}

// Update routes messages to the active screen; handles window resize, screen switching, and global Ctrl+C.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		// The terminal answered Init's query. Rebuild the palette against the real background
		// and hand it to every screen — not just the active one, since the others were built
		// with the provisional dark styles and are switched to without being reconstructed.
		m.isDark = msg.IsDark()
		m.styles = newStyles(m.isDark)
		m.menu = m.menu.withStyles(m.styles, m.isDark)
		m.syncWizard = m.syncWizard.withStyles(m.styles)
		m.userConfig = m.userConfig.withStyles(m.styles, m.isDark)
		m.syncConfig = m.syncConfig.withStyles(m.styles)
		m.profiles = m.profiles.withStyles(m.styles, m.isDark)
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		var cmd tea.Cmd
		switch m.screen {
		case menuScreen:
			m.menu, cmd = m.menu.Update(msg)
		case syncWizardScreen:
			m.syncWizard, cmd = m.syncWizard.Update(msg)
		case userConfigScreen:
			m.userConfig, cmd = m.userConfig.Update(msg)
		case syncConfigScreen:
			m.syncConfig, cmd = m.syncConfig.Update(msg)
		case profileScreen:
			m.profiles, cmd = m.profiles.Update(msg)
		}
		return m, cmd

	case launchProfileMsg:
		wiz := newSyncWizardModelFromProfile(m.styles, m.handler, msg.profile)
		// Give the wizard the current size before kicking off the preview so the viewport (built
		// when the async result lands) isn't created with a zero (negative) width/height.
		wiz, _ = wiz.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		wiz, cmd := wiz.startPreview()
		m.syncWizard = wiz
		m.screen = syncWizardScreen
		return m, cmd

	case switchScreenMsg:
		m.screen = msg.screen
		switch msg.screen {
		case menuScreen:
			m.menu = newMenuModel(m.styles, m.isDark, lastSyncEntry(m.handler))
			// Feed the fresh menu the current size so it re-derives its layout (e.g. the
			// logo panel); no new WindowSizeMsg is sent just for returning to this screen.
			var cmd tea.Cmd
			m.menu, cmd = m.menu.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			return m, tea.Batch(m.menu.Init(), cmd)
		case syncWizardScreen:
			m.syncWizard = newSyncWizardModel(m.styles, m.handler)
			var cmd tea.Cmd
			m.syncWizard, cmd = m.syncWizard.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			return m, tea.Batch(m.syncWizard.Init(), cmd)
		case userConfigScreen:
			m.userConfig = newUserConfigModel(m.styles, m.isDark, m.handler)
			var cmd tea.Cmd
			m.userConfig, cmd = m.userConfig.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			return m, tea.Batch(m.userConfig.Init(), cmd)
		case syncConfigScreen:
			m.syncConfig = newSyncConfigModel(m.styles)
			var cmd tea.Cmd
			m.syncConfig, cmd = m.syncConfig.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			return m, tea.Batch(m.syncConfig.Init(), cmd)
		case profileScreen:
			m.profiles = newProfileModel(m.styles, m.isDark, m.handler)
			var cmd tea.Cmd
			m.profiles, cmd = m.profiles.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			return m, tea.Batch(m.profiles.Init(), cmd)
		}

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			// If a sync is in flight (or the preview is resolving), let the wizard handle ctrl+c
			// (below) so it cancels that context cleanly instead of the process being torn down
			// mid-write / mid-query.
			busy := m.screen == syncWizardScreen && (m.syncWizard.isRunning() || m.syncWizard.isPreviewLoading())
			if !busy {
				return m, tea.Quit
			}
		}
	}

	var cmd tea.Cmd
	switch m.screen {
	case menuScreen:
		m.menu, cmd = m.menu.Update(msg)
	case syncWizardScreen:
		m.syncWizard, cmd = m.syncWizard.Update(msg)
	case userConfigScreen:
		m.userConfig, cmd = m.userConfig.Update(msg)
	case syncConfigScreen:
		m.syncConfig, cmd = m.syncConfig.Update(msg)
	case profileScreen:
		m.profiles, cmd = m.profiles.Update(msg)
	}
	return m, cmd
}

// View delegates rendering to whichever screen is currently active. In Bubble Tea v2 a View is a
// struct rather than a string, and terminal features are declared on it per-frame instead of being
// set once at program start — hence AltScreen here rather than a tea.WithAltScreen option.
func (m model) View() tea.View {
	v := tea.NewView(m.activeScreenView())
	v.AltScreen = true
	return v
}

// activeScreenView renders the current screen. The screens still render to plain strings.
func (m model) activeScreenView() string {
	switch m.screen {
	case syncWizardScreen:
		return m.syncWizard.View()
	case userConfigScreen:
		return m.userConfig.View()
	case syncConfigScreen:
		return m.syncConfig.View()
	case profileScreen:
		return m.profiles.View()
	default:
		return m.menu.View()
	}
}
