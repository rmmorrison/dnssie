package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/rmmorrison/dnssie/internal/config"
	"github.com/rmmorrison/dnssie/internal/supervisor"
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

// serverStatusMsg carries the detached server's current status.
type serverStatusMsg struct {
	running bool
	info    supervisor.Info
	err     error
}

// serverActionMsg reports the result of a start/stop request.
type serverActionMsg struct{ err error }

// statusTickMsg drives periodic status polling while the screen is open.
type statusTickMsg struct{}

// recentQueriesMsg carries the latest lookups the running server logged.
type recentQueriesMsg struct{ lines []string }

// maxRecentShown bounds the live lookup list rendered on the screen.
const maxRecentShown = 10

func recentQueriesCmd() tea.Cmd {
	return func() tea.Msg {
		lines, _ := supervisor.RecentQueries(maxRecentShown)
		return recentQueriesMsg{lines: lines}
	}
}

func serverStatusCmd() tea.Cmd {
	return func() tea.Msg {
		running, info, err := supervisor.Status()
		return serverStatusMsg{running: running, info: info, err: err}
	}
}

func startServerCmd(cfg config.Config) tea.Cmd {
	return func() tea.Msg { return serverActionMsg{err: supervisor.Start(cfg)} }
}

func stopServerCmd() tea.Cmd {
	return func() tea.Msg { return serverActionMsg{err: supervisor.Stop()} }
}

func statusTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return statusTickMsg{} })
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

	running       bool
	srvInfo       supervisor.Info
	statusErr     error
	busy          bool     // a start/stop request is in flight
	actionErr     error    // last start/stop failure
	recentQueries []string // live lookups while the server runs

	st     styles
	width  int
	height int
}

func newServer() server {
	in := textinput.New()
	in.CharLimit = 64
	return server{step: serverLoading, input: in, st: newStyles(true)}
}

