package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// action identifies the choice a user makes on the main menu. Each one will
// eventually open its own screen; for now they are placeholders.
type action int

const (
	actionCreateRecord action = iota
	actionManageRecords
	actionServerStatus
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
	width  int
	height int

	// selected holds the most recent choice so we can show a placeholder
	// until the real screens are wired up.
	selected *menuItem
}

func newMenu() menu {
	return menu{
		items: []menuItem{
			{actionCreateRecord, "Create a new record", "Add a new DNS record"},
			{actionManageRecords, "Manage existing records", "View, edit, or delete records"},
			{actionServerStatus, "DNS server status", "Check whether the DNS server is running"},
			{actionQuit, "Quit", "Exit dnssie"},
		},
	}
}

func (m menu) Init() tea.Cmd {
	return nil
}

func (m menu) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
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

		case "enter", " ":
			choice := m.items[m.cursor]
			if choice.action == actionQuit {
				return m, tea.Quit
			}
			// The real screens aren't built yet, so just record the
			// choice and show a placeholder.
			m.selected = &choice
			return m, nil
		}
	}

	return m, nil
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4"))

	subtitleStyle = lipgloss.NewStyle().
			Faint(true)

	selectedItemStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#7D56F4"))

	itemStyle = lipgloss.NewStyle()

	descStyle = lipgloss.NewStyle().
			Faint(true)

	statusStyle = lipgloss.NewStyle().
			Italic(true).
			Foreground(lipgloss.Color("#43BF6D"))

	helpStyle = lipgloss.NewStyle().
			Faint(true)

	appStyle = lipgloss.NewStyle().
			Padding(1, 2)
)

func (m menu) View() tea.View {
	var b strings.Builder

	b.WriteString(titleStyle.Render("dnssie"))
	b.WriteByte('\n')
	b.WriteString(subtitleStyle.Render("DNS record manager"))
	b.WriteString("\n\n")

	for i, item := range m.items {
		cursor := "  "
		line := itemStyle.Render(item.title)
		if i == m.cursor {
			cursor = selectedItemStyle.Render("> ")
			line = selectedItemStyle.Render(item.title)
		}
		b.WriteString(cursor)
		b.WriteString(line)
		b.WriteByte('\n')
		b.WriteString("  ")
		b.WriteString(descStyle.Render(item.desc))
		b.WriteByte('\n')
	}

	if m.selected != nil {
		b.WriteByte('\n')
		b.WriteString(statusStyle.Render(m.selected.title + " — coming soon"))
		b.WriteByte('\n')
	}

	b.WriteByte('\n')
	b.WriteString(helpStyle.Render("↑/↓: navigate • enter: select • q: quit"))

	v := tea.NewView(appStyle.Render(b.String()))
	v.AltScreen = true
	return v
}
