package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// styles holds every themed style, resolved for the terminal's background.
// Colors with an explicit foreground get a light/dark pair so the UI stays
// legible in both light and dark terminals; styles that rely only on relative
// attributes (Faint, Bold, Underline) or the terminal's default foreground
// need no pair and adapt on their own.
type styles struct {
	accent color.Color // used directly where a bare color is needed

	app       lipgloss.Style
	box       lipgloss.Style
	borderInk lipgloss.Style
	footer    lipgloss.Style

	title    lipgloss.Style
	subtitle lipgloss.Style
	selected lipgloss.Style
	item     lipgloss.Style
	desc     lipgloss.Style
	success  lipgloss.Style
	danger   lipgloss.Style
	group    lipgloss.Style

	tab       lipgloss.Style
	activeTab lipgloss.Style
	tabRule   lipgloss.Style
	tableHead lipgloss.Style
	tableSel  lipgloss.Style
	tableCell lipgloss.Style
}

// newStyles builds the theme for a dark (dark=true) or light terminal. The
// dark values match the original palette; the light values are darkened so
// they keep enough contrast on a white background.
func newStyles(dark bool) styles {
	ld := lipgloss.LightDark(dark)

	// light, dark
	accent := ld(lipgloss.Color("#5A2FC2"), lipgloss.Color("#7D56F4"))
	success := ld(lipgloss.Color("#1A7F37"), lipgloss.Color("#43BF6D"))
	danger := ld(lipgloss.Color("#C0362C"), lipgloss.Color("#E64545"))
	// White reads well on the accent fill in either mode, so it's constant.
	onAccent := lipgloss.Color("#FFFFFF")

	return styles{
		accent: accent,

		app: lipgloss.NewStyle().Padding(1, 2),
		box: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(accent).
			Padding(1, 2),
		borderInk: lipgloss.NewStyle().Foreground(accent),
		footer: lipgloss.NewStyle().
			Faint(true).
			PaddingLeft(2).
			PaddingTop(1),

		title:    lipgloss.NewStyle().Bold(true).Foreground(accent),
		subtitle: lipgloss.NewStyle().Faint(true),
		selected: lipgloss.NewStyle().Bold(true).Foreground(accent),
		item:     lipgloss.NewStyle(),
		desc:     lipgloss.NewStyle().Faint(true),
		success:  lipgloss.NewStyle().Italic(true).Foreground(success),
		danger:   lipgloss.NewStyle().Bold(true).Foreground(danger),
		group:    lipgloss.NewStyle().Bold(true).Underline(true),

		tab: lipgloss.NewStyle().Faint(true).Padding(0, 1),
		activeTab: lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1).
			Foreground(onAccent).
			Background(accent),
		tabRule: lipgloss.NewStyle().Foreground(accent),
		tableHead: lipgloss.NewStyle().
			Bold(true).
			Foreground(accent).
			Padding(0, 1),
		tableSel: lipgloss.NewStyle().
			Bold(true).
			Foreground(onAccent).
			Background(accent).
			Padding(0, 1),
		tableCell: lipgloss.NewStyle().Padding(0, 1),
	}
}

// themeMsg carries a refreshed theme to every sub-model, mirroring how
// tea.WindowSizeMsg is fanned out from the root model.
type themeMsg struct{ st styles }
