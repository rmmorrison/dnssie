package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/rmmorrison/dnssie/internal/config"
	"github.com/rmmorrison/dnssie/internal/store"
)

func viewLines(a app) int {
	return strings.Count(a.View().Content, "\n") + 1
}

// TestFrameNeverExceedsTerminal guards the bug where a screen rendered more
// lines than the terminal, scrolling the frame and corrupting the bottom row.
// No screen, in any state, may render more lines than the terminal height.
func TestFrameNeverExceedsTerminal(t *testing.T) {
	mk := func(n int) []store.Record {
		r := make([]store.Record, n)
		for i := range r {
			r[i] = store.Record{Type: "A", Name: fmt.Sprintf("h%03d.test.", i), Value: "1.1.1.1"}
		}
		return r
	}

	for h := 18; h <= 60; h++ {
		base := newApp()
		mdl, _ := base.Update(tea.WindowSizeMsg{Width: 80, Height: h})
		base = mdl.(app)

		screens := map[string]app{}

		menu := base
		menu.screen = screenMenu
		screens["menu"] = menu

		for name, recs := range map[string][]store.Record{
			"manage-empty": nil,
			"manage-one":   mk(1),
			"manage-full":  mk(80),
		} {
			a := base
			a.screen = screenManage
			mg := browsing(recs)
			mg.width, mg.height, mg.st = 80, h, a.styles
			a.manage = mg
			screens[name] = a

			withErr := a
			mgErr := mg
			mgErr.opErr = fmt.Errorf("disk full")
			withErr.manage = mgErr
			screens[name+"-error"] = withErr
		}

		srv := base
		srv.screen = screenServer
		s := srv.server
		s.cfg = config.Config{Port: 1053, Resolvers: config.Resolvers{
			Mode:     config.ModeManual,
			Upstream: []string{"1.1.1.1:53", "8.8.8.8:53", "9.9.9.9:53"},
		}}
		s.step = serverBrowsing
		s.running = true
		s.srvInfo.Addr = "127.0.0.1:1053"
		for i := 0; i < 120; i++ {
			s.recentQueries = append(s.recentQueries, fmt.Sprintf("12:00:00 q%03d.test. A local", i))
		}
		srv.server = s
		screens["server-running"] = srv

		for name, a := range screens {
			if got := viewLines(a); got > h {
				t.Errorf("%s at height %d rendered %d lines (exceeds terminal)", name, h, got)
			}
		}
	}
}

// TestManageCardStableAcrossStates checks the manage card is the same height
// whether the tab is empty, full, or showing an error banner, so it doesn't
// jump around as the user navigates.
func TestManageCardStableAcrossStates(t *testing.T) {
	mk := func(n int) []store.Record {
		r := make([]store.Record, n)
		for i := range r {
			r[i] = store.Record{Type: "A", Name: fmt.Sprintf("h%03d.test.", i), Value: "1.1.1.1"}
		}
		return r
	}

	for _, h := range []int{24, 30, 40, 55} {
		base := newApp()
		mdl, _ := base.Update(tea.WindowSizeMsg{Width: 80, Height: h})
		base = mdl.(app)

		size := func(recs []store.Record, withErr bool) int {
			a := base
			a.screen = screenManage
			mg := browsing(recs)
			mg.width, mg.height, mg.st = 80, h, a.styles
			if withErr {
				mg.opErr = fmt.Errorf("boom")
			}
			a.manage = mg
			return viewLines(a)
		}

		empty := size(nil, false)
		one := size(mk(1), false)
		full := size(mk(80), false)
		errd := size(mk(1), true)
		if empty != one || one != full || full != errd {
			t.Errorf("height %d: card not stable (empty=%d one=%d full=%d err=%d)",
				h, empty, one, full, errd)
		}
	}
}
