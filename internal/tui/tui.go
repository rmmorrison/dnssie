// Package tui implements the dnssie terminal user interface.
package tui

import (
	tea "charm.land/bubbletea/v2"
)

// Run starts the dnssie TUI and blocks until the user exits.
func Run() error {
	_, err := tea.NewProgram(newApp()).Run()
	return err
}
