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
func titledBox(st styles, title, body string, width int) string {
	box := st.box.Width(width).Render(body)
	lines := strings.Split(box, "\n")
	if len(lines) == 0 {
		return box
	}

	total := lipgloss.Width(lines[0]) // full rendered top-border width
	label := st.title.Render(" " + title + " ")
	// Rebuilt line is "╭─"(2) + label + "─"*dashes + "╮"(1), so to keep the
	// same width as the box: dashes = total - 3 - width(label).
	dashes := total - 3 - lipgloss.Width(label)
	if dashes < 0 {
		dashes = 0
	}
	lines[0] = st.borderInk.Render("╭─") + label +
		st.borderInk.Render(strings.Repeat("─", dashes)+"╮")
	return strings.Join(lines, "\n")
}

// app is the root model. It owns the active screen, the resolved theme, and
// routes messages, window size, and theme changes to the relevant sub-model.
type app struct {
	screen  screen
	menu    menu
	create  createRecord
	manage  manage
	server  server
	styles  styles
	hasDark bool
	width   int
	height  int
}

func newApp() app {
	// Default to the dark palette until the terminal reports its background
	// (via tea.BackgroundColorMsg); this preserves the original look on
	// terminals that don't answer the query.
	st := newStyles(true)
	a := app{
		screen:  screenMenu,
		menu:    newMenu(),
		create:  newCreateRecord(),
		manage:  newManage(),
		server:  newServer(),
		styles:  st,
		hasDark: true,
	}
	a.menu.st = st
	a.create.st = st
	a.manage.st = st
	a.server.st = st
	return a
}

func (a app) Init() tea.Cmd {
	return tea.Batch(
		a.menu.Init(),
		func() tea.Msg { return tea.RequestBackgroundColor() },
	)
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

	case tea.BackgroundColorMsg:
		a.hasDark = msg.IsDark()
		a.styles = newStyles(a.hasDark)
		// Fan the refreshed theme out to every sub-model.
		tm := themeMsg{a.styles}
		a.menu, _ = a.menu.Update(tm)
		a.create, _ = a.create.Update(tm)
		a.manage, _ = a.manage.Update(tm)
		a.server, _ = a.server.Update(tm)
		return a, nil

	case changeScreenMsg:
		a.screen = msg.to
		tm := themeMsg{a.styles}
		switch msg.to {
		case screenCreate:
			// Start each visit to the create screen fresh.
			a.create = newCreateRecord()
			a.create, _ = a.create.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
			a.create, _ = a.create.Update(tm)
			return a, a.create.Init()
		case screenManage:
			// Reload records fresh on each visit.
			a.manage = newManage()
			a.manage, _ = a.manage.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
			a.manage, _ = a.manage.Update(tm)
			return a, a.manage.Init()
		case screenServer:
			// Reload config fresh on each visit.
			a.server = newServer()
			a.server, _ = a.server.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
			a.server, _ = a.server.Update(tm)
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

	card := titledBox(a.styles, "dnssie", body, contentWidth(a.width))
	out := card + "\n" + a.styles.footer.Render(foot)

	v := tea.NewView(a.styles.app.Render(out))
	v.AltScreen = true
	return v
}
