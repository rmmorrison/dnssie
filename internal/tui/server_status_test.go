package tui

import (
	"strings"
	"testing"

	"github.com/rmmorrison/dnssie/internal/supervisor"
)

func TestServerStatusRunningView(t *testing.T) {
	m := browsingServer(systemCfg()) // cfg.Port = 53
	m, _ = m.Update(serverStatusMsg{
		running: true,
		info:    supervisor.Info{State: supervisor.State{Addr: "127.0.0.1:5353", Port: 5353}},
	})
	if !m.running {
		t.Fatal("model should be marked running")
	}
	v := m.View()
	if !strings.Contains(v, "running") || !strings.Contains(v, "127.0.0.1:5353") {
		t.Errorf("view missing running status:\n%s", v)
	}
	// cfg.Port (53) != running port (5353) -> restart required.
	if !strings.Contains(v, "restart required") {
		t.Errorf("view missing restart-required banner:\n%s", v)
	}
}

func TestServerStatusStoppedView(t *testing.T) {
	m := browsingServer(systemCfg())
	m, _ = m.Update(serverStatusMsg{running: false})
	v := m.View()
	if !strings.Contains(v, "stopped") {
		t.Errorf("view missing stopped status:\n%s", v)
	}
	if !strings.Contains(v, "start server") {
		t.Errorf("view missing start affordance:\n%s", v)
	}
}

func TestServerStartStopKey(t *testing.T) {
	m := browsingServer(systemCfg())
	m.running = false

	m2, cmd := m.updateBrowsing(key("s"))
	if !m2.busy || cmd == nil {
		t.Errorf("pressing s while stopped: busy=%v cmd=nil:%v, want busy + cmd", m2.busy, cmd == nil)
	}

	// Already busy -> no-op (no second action issued).
	_, cmd2 := m2.updateBrowsing(key("s"))
	if cmd2 != nil {
		t.Error("pressing s while busy should be a no-op")
	}

	// Running -> pressing s issues a (stop) command.
	r := browsingServer(systemCfg())
	r.running = true
	r2, cmd3 := r.updateBrowsing(key("s"))
	if !r2.busy || cmd3 == nil {
		t.Error("pressing s while running should issue a stop command")
	}
}
