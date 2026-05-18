package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// screen identifies which view the app is currently showing.
type screen int

const (
	screenMenu screen = iota
	screenCreate
	screenManage
	screenServer
)

// changeScreenMsg asks the app to switch to a different screen. Sub-models
// emit it via changeScreen so the root model owns all navigation.
type changeScreenMsg struct{ to screen }

func changeScreen(to screen) tea.Cmd {
	return func() tea.Msg { return changeScreenMsg{to} }
}

// accent is dnssie's primary brand color, used for the frame and title.
var accent = lipgloss.Color("#7D56F4")

var (
	appStyle = lipgloss.NewStyle().Padding(1, 2)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(accent).
			Padding(1, 2)

	borderInk = lipgloss.NewStyle().Foreground(accent)

	footerStyle = lipgloss.NewStyle().
			Faint(true).
			PaddingLeft(2).
			PaddingTop(1)
)

const maxContentWidth = 72

// contentWidth is the usable text width inside the card, derived from the
// terminal width with a sane cap and floor.
func contentWidth(termWidth int) int {
	if termWidth <= 0 {
		return maxContentWidth
	}
	w := termWidth - 10 // outer padding + border + inner padding
	if w > maxContentWidth {
		w = maxContentWidth
	}
	if w < 24 {
		w = 24
	}
	return w
}

// titledBox renders body inside a rounded border whose top edge carries the
// given title, e.g. ╭─ dnssie ───────╮.
func titledBox(title, body string, width int) string {
	box := boxStyle.Width(width).Render(body)
	lines := strings.Split(box, "\n")
	if len(lines) == 0 {
		return box
	}

	total := lipgloss.Width(lines[0]) // full rendered top-border width
	label := titleStyle.Render(" " + title + " ")
	// Rebuilt line is "╭─"(2) + label + "─"*dashes + "╮"(1), so to keep the
	// same width as the box: dashes = total - 3 - width(label).
	dashes := total - 3 - lipgloss.Width(label)
	if dashes < 0 {
		dashes = 0
	}
	lines[0] = borderInk.Render("╭─") + label +
		borderInk.Render(strings.Repeat("─", dashes)+"╮")
	return strings.Join(lines, "\n")
}

// app is the root model. It owns the active screen and routes messages and
// window size to the relevant sub-model.
type app struct {
	screen screen
	menu   menu
	create createRecord
	manage manage
	server server
	width  int
	height int
}

func newApp() app {
	return app{
		screen: screenMenu,
		menu:   newMenu(),
		create: newCreateRecord(),
		manage: newManage(),
		server: newServer(),
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
		a.server, _ = a.server.Update(msg)
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
		case screenServer:
			// Reload config fresh on each visit.
			a.server = newServer()
			a.server, _ = a.server.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
			return a, a.server.Init()
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
	case screenServer:
		a.server, cmd = a.server.Update(msg)
	}
	return a, cmd
}

func (a app) View() tea.View {
	var body, foot string
	switch a.screen {
	case screenMenu:
		body, foot = a.menu.View(), a.menu.footer()
	case screenCreate:
		body, foot = a.create.View(), a.create.footer()
	case screenManage:
		body, foot = a.manage.View(), a.manage.footer()
	case screenServer:
		body, foot = a.server.View(), a.server.footer()
	}

	card := titledBox("dnssie", body, contentWidth(a.width))
	out := card + "\n" + footerStyle.Render(foot)

	v := tea.NewView(appStyle.Render(out))
	v.AltScreen = true
	return v
}