func (m server) Init() tea.Cmd {
	return tea.Batch(loadConfigCmd(), systemResolversCmd(),
		serverStatusCmd(), recentQueriesCmd(), statusTickCmd())
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

	case themeMsg:
		m.st = msg.st
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

	case serverStatusMsg:
		m.running = msg.running
		m.srvInfo = msg.info
		m.statusErr = msg.err
		m.busy = false
		if !msg.running {
			m.recentQueries = nil // don't show a stopped server's history
		}
		return m, nil

	case recentQueriesMsg:
		m.recentQueries = msg.lines
		return m, nil

	case serverActionMsg:
		m.busy = false
		m.actionErr = msg.err
		return m, serverStatusCmd() // refresh immediately

	case statusTickMsg:
		// Keep the heartbeat alive across all steps; only probe while
		// browsing and idle to avoid redundant work.
		cmds := []tea.Cmd{statusTickCmd()}
		if m.step == serverBrowsing && !m.busy {
			cmds = append(cmds, serverStatusCmd(), recentQueriesCmd())
		}
		return m, tea.Batch(cmds...)

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
	case "left":
		if m.cursor == focusSource {
			return m.cycleSource(-1)
		}
	case "right", "space":
		if m.cursor == focusSource {
			return m.cycleSource(1)
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
	case "s":
		if m.busy {
			return m, nil
		}
		m.busy = true
		m.actionErr = nil
		if m.running {
			return m, stopServerCmd()
		}
		return m, startServerCmd(m.cfg)
	case "enter":
		switch {
		case m.cursor == focusPort:
			m.editErr = nil
			m.input.SetValue(strconv.Itoa(m.cfg.Port))
			m.input.CursorEnd()
			m.step = serverEditPort
			return m, m.input.Focus()
		case m.cursor == focusSource:
			return m.cycleSource(1)
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

// sourceModes is the cycle order for the "Resolver source" setting.
var sourceModes = []string{config.ModeSystem, config.ModeManual, config.ModeOff}

// cycleSource advances the resolver source by delta (wrapping) and persists it.
func (m server) cycleSource(delta int) (server, tea.Cmd) {
	idx := 0
	for i, mode := range sourceModes {
		if m.cfg.Resolvers.Mode == mode {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(sourceModes)) % len(sourceModes)
	m.cfg.Resolvers.Mode = sourceModes[idx]
	m.clampCursor()
	m.step = serverSaving
	return m, saveConfigCmd(m.cfg)
}

// toggleSource advances to the next resolver source. Retained for callers and
// tests that don't care about direction.
func (m server) toggleSource() (server, tea.Cmd) { return m.cycleSource(1) }

// sourceLabel is the display name for the current resolver source.
func (m server) sourceLabel() string {
	switch m.cfg.Resolvers.Mode {
	case config.ModeManual:
		return "Manual"
	case config.ModeOff:
		return "Off"
	default:
		return "System"
	}
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
	b.WriteString(m.st.title.Render("DNS server"))
	b.WriteString("\n\n")

	switch m.step {
	case serverLoading:
		b.WriteString(m.st.subtitle.Render("Loading…"))
		return b.String()
	case serverSaving:
		b.WriteString(m.st.subtitle.Render("Saving…"))
		return b.String()

	case serverEditPort:
		b.WriteString("Listen port\n")
		b.WriteString(m.input.View())
		if m.editErr != nil {
			b.WriteString("\n\n")
			b.WriteString(m.st.danger.Render(m.editErr.Error()))
		}
		return b.String()

	case serverEditUpstream:
		title := "Edit upstream resolver"
		if m.editingNew {
			title = "Add upstream resolver"
		}
		b.WriteString(m.st.subtitle.Render(title))
		b.WriteString("\n\n")
		b.WriteString(m.input.View())
		if m.editErr != nil {
			b.WriteString("\n\n")
			b.WriteString(m.st.danger.Render(m.editErr.Error()))
		}
		return b.String()

	case serverConfirmDelete:
		b.WriteString(m.st.danger.Render("Delete this upstream resolver? This cannot be undone."))
		b.WriteString("\n\n")
		if m.upIndex >= 0 && m.upIndex < len(m.cfg.Resolvers.Upstream) {
			b.WriteString("  " + m.cfg.Resolvers.Upstream[m.upIndex])
		}
		return b.String()
	}

	// serverBrowsing
	if m.loadErr != nil {
		b.WriteString(m.st.danger.Render("Failed to load config: " + m.loadErr.Error()))
		b.WriteString("\n\n")
	}
	if m.opErr != nil {
		b.WriteString(m.st.danger.Render("Save failed: " + m.opErr.Error()))
		b.WriteString("\n\n")
	}

	// Server status + start/stop control.
	switch {
	case m.statusErr != nil:
		b.WriteString(m.st.danger.Render("Status unavailable: " + m.statusErr.Error()))
		b.WriteByte('\n')
	case m.running:
		b.WriteString(m.st.success.Render("● running") + "  " + m.srvInfo.Addr + "\n")
		if !m.srvInfo.Started.IsZero() {
			b.WriteString(m.st.subtitle.Render("  started "+
				m.srvInfo.Started.Format("2006-01-02 15:04:05")) + "\n")
		}
	default:
		b.WriteString(m.st.subtitle.Render("○ stopped") + "\n")
	}
	if m.busy {
		b.WriteString(m.st.subtitle.Render("  working…") + "\n")
	}
	if m.actionErr != nil {
		b.WriteString(m.st.danger.Render("  "+m.actionErr.Error()) + "\n")
	}
	if m.running && m.srvInfo.Port != 0 && m.srvInfo.Port != m.cfg.Port {
		b.WriteString(m.st.danger.Render(fmt.Sprintf(
			"  ⚠ restart required: running on :%d, config is :%d",
			m.srvInfo.Port, m.cfg.Port)) + "\n")
	}
	act := "start"
	if m.running {
		act = "stop"
	}
	b.WriteString(m.st.item.Render(fmt.Sprintf("  [ s ] %s server", act)) + "\n\n")

	b.WriteString(row(m.st, m.cursor == focusPort, "Listen port", strconv.Itoa(m.cfg.Port)))
	b.WriteString(row(m.st, m.cursor == focusSource, "Resolver source", m.sourceLabel()))
	b.WriteByte('\n')

	switch m.cfg.Resolvers.Mode {
	case config.ModeManual:
		b.WriteString(m.st.group.Render("Upstream resolvers"))
		b.WriteByte('\n')
		if len(m.cfg.Resolvers.Upstream) == 0 {
			b.WriteString(m.st.subtitle.Render("  (none yet)"))
			b.WriteByte('\n')
		}
		for i, up := range m.cfg.Resolvers.Upstream {
			focused := m.cursor == focusFixedCount+i
			b.WriteString(line(m.st, focused, up))
		}
		b.WriteString(line(m.st, m.onAddRow(), m.st.subtitle.Render("+ add resolver")))
	case config.ModeOff:
		b.WriteString(m.st.group.Render("Forwarding disabled"))
		b.WriteByte('\n')
		b.WriteString(m.st.subtitle.Render(
			"  Unmatched queries return NXDOMAIN; nothing is forwarded."))
		b.WriteByte('\n')
	default:
		b.WriteString(m.st.group.Render("System resolvers"))
		b.WriteByte('\n')
		switch {
		case m.sysErr != nil:
			b.WriteString(m.st.subtitle.Render("  Unable to determine system resolvers"))
			b.WriteByte('\n')
		case len(m.sysRes) == 0:
			b.WriteString(m.st.subtitle.Render("  (none found)"))
			b.WriteByte('\n')
		default:
			for _, r := range m.sysRes {
				b.WriteString(m.st.subtitle.Render("  " + r))
				b.WriteByte('\n')
			}
		}
	}

	if m.running {
		b.WriteByte('\n')
		b.WriteString(m.st.group.Render("Recent lookups"))
		b.WriteByte('\n')
		if len(m.recentQueries) == 0 {
			b.WriteString(m.st.subtitle.Render("  (waiting for queries…)"))
			b.WriteByte('\n')
		} else {
			w := contentWidth(m.width) - 2
			for _, q := range m.recentQueries {
				b.WriteString("  ")
				b.WriteString(m.st.subtitle.Render(clip(q, w)))
				b.WriteByte('\n')
			}
		}
	}

	return b.String()
}

// clip truncates s to at most w display columns, adding an ellipsis when cut.
func clip(s string, w int) string {
	if w < 1 {
		w = 1
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return string(r[:w-1]) + "…"
}

func (m server) footer() string {
	switch m.step {
	case serverLoading, serverSaving:
		return ""
	case serverEditPort, serverEditUpstream:
		return "enter save · esc cancel"
	case serverConfirmDelete:
		return "enter delete · esc cancel"
	}
	if m.loadErr != nil {
		return "esc back"
	}

	// Browsing: tailor the hint to the highlighted row so it only advertises
	// keys that actually do something there.
	parts := []string{"↑/↓ navigate"}
	switch {
	case m.cursor == focusPort:
		parts = append(parts, "enter edit port")
	case m.cursor == focusSource:
		parts = append(parts, "←/→ change source")
	case m.onAddRow():
		parts = append(parts, "enter add resolver")
	default: // an upstream entry (manual mode)
		parts = append(parts, "enter edit", "d delete")
	}
	if m.manual() && !m.onAddRow() {
		parts = append(parts, "a add")
	}
	parts = append(parts, "s start/stop", "esc back")
	return strings.Join(parts, " · ")
}

// row renders a "label  value" settings line with a focus marker.
func row(st styles, focused bool, label, value string) string {
	text := fmt.Sprintf("%-16s %s", label, value)
	return line(st, focused, text)
}

// line renders a single list line with a focus marker.
func line(st styles, focused bool, text string) string {
	if focused {
		return st.selected.Render("▌ "+text) + "\n"
	}
	return st.item.Render("  "+text) + "\n"
}
