package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// action identifies the choice a user makes on the main menu.
type action int

const (
	actionCreateRecord action = iota
	actionManageRecords
	actionServer
	actionQuit
)

// menuItem is a single selectable entry on the main menu.
type menuItem struct {
	action action
	title  string
	desc   string
}

// menu is the landing screen shown when dnssie starts. It lets the user pick
// what they want to do.
type menu struct {
	items  []menuItem
	cursor int
	st     styles
	width  int
	height int
}

func newMenu() menu {
	return menu{
		st: newStyles(true),
		items: []menuItem{
			{actionCreateRecord, "Create a new record", "Add a new DNS record"},
			{actionManageRecords, "Manage existing records", "View, edit, or delete records"},
			{actionServer, "DNS server", "Start/stop the server and configure port and resolvers"},
			{actionQuit, "Quit", "Exit dnssie"},
		},
	}
}

func (m menu) Init() tea.Cmd {
	return nil
}

func (m menu) Update(msg tea.Msg) (menu, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case themeMsg:
		m.st = msg.st
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
			return m, nil

		case "enter", "space":
			choice := m.items[m.cursor]
			switch choice.action {
			case actionQuit:
				return m, tea.Quit
			case actionCreateRecord:
				return m, changeScreen(screenCreate)
			case actionManageRecords:
				return m, changeScreen(screenManage)
			case actionServer:
				return m, changeScreen(screenServer)
			}
		}
	}

	return m, nil
}

func (m menu) View() string {
	var b strings.Builder

	b.WriteString(m.st.subtitle.Render("dev-friendly DNS server"))
	b.WriteString("\n\n")

	for i, item := range m.items {
		if i > 0 {
			b.WriteByte('\n')
		}
		if i == m.cursor {
			b.WriteString(m.st.selected.Render("▌ " + item.title))
		} else {
			b.WriteString(m.st.item.Render("  " + item.title))
		}
		b.WriteByte('\n')
		b.WriteString("  ")
		b.WriteString(m.st.desc.Render(item.desc))
		b.WriteByte('\n')
	}

	return b.String()
}

func (m menu) footer() string {
	return "↑/↓ navigate · enter select · q quit"
}
