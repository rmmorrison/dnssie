package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/rmmorrison/dnssie/internal/config"
)

// browsingServer returns a server model populated with cfg, ready to browse.
func browsingServer(cfg config.Config) server {
	m := newServer()
	m.cfg = cfg
	m.step = serverBrowsing
	return m
}

func systemCfg() config.Config {
	return config.Config{Port: 53, Resolvers: config.Resolvers{Mode: config.ModeSystem}}
}

func manualCfg(ups ...string) config.Config {
	return config.Config{Port: 53, Resolvers: config.Resolvers{Mode: config.ModeManual, Upstream: ups}}
}

func TestServerToggleSourceWithSpace(t *testing.T) {
	m := browsingServer(systemCfg())
	m.cursor = focusSource

	m, cmd := m.updateBrowsing(key("space"))
	if m.cfg.Resolvers.Mode != config.ModeManual {
		t.Errorf("mode = %q, want manual after toggle", m.cfg.Resolvers.Mode)
	}
	if m.step != serverSaving {
		t.Errorf("step = %v, want serverSaving", m.step)
	}
	if cmd == nil {
		t.Error("expected a save command")
	}
}

func TestServerCycleSourceForwardWraps(t *testing.T) {
	m := browsingServer(systemCfg())
	m.cursor = focusSource

	// enter / space / right all advance: system -> manual -> off -> system.
	for _, want := range []string{config.ModeManual, config.ModeOff, config.ModeSystem} {
		m, _ = m.updateBrowsing(key("enter"))
		if m.cfg.Resolvers.Mode != want {
			t.Fatalf("mode = %q, want %q", m.cfg.Resolvers.Mode, want)
		}
		m.step = serverBrowsing // toggle moved us to serverSaving
	}
}

func TestServerCycleSourceLeftGoesBackward(t *testing.T) {
	m := browsingServer(systemCfg())
	m.cursor = focusSource

	m, _ = m.updateBrowsing(key("left")) // system -> off (wrap backward)
	if m.cfg.Resolvers.Mode != config.ModeOff {
		t.Errorf("mode = %q, want off after left from system", m.cfg.Resolvers.Mode)
	}
}

func TestServerEditPortValid(t *testing.T) {
	m := browsingServer(systemCfg())
	m.cursor = focusPort

	m, _ = m.updateBrowsing(key("enter"))
	if m.step != serverEditPort {
		t.Fatalf("step = %v, want serverEditPort", m.step)
	}
	if m.input.Value() != "53" {
		t.Errorf("port input = %q, want prefilled 53", m.input.Value())
	}

	m.input.SetValue("5353")
	m, cmd := m.updateEditPort(key("enter"))
	if m.cfg.Port != 5353 {
		t.Errorf("port = %d, want 5353", m.cfg.Port)
	}
	if m.step != serverSaving || cmd == nil {
		t.Errorf("step = %v cmd nil = %v, want serverSaving + cmd", m.step, cmd == nil)
	}
}

func TestServerEditPortRejectsInvalid(t *testing.T) {
	for _, bad := range []string{"0", "70000", "abc", ""} {
		m := browsingServer(systemCfg())
		m.cursor = focusPort
		m, _ = m.updateBrowsing(key("enter"))

		m.input.SetValue(bad)
		m, cmd := m.updateEditPort(key("enter"))
		if m.step != serverEditPort {
			t.Errorf("input %q: step = %v, want stay in serverEditPort", bad, m.step)
		}
		if m.editErr == nil {
			t.Errorf("input %q: expected a validation error", bad)
		}
		if cmd != nil {
			t.Errorf("input %q: expected no save command", bad)
		}
		if m.cfg.Port != 53 {
			t.Errorf("input %q: port mutated to %d", bad, m.cfg.Port)
		}
	}
}

func TestServerAddUpstream(t *testing.T) {
	m := browsingServer(manualCfg())

	m, _ = m.updateBrowsing(key("a"))
	if m.step != serverEditUpstream || !m.editingNew {
		t.Fatalf("step = %v editingNew = %v, want edit/new", m.step, m.editingNew)
	}

	m.input.SetValue("9.9.9.9")
	m, cmd := m.updateEditUpstream(key("enter"))
	if len(m.cfg.Resolvers.Upstream) != 1 || m.cfg.Resolvers.Upstream[0] != "9.9.9.9:53" {
		t.Errorf("upstream = %v, want [9.9.9.9:53] (normalized)", m.cfg.Resolvers.Upstream)
	}
	if m.step != serverSaving || cmd == nil {
		t.Error("expected serverSaving + save command")
	}
}

