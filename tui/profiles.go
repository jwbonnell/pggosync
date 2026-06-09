package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/jwbonnell/pggosync/config"
)

type profileModel struct {
	list    list.Model
	handler *config.UserConfigHandler
	width   int
	height  int
}

type profileItem struct {
	profile config.SyncProfile
}

func (i profileItem) Title() string { return i.profile.Name }
func (i profileItem) Description() string {
	return fmt.Sprintf("%s → %s · %s", i.profile.Source, i.profile.Dest, i.profile.ConfigFile)
}
func (i profileItem) FilterValue() string { return i.profile.Name }

type launchProfileMsg struct{ profile config.SyncProfile }

func newProfileModel(handler *config.UserConfigHandler) profileModel {
	profiles, _ := handler.LoadProfiles()
	items := make([]list.Item, len(profiles.Profiles))
	for i, p := range profiles.Profiles {
		items[i] = profileItem{profile: p}
	}
	l := list.New(items, newMenuItemDelegate(), 60, 20)
	l.Title = "Sync Profiles"
	l.Styles.Title = titleStyle
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(true)
	return profileModel{list: l, handler: handler}
}

func (m profileModel) Init() tea.Cmd { return nil }

func (m profileModel) Update(msg tea.Msg) (profileModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "enter", " ":
			if item, ok := m.list.SelectedItem().(profileItem); ok {
				return m, func() tea.Msg { return launchProfileMsg{profile: item.profile} }
			}
		case "d":
			if item, ok := m.list.SelectedItem().(profileItem); ok {
				_ = m.handler.DeleteProfile(item.profile.Name)
				fresh := newProfileModel(m.handler)
				h, v := docStyle.GetFrameSize()
				fresh.list.SetSize(m.width-h, m.height-v)
				fresh.width = m.width
				fresh.height = m.height
				return fresh, nil
			}
		case "esc", "q":
			return m, func() tea.Msg { return switchScreenMsg{screen: menuScreen} }
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m profileModel) View() string {
	content := strings.TrimRight(m.list.View(), "\n")
	content += "\n\n" + helpStyle.Render("enter: launch · d: delete")
	return docStyle.Render(content)
}
