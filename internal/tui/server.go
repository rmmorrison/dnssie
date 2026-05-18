package tui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/rmmorrison/dnssie/internal/config"
)

// configLoadedMsg carries the result of reading the server config from disk.
type configLoadedMsg struct {
	cfg config.Config
	err error
}

// systemResolversMsg carries the OS-configured resolvers (or why they
// couldn't be determined).
type systemResolversMsg struct {
	resolvers []string
	err       error
}

// configSavedMsg reports the outcome of persisting the server config.
type configSavedMsg struct {
	err error
}

func loadConfigCmd() tea.Cmd {
	return func() tea.Msg {
		st, err := config.Default()
		if err != nil {
			return configLoadedMsg{cfg: config.Defaults(), err: err}
		}
		cfg, err := st.Load()
		return configLoadedMsg{cfg: cfg, err: err}
	}
}

func systemResolversCmd() tea.Cmd {
	return func() tea.Msg {
		res, err := config.SystemResolvers()
		return systemResolversMsg{resolvers: res, err: err}
	}
}

func saveConfigCmd(cfg config.Config) tea.Cmd {
	return func() tea.Msg {
		st, err := config.Default()
		if err != nil {
			return configSavedMsg{err: err}
		}
		return configSavedMsg{err: st.Save(cfg)}
	}
}

type serverStep int

const (
	serverLoading serverStep = iota
	serverBrowsing
	serverEditPort
	serverEditUpstream
	serverConfirmDelete
	serverSaving
)

// Fixed focusable rows that always precede the (manual-only) upstream list.
const (
	focusPort = iota
	focusSource
	focusFixedCount
)

// server is the screen for configuring the DNS server: listen port and the
// upstream resolvers used when a query doesn't match a local record.
type server struct {
	step       serverStep
	cfg        config.Config
	sysRes     []string
	sysErr     error
	cursor     int  // index into the focusable rows
	upIndex    int  // upstream being edited/deleted
	editingNew bool // adding (vs editing) an upstream
	input      textinput.Model
	loadErr    error
	opErr      error
	editErr    error // transient validation error in an edit step
	width      int
	height     int
}

func newServer() server {
	in := textinput.New()
	in.CharLimit = 64
	return server{step: serverLoading, input: in}
}

func (m server) Init() tea.Cmd {
	return tea.Batch(loadConfigCmd(), systemResolversCmd())
}

func (m server) manual() bool {
	return m.cfg.Resolvers.Mode == config.ModeManual
}

// focusCount is the number of navigable rows: port + source, plus (in manual
// mode) one row per upstream and a trailing "add" row.
func (m server) focusCount() int {
	if m.manual() {
		return focusFixedCount + len(m.cfg.Resolvers.Upstream) + 1
	}
	return focusFixedCount
}

// upstreamAt maps the cursor to an upstream index, if it points at one.
func (m server) upstreamAt() (int, bool) {
	if !m.manual() || m.cursor < focusFixedCount {
		return 0, false
	}
	i := m.cursor - focusFixedCount
	if i < len(m.cfg.Resolvers.Upstream) {
		return i, true
	}
	return 0, false
}

func (m server) onAddRow() bool {
	return m.manual() && m.cursor == focusFixedCount+len(m.cfg.Resolvers.Upstream)
}

func (m *server) clampCursor() {
	if m.cursor >= m.focusCount() {
		m.cursor = max(m.focusCount()-1, 0)
	}
}

