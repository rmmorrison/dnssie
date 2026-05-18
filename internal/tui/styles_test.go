package tui

import (
	"image/color"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// luminance is a rough perceived-brightness of a color in [0,1].
func luminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	// RGBA returns 16-bit values; scale to [0,1].
	return (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 65535
}

func TestNewStylesAdaptsToBackground(t *testing.T) {
	dark := newStyles(true)
	light := newStyles(false)

	if dark.accent == light.accent {
		t.Fatal("accent should differ between light and dark themes")
	}
	// On a light terminal the accent must be darker (more contrast on white).
	if luminance(light.accent) >= luminance(dark.accent) {
		t.Errorf("light accent (lum %.3f) should be darker than dark accent (lum %.3f)",
			luminance(light.accent), luminance(dark.accent))
	}

	// A title rendered in each theme must carry different color codes.
	if dark.title.Render("x") == light.title.Render("x") {
		t.Error("title styling should differ between themes")
	}
}

func TestBackgroundColorMsgAppliesThemeEverywhere(t *testing.T) {
	a := newApp()
	if !a.hasDark {
		t.Fatal("app should default to the dark theme")
	}

	// A white background reports IsDark()==false -> light theme.
	m, _ := a.Update(tea.BackgroundColorMsg{Color: color.White})
	a = m.(app)

	if a.hasDark {
		t.Error("hasDark should be false after a light-background message")
	}
	light := newStyles(false)
	for name, got := range map[string]styles{
		"app":    a.styles,
		"menu":   a.menu.st,
		"create": a.create.st,
		"manage": a.manage.st,
		"server": a.server.st,
	} {
		if got.accent != light.accent {
			t.Errorf("%s did not receive the light theme: accent %v, want %v",
				name, got.accent, light.accent)
		}
	}

	// Switching back to a dark background restores the dark theme.
	m, _ = a.Update(tea.BackgroundColorMsg{Color: color.Black})
	a = m.(app)
	if !a.hasDark || a.menu.st.accent != newStyles(true).accent {
		t.Error("a dark-background message should restore the dark theme")
	}
}
