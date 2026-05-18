package tui

import (
	"errors"
	"testing"

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

func TestServerToggleSourceWithEnter(t *testing.T) {
	m := browsingServer(manualCfg("1.1.1.1:53"))
	m.cursor = focusSource

	m, _ = m.updateBrowsing(key("enter"))
	if m.cfg.Resolvers.Mode != config.ModeSystem {
		t.Errorf("mode = %q, want system after toggle", m.cfg.Resolvers.Mode)
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

func TestServerToggleToSystemClampsCursor(t *testing.T) {
	m := browsingServer(manualCfg("1.1.1.1:53", "8.8.8.8:53"))
	m.cursor = 4 // the "add" row in manual mode
	m, _ = m.toggleSource()
	if m.cursor >= m.focusCount() {
		t.Errorf("cursor = %d not clamped to focusCount %d", m.cursor, m.focusCount())
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