func (m server) Update(msg tea.Msg) (server, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.SetWidth(min(msg.Width-8, 60))
		return m, nil

	case configLoadedMsg:
		if msg.err != nil {
			m.loadErr = msg.err
		} else {
			m.loadErr = nil
			m.cfg = msg.cfg
		}
		m.clampCursor()
		m.step = serverBrowsing
		return m, nil

	case systemResolversMsg:
		m.sysRes = msg.resolvers
		m.sysErr = msg.err
		return m, nil

	case configSavedMsg:
		if msg.err != nil {
			m.opErr = msg.err
		} else {
			m.opErr = nil
		}
		// Resync from disk so the view reflects what was persisted.
		m.step = serverLoading
		return m, loadConfigCmd()

	case tea.KeyPressMsg:
		switch m.step {
		case serverBrowsing:
			return m.updateBrowsing(msg)
		case serverEditPort:
			return m.updateEditPort(msg)
		case serverEditUpstream:
			return m.updateEditUpstream(msg)
		case serverConfirmDelete:
			return m.updateConfirmDelete(msg)
		case serverLoading, serverSaving:
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}
	}

	if m.step == serverEditPort || m.step == serverEditUpstream {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m server) updateBrowsing(msg tea.KeyPressMsg) (server, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		return m, changeScreen(screenMenu)
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < m.focusCount()-1 {
			m.cursor++
		}
	case "left", "right", "space":
		if m.cursor == focusSource {
			return m.toggleSource()
		}
	case "a":
		if m.manual() {
			m.editingNew = true
			m.editErr = nil
			m.input.SetValue("")
			m.step = serverEditUpstream
			return m, m.input.Focus()
		}
	case "d":
		if i, ok := m.upstreamAt(); ok {
			m.upIndex = i
			m.opErr = nil
			m.step = serverConfirmDelete
			return m, nil
		}
	case "enter":
		switch {
		case m.cursor == focusPort:
			m.editErr = nil
			m.input.SetValue(strconv.Itoa(m.cfg.Port))
			m.input.CursorEnd()
			m.step = serverEditPort
			return m, m.input.Focus()
		case m.cursor == focusSource:
			return m.toggleSource()
		case m.onAddRow():
			m.editingNew = true
			m.editErr = nil
			m.input.SetValue("")
			m.step = serverEditUpstream
			return m, m.input.Focus()
		default:
			if i, ok := m.upstreamAt(); ok {
				m.editingNew = false
				m.upIndex = i
				m.editErr = nil
				m.input.SetValue(m.cfg.Resolvers.Upstream[i])
				m.input.CursorEnd()
				m.step = serverEditUpstream
				return m, m.input.Focus()
			}
		}
	}
	return m, nil
}

func (m server) toggleSource() (server, tea.Cmd) {
	if m.manual() {
		m.cfg.Resolvers.Mode = config.ModeSystem
	} else {
		m.cfg.Resolvers.Mode = config.ModeManual
	}
	m.clampCursor()
	m.step = serverSaving
	return m, saveConfigCmd(m.cfg)
}

func (m server) updateEditPort(msg tea.KeyPressMsg) (server, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.input.Blur()
		m.editErr = nil
		m.step = serverBrowsing
		return m, nil
	case "enter":
		p, err := strconv.Atoi(strings.TrimSpace(m.input.Value()))
		if err != nil || p < 1 || p > 65535 {
			m.editErr = fmt.Errorf("port must be a number between 1 and 65535")
			return m, nil
		}
		m.input.Blur()
		m.editErr = nil
		m.cfg.Port = p
		m.step = serverSaving
		return m, saveConfigCmd(m.cfg)
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m server) updateEditUpstream(msg tea.KeyPressMsg) (server, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.input.Blur()
		m.editErr = nil
		m.step = serverBrowsing
		return m, nil
	case "enter":
		v := config.NormalizeUpstream(m.input.Value())
		if v == "" {
			m.editErr = fmt.Errorf("enter a resolver address, e.g. 1.1.1.1 or 1.1.1.1:53")
			return m, nil
		}
		m.input.Blur()
		m.editErr = nil
		if m.editingNew {
			m.cfg.Resolvers.Upstream = append(m.cfg.Resolvers.Upstream, v)
		} else {
			m.cfg.Resolvers.Upstream[m.upIndex] = v
		}
		m.step = serverSaving
		return m, saveConfigCmd(m.cfg)
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m server) updateConfirmDelete(msg tea.KeyPressMsg) (server, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "n":
		m.step = serverBrowsing
		return m, nil
	case "enter":
		up := m.cfg.Resolvers.Upstream
		if m.upIndex >= 0 && m.upIndex < len(up) {
			m.cfg.Resolvers.Upstream = append(up[:m.upIndex], up[m.upIndex+1:]...)
		}
		m.clampCursor()
		m.step = serverSaving
		return m, saveConfigCmd(m.cfg)
	}
	return m, nil
}