func TestServerAddUpstreamRejectsEmpty(t *testing.T) {
	m := browsingServer(manualCfg())
	m, _ = m.updateBrowsing(key("a"))

	m.input.SetValue("   ")
	m, cmd := m.updateEditUpstream(key("enter"))
	if m.step != serverEditUpstream || m.editErr == nil || cmd != nil {
		t.Error("empty upstream should be rejected with an error and no save")
	}
	if len(m.cfg.Resolvers.Upstream) != 0 {
		t.Errorf("upstream = %v, want empty", m.cfg.Resolvers.Upstream)
	}
}

func TestServerEditExistingUpstream(t *testing.T) {
	m := browsingServer(manualCfg("1.1.1.1:53", "8.8.8.8:53"))
	m.cursor = focusFixedCount + 1 // second upstream

	m, _ = m.updateBrowsing(key("enter"))
	if m.step != serverEditUpstream || m.editingNew {
		t.Fatalf("step = %v editingNew = %v, want edit/existing", m.step, m.editingNew)
	}
	if m.input.Value() != "8.8.8.8:53" {
		t.Errorf("input = %q, want prefilled 8.8.8.8:53", m.input.Value())
	}

	m.input.SetValue("9.9.9.9:53")
	m, _ = m.updateEditUpstream(key("enter"))
	if m.cfg.Resolvers.Upstream[1] != "9.9.9.9:53" {
		t.Errorf("upstream[1] = %q, want 9.9.9.9:53", m.cfg.Resolvers.Upstream[1])
	}
}

func TestServerDeleteUpstream(t *testing.T) {
	m := browsingServer(manualCfg("1.1.1.1:53", "8.8.8.8:53"))
	m.cursor = focusFixedCount // first upstream

	m, _ = m.updateBrowsing(key("d"))
	if m.step != serverConfirmDelete {
		t.Fatalf("step = %v, want serverConfirmDelete", m.step)
	}

	m, cmd := m.updateConfirmDelete(key("enter"))
	if len(m.cfg.Resolvers.Upstream) != 1 || m.cfg.Resolvers.Upstream[0] != "8.8.8.8:53" {
		t.Errorf("upstream = %v, want [8.8.8.8:53]", m.cfg.Resolvers.Upstream)
	}
	if m.step != serverSaving || cmd == nil {
		t.Error("expected serverSaving + save command")
	}
}

func TestServerDeleteUpstreamCancel(t *testing.T) {
	m := browsingServer(manualCfg("1.1.1.1:53"))
	m.cursor = focusFixedCount

	m, _ = m.updateBrowsing(key("d"))
	m, _ = m.updateConfirmDelete(key("esc"))
	if m.step != serverBrowsing {
		t.Errorf("step = %v, want serverBrowsing", m.step)
	}
	if len(m.cfg.Resolvers.Upstream) != 1 {
		t.Errorf("upstream = %v, want unchanged", m.cfg.Resolvers.Upstream)
	}
}

func TestServerNavigationClamps(t *testing.T) {
	m := browsingServer(manualCfg("1.1.1.1:53"))
	// rows: port(0), source(1), upstream(2), add(3) => 4 rows.
	for i := 0; i < 10; i++ {
		m, _ = m.updateBrowsing(key("down"))
	}
	if m.cursor != 3 {
		t.Errorf("cursor = %d, want clamped at 3", m.cursor)
	}
	for i := 0; i < 10; i++ {
		m, _ = m.updateBrowsing(key("up"))
	}
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want clamped at 0", m.cursor)
	}
}

func TestServerCycleSourceClampsCursor(t *testing.T) {
	m := browsingServer(manualCfg("1.1.1.1:53", "8.8.8.8:53"))
	m.cursor = 4 // the "add" row, only reachable in manual mode
	m, _ = m.cycleSource(1)
	if m.cursor >= m.focusCount() {
		t.Errorf("cursor = %d not clamped to focusCount %d", m.cursor, m.focusCount())
	}
}

func TestServerOffModeView(t *testing.T) {
	m := browsingServer(config.Config{
		Port:      53,
		Resolvers: config.Resolvers{Mode: config.ModeOff},
	})
	out := m.View()
	for _, want := range []string{
		"Resolver source", "Off",
		"Unmatched queries return NXDOMAIN; nothing is forwarded.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Off-mode view missing %q\n%s", want, out)
		}
	}
}

func TestServerConfigLoadedPopulates(t *testing.T) {
	m := newServer()
	m, _ = m.Update(configLoadedMsg{cfg: manualCfg("1.1.1.1:53")})
	if m.step != serverBrowsing {
		t.Fatalf("step = %v, want serverBrowsing", m.step)
	}
	if !m.manual() || len(m.cfg.Resolvers.Upstream) != 1 {
		t.Errorf("cfg not populated: %+v", m.cfg)
	}
}

