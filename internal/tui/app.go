package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// screen identifies which view the app is currently showing.
type screen int

const (
	screenMenu screen = iota
	screenCreate
	screenManage
)

// changeScreenMsg asks the app to switch to a different screen. Sub-models
// emit it via changeScreen so the root model owns all navigation.
type changeScreenMsg struct{ to screen }

func changeScreen(to screen) tea.Cmd {
	return func() tea.Msg { return changeScreenMsg{to} }
}

var appStyle = lipgloss.NewStyle().Padding(1, 2)

// app is the root model. It owns the active screen and routes messages and
// window size to the relevant sub-model.
type app struct {
	screen screen
	menu   menu
	create createRecord
	manage manage
	width  int
	height int
}

func newApp() app {
	return app{
		screen: screenMenu,
		menu:   newMenu(),
		create: newCreateRecord(),
		manage: newManage(),
	}
}

func (a app) Init() tea.Cmd {
	return a.menu.Init()
}

func (a app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		// Keep sub-models sized even while inactive.
		a.menu, _ = a.menu.Update(msg)
		a.create, _ = a.create.Update(msg)
		a.manage, _ = a.manage.Update(msg)
		return a, nil

	case changeScreenMsg:
		a.screen = msg.to
		switch msg.to {
		case screenCreate:
			// Start each visit to the create screen fresh.
			a.create = newCreateRecord()
			a.create, _ = a.create.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
			return a, a.create.Init()
		case screenManage:
			// Reload records fresh on each visit.
			a.manage = newManage()
			a.manage, _ = a.manage.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
			return a, a.manage.Init()
		case screenMenu:
			return a, a.menu.Init()
		}
		return a, nil
	}

	var cmd tea.Cmd
	switch a.screen {
	case screenMenu:
		a.menu, cmd = a.menu.Update(msg)
	case screenCreate:
		a.create, cmd = a.create.Update(msg)
	case screenManage:
		a.manage, cmd = a.manage.Update(msg)
	}
	return a, cmd
}

func (a app) View() tea.View {
	var content string
	switch a.screen {
	case screenMenu:
		content = a.menu.View()
	case screenCreate:
		content = a.create.View()
	case screenManage:
		content = a.manage.View()
	}

	v := tea.NewView(appStyle.Render(content))
	v.AltScreen = true
	return v
}
