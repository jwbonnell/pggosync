package tui

import (
	tea "github.com/charmbracelet/bubbletea"
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
	width      int
	height     int
}

type switchScreenMsg struct {
	screen screen
}

// Run starts the Bubble Tea program in alt-screen mode with the main menu as the initial screen.
func Run(handler *config.UserConfigHandler) error {
	m := model{
		screen:     menuScreen,
		menu:       newMenuModel(lastSyncEntry(handler)),
		syncWizard: newSyncWizardModel(handler),
		userConfig: newUserConfigModel(handler),
		syncConfig: newSyncConfigModel(),
		profiles:   newProfileModel(handler),
		handler:    handler,
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
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

// Init delegates to the menu's Init so the list renders immediately.
func (m model) Init() tea.Cmd {
	return m.menu.Init()
}

// Update routes messages to the active screen; handles window resize, screen switching, and global Ctrl+C.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
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
		wiz := newSyncWizardModelFromProfile(m.handler, msg.profile)
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
			m.menu = newMenuModel(lastSyncEntry(m.handler))
			// Feed the fresh menu the current size so it re-derives its layout (e.g. the
			// logo panel); no new WindowSizeMsg is sent just for returning to this screen.
			var cmd tea.Cmd
			m.menu, cmd = m.menu.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			return m, tea.Batch(m.menu.Init(), cmd)
		case syncWizardScreen:
			m.syncWizard = newSyncWizardModel(m.handler)
			var cmd tea.Cmd
			m.syncWizard, cmd = m.syncWizard.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			return m, tea.Batch(m.syncWizard.Init(), cmd)
		case userConfigScreen:
			m.userConfig = newUserConfigModel(m.handler)
			var cmd tea.Cmd
			m.userConfig, cmd = m.userConfig.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			return m, tea.Batch(m.userConfig.Init(), cmd)
		case syncConfigScreen:
			m.syncConfig = newSyncConfigModel()
			var cmd tea.Cmd
			m.syncConfig, cmd = m.syncConfig.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			return m, tea.Batch(m.syncConfig.Init(), cmd)
		case profileScreen:
			m.profiles = newProfileModel(m.handler)
			var cmd tea.Cmd
			m.profiles, cmd = m.profiles.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			return m, tea.Batch(m.profiles.Init(), cmd)
		}

	case tea.KeyMsg:
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

// View delegates rendering to whichever screen is currently active.
func (m model) View() string {
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