func TestServerConfigLoadError(t *testing.T) {
	m := newServer()
	m, _ = m.Update(configLoadedMsg{err: errors.New("boom")})
	if m.loadErr == nil {
		t.Error("loadErr should be set")
	}
	if m.step != serverBrowsing {
		t.Errorf("step = %v, want serverBrowsing so the error renders", m.step)
	}
}

func TestServerSystemResolversMsg(t *testing.T) {
	m := newServer()
	m, _ = m.Update(systemResolversMsg{resolvers: []string{"1.1.1.1:53"}})
	if len(m.sysRes) != 1 || m.sysRes[0] != "1.1.1.1:53" {
		t.Errorf("sysRes = %v, want [1.1.1.1:53]", m.sysRes)
	}

	m, _ = m.Update(systemResolversMsg{err: config.ErrSystemResolversUnavailable})
	if m.sysErr == nil {
		t.Error("sysErr should be set")
	}
}

func TestServerBodyBudget(t *testing.T) {
	cases := map[int]int{
		0:  15, // no size yet -> default
		25: 15, // 25 - 10
		40: 30, // 40 - 10
	}
	for h, want := range cases {
		m := newServer()
		m.height = h
		if got := m.bodyBudget(); got != want {
			t.Errorf("bodyBudget(height=%d) = %d, want %d", h, got, want)
		}
	}
}

// runningServerApp returns a root app on the server screen, sized w x h, with a
// running server that has logged nLookups queries.
func runningServerApp(t *testing.T, w, h, nLookups int) app {
	t.Helper()
	a := newApp()
	mdl, _ := a.Update(tea.WindowSizeMsg{Width: w, Height: h})
	a = mdl.(app)
	a.screen = screenServer

	s := a.server
	s.cfg = config.Config{Port: 1053, Resolvers: config.Resolvers{Mode: config.ModeSystem}}
	s.step = serverBrowsing
	s.running = true
	s.srvInfo.Addr = "127.0.0.1:1053"
	s.sysRes = []string{"1.1.1.1:53", "8.8.8.8:53"}
	for i := 0; i < nLookups; i++ {
		s.recentQueries = append(s.recentQueries,
			fmt.Sprintf("12:00:%02d  q%02d.test. A local", i%60, i))
	}
	a.server = s
	return a
}

func TestServerRecentLookupsNeverOverflowTerminal(t *testing.T) {
	for _, h := range []int{20, 25, 30, 40, 60} {
		a := runningServerApp(t, 90, h, 80)
		out := a.View().Content
		if got := strings.Count(out, "\n") + 1; got > h {
			t.Errorf("terminal height %d: rendered %d lines (overflow):\n%s", h, got, out)
		}
	}
}

func TestServerRecentLookupsTailWithIndicator(t *testing.T) {
	a := runningServerApp(t, 90, 40, 80) // q0..q79; tall enough for several rows
	out := a.server.View()

	if !strings.Contains(out, "q79.test.") {
		t.Errorf("newest lookup q79 missing:\n%s", out)
	}
	if strings.Contains(out, "q00.test.") {
		t.Errorf("oldest lookup q00 should be hidden:\n%s", out)
	}
	if !strings.Contains(out, "earlier lookups hidden") {
		t.Errorf("missing 'earlier lookups hidden' indicator:\n%s", out)
	}
	if rows := strings.Count(out, ".test."); rows > a.server.bodyBudget() || rows < 1 {
		t.Errorf("rendered %d lookup rows, want between 1 and budget %d",
			rows, a.server.bodyBudget())
	}

	// Most recent first: q79 must appear above an older visible entry, and
	// the "earlier hidden" indicator sits at the bottom of the list.
	iNewest := strings.Index(out, "q79.test.")
	iOlder := strings.Index(out, "q78.test.")
	iHidden := strings.Index(out, "earlier lookups hidden")
	if iNewest < 0 || iOlder < 0 || iNewest > iOlder {
		t.Errorf("expected q79 to render above q78 (descending order):\n%s", out)
	}
	if iHidden < iOlder {
		t.Errorf("'earlier lookups hidden' should be below the entries:\n%s", out)
	}
}

func TestServerRecentLookupsGrowWithTerminal(t *testing.T) {
	small := runningServerApp(t, 90, 24, 80)
	large := runningServerApp(t, 90, 50, 80)

	rs := strings.Count(small.server.View(), ".test.")
	rl := strings.Count(large.server.View(), ".test.")
	if rl <= rs {
		t.Errorf("a taller terminal should show more lookups: 24-row=%d, 50-row=%d", rs, rl)
	}
}