func (m server) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("DNS server settings"))
	b.WriteString("\n\n")

	switch m.step {
	case serverLoading:
		b.WriteString(subtitleStyle.Render("Loading…"))
		return b.String()
	case serverSaving:
		b.WriteString(subtitleStyle.Render("Saving…"))
		return b.String()

	case serverEditPort:
		b.WriteString("Listen port\n")
		b.WriteString(m.input.View())
		b.WriteString("\n\n")
		if m.editErr != nil {
			b.WriteString(errorStyle.Render(m.editErr.Error()))
			b.WriteString("\n\n")
		}
		b.WriteString(helpStyle.Render("enter: save • esc: cancel"))
		return b.String()

	case serverEditUpstream:
		title := "Edit upstream resolver"
		if m.editingNew {
			title = "Add upstream resolver"
		}
		b.WriteString(subtitleStyle.Render(title))
		b.WriteString("\n\n")
		b.WriteString(m.input.View())
		b.WriteString("\n\n")
		if m.editErr != nil {
			b.WriteString(errorStyle.Render(m.editErr.Error()))
			b.WriteString("\n\n")
		}
		b.WriteString(helpStyle.Render("enter: save • esc: cancel"))
		return b.String()

	case serverConfirmDelete:
		b.WriteString(errorStyle.Render("Delete this upstream resolver? This cannot be undone."))
		b.WriteString("\n\n")
		if m.upIndex >= 0 && m.upIndex < len(m.cfg.Resolvers.Upstream) {
			b.WriteString("  " + m.cfg.Resolvers.Upstream[m.upIndex] + "\n\n")
		}
		b.WriteString(helpStyle.Render("enter: delete • esc: cancel"))
		return b.String()
	}

	// serverBrowsing
	if m.loadErr != nil {
		b.WriteString(errorStyle.Render("Failed to load config: " + m.loadErr.Error()))
		b.WriteString("\n\n")
	}
	if m.opErr != nil {
		b.WriteString(errorStyle.Render("Save failed: " + m.opErr.Error()))
		b.WriteString("\n\n")
	}

	source := "System"
	if m.manual() {
		source = "Manual"
	}
	b.WriteString(row(m.cursor == focusPort, "Listen port", strconv.Itoa(m.cfg.Port)))
	b.WriteString(row(m.cursor == focusSource, "Resolver source", source))
	b.WriteByte('\n')

	if m.manual() {
		b.WriteString(groupStyle.Render("Upstream resolvers"))
		b.WriteByte('\n')
		if len(m.cfg.Resolvers.Upstream) == 0 {
			b.WriteString(subtitleStyle.Render("  (none yet)"))
			b.WriteByte('\n')
		}
		for i, up := range m.cfg.Resolvers.Upstream {
			focused := m.cursor == focusFixedCount+i
			b.WriteString(line(focused, up))
		}
		b.WriteString(line(m.onAddRow(), subtitleStyle.Render("+ add resolver")))
		b.WriteByte('\n')
		b.WriteString(helpStyle.Render("↑/↓: navigate • enter: edit • a: add • d: delete • esc: back"))
	} else {
		b.WriteString(groupStyle.Render("System resolvers"))
		b.WriteByte('\n')
		switch {
		case m.sysErr != nil:
			b.WriteString(subtitleStyle.Render("  Unable to determine system resolvers"))
			b.WriteByte('\n')
		case len(m.sysRes) == 0:
			b.WriteString(subtitleStyle.Render("  (none found)"))
			b.WriteByte('\n')
		default:
			for _, r := range m.sysRes {
				b.WriteString(subtitleStyle.Render("  " + r))
				b.WriteByte('\n')
			}
		}
		b.WriteByte('\n')
		b.WriteString(helpStyle.Render("↑/↓: navigate • enter: edit • space: toggle source • esc: back"))
	}

	return b.String()
}

// row renders a "label  value" settings line with a focus marker.
func row(focused bool, label, value string) string {
	text := fmt.Sprintf("%-16s %s", label, value)
	return line(focused, text)
}

// line renders a single list line with a focus marker.
func line(focused bool, text string) string {
	if focused {
		return selectedItemStyle.Render("> "+text) + "\n"
	}
	return itemStyle.Render("  "+text) + "\n"
}
