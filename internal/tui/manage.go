package tui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/rmmorrison/dnssie/internal/store"
)

// recordsLoadedMsg carries the result of reading every record from disk.
type recordsLoadedMsg struct {
	records []store.Record
	err     error
}

// recordsSavedMsg reports the outcome of persisting the full record set after
// an edit or delete.
type recordsSavedMsg struct {
	err error
}

func loadRecordsCmd() tea.Cmd {
	return func() tea.Msg {
		st, err := store.Default()
		if err != nil {
			return recordsLoadedMsg{err: err}
		}
		records, err := st.Load()
		return recordsLoadedMsg{records: records, err: err}
	}
}

func saveRecordsCmd(records []store.Record) tea.Cmd {
	return func() tea.Msg {
		st, err := store.Default()
		if err != nil {
			return recordsSavedMsg{err: err}
		}
		return recordsSavedMsg{err: st.Save(records)}
	}
}

// typeRank orders record types the same way the create screen lists them, so
// the manage view groups them in a familiar order. Unknown types sort last.
func typeRank(t string) int {
	for i, rt := range supportedTypes {
		if rt.name == t {
			return i
		}
	}
	return len(supportedTypes)
}

type manageStep int

const (
	manageLoading manageStep = iota
	manageBrowsing
	manageEditingName
	manageEditingValue
	manageConfirmDelete
	manageSaving
)

// manage is the screen for browsing persisted records grouped by type, with
// editing and (confirmed) deletion of the highlighted record.
type manage struct {
	step    manageStep
	records []store.Record // canonical set as loaded from disk
	order   []int          // indices into records, grouped by type then name
	cursor  int            // index into order
	name    textinput.Model
	value   textinput.Model
	loadErr error
	opErr   error // error from the last edit/delete save
	width   int
	height  int
}

func newManage() manage {
	name := textinput.New()
	name.CharLimit = 253

	value := textinput.New()
	value.CharLimit = 512

	return manage{
		step:  manageLoading,
		name:  name,
		value: value,
	}
}

func (m manage) Init() tea.Cmd {
	return loadRecordsCmd()
}

// rebuildOrder recomputes the grouped display order and clamps the cursor.
func (m *manage) rebuildOrder() {
	m.order = make([]int, len(m.records))
	for i := range m.records {
		m.order[i] = i
	}
	sort.SliceStable(m.order, func(a, b int) bool {
		ra, rb := m.records[m.order[a]], m.records[m.order[b]]
		if ra.Type != rb.Type {
			if tra, trb := typeRank(ra.Type), typeRank(rb.Type); tra != trb {
				return tra < trb
			}
			return ra.Type < rb.Type
		}
		if ra.Name != rb.Name {
			return ra.Name < rb.Name
		}
		return ra.Value < rb.Value
	})
	if m.cursor >= len(m.order) {
		m.cursor = max(len(m.order)-1, 0)
	}
}

// selected returns the highlighted record's index into m.records, or false if
// there is nothing to select.
func (m manage) selected() (int, bool) {
	if m.step == manageLoading || len(m.order) == 0 {
		return 0, false
	}
	return m.order[m.cursor], true
}

func (m manage) Update(msg tea.Msg) (manage, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		w := min(msg.Width-8, 60)
		m.name.SetWidth(w)
		m.value.SetWidth(w)
		return m, nil

	case recordsLoadedMsg:
		if msg.err != nil {
			m.loadErr = msg.err
			m.records, m.order = nil, nil
			m.step = manageBrowsing
			return m, nil
		}
		m.loadErr = nil
		m.records = msg.records
		m.rebuildOrder()
		m.step = manageBrowsing
		return m, nil

	case recordsSavedMsg:
		if msg.err != nil {
			m.opErr = msg.err
		} else {
			m.opErr = nil
		}
		// Resync from disk so the view reflects what was actually persisted.
		m.step = manageLoading
		return m, loadRecordsCmd()

	case tea.KeyPressMsg:
		switch m.step {
		case manageBrowsing:
			return m.updateBrowsing(msg)
		case manageEditingName:
			return m.updateEditName(msg)
		case manageEditingValue:
			return m.updateEditValue(msg)
		case manageConfirmDelete:
			return m.updateConfirmDelete(msg)
		case manageLoading, manageSaving:
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}
	}

	// Keep the focused text input ticking (e.g. cursor blink).
	var cmd tea.Cmd
	switch m.step {
	case manageEditingName:
		m.name, cmd = m.name.Update(msg)
	case manageEditingValue:
		m.value, cmd = m.value.Update(msg)
	}
	return m, cmd
}

func (m manage) updateBrowsing(msg tea.KeyPressMsg) (manage, tea.Cmd) {
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
		if m.cursor < len(m.order)-1 {
			m.cursor++
		}
	case "e":
		idx, ok := m.selected()
		if !ok {
			return m, nil
		}
		m.opErr = nil
		m.name.SetValue(m.records[idx].Name)
		m.name.CursorEnd()
		m.step = manageEditingName
		return m, m.name.Focus()
	case "d":
		if _, ok := m.selected(); !ok {
			return m, nil
		}
		m.opErr = nil
		m.step = manageConfirmDelete
		return m, nil
	}
	return m, nil
}

func (m manage) updateEditName(msg tea.KeyPressMsg) (manage, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		// Cancel the edit entirely; nothing is mutated until the final save.
		m.name.Blur()
		m.step = manageBrowsing
		return m, nil
	case "enter":
		if strings.TrimSpace(m.name.Value()) == "" {
			return m, nil
		}
		idx, ok := m.selected()
		if !ok {
			m.name.Blur()
			m.step = manageBrowsing
			return m, nil
		}
		m.name.Blur()
		m.value.SetValue(m.records[idx].Value)
		m.value.CursorEnd()
		m.step = manageEditingValue
		return m, m.value.Focus()
	}

	var cmd tea.Cmd
	m.name, cmd = m.name.Update(msg)
	return m, cmd
}

func (m manage) updateEditValue(msg tea.KeyPressMsg) (manage, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		// Step back to the name field.
		m.value.Blur()
		m.step = manageEditingName
		return m, m.name.Focus()
	case "enter":
		if strings.TrimSpace(m.value.Value()) == "" {
			return m, nil
		}
		idx, ok := m.selected()
		if !ok {
			m.value.Blur()
			m.step = manageBrowsing
			return m, nil
		}
		m.value.Blur()
		m.records[idx].Name = fqdn(m.name.Value())
		m.records[idx].Value = m.value.Value()
		m.step = manageSaving
		return m, saveRecordsCmd(m.records)
	}

	var cmd tea.Cmd
	m.value, cmd = m.value.Update(msg)
	return m, cmd
}

func (m manage) updateConfirmDelete(msg tea.KeyPressMsg) (manage, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "n":
		m.step = manageBrowsing
		return m, nil
	case "enter":
		idx, ok := m.selected()
		if !ok {
			m.step = manageBrowsing
			return m, nil
		}
		m.records = append(m.records[:idx], m.records[idx+1:]...)
		m.step = manageSaving
		return m, saveRecordsCmd(m.records)
	}
	return m, nil
}

func (m manage) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Manage records"))
	b.WriteString("\n\n")

	switch m.step {
	case manageLoading:
		b.WriteString(subtitleStyle.Render("Loading records…"))
		return b.String()

	case manageSaving:
		b.WriteString(subtitleStyle.Render("Saving…"))
		return b.String()

	case manageEditingName:
		rec := m.records[m.order[m.cursor]]
		b.WriteString(subtitleStyle.Render("Editing "))
		b.WriteString(selectedItemStyle.Render(rec.Type))
		b.WriteString(subtitleStyle.Render(" record"))
		b.WriteString("\n\n")
		b.WriteString("Name (fully-qualified)\n")
		b.WriteString(m.name.View())
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("enter: continue • esc: cancel"))
		return b.String()

	case manageEditingValue:
		rec := m.records[m.order[m.cursor]]
		b.WriteString(subtitleStyle.Render("Editing "))
		b.WriteString(selectedItemStyle.Render(rec.Type))
		b.WriteString(subtitleStyle.Render(" record   Name: "))
		b.WriteString(m.name.Value())
		b.WriteString("\n\n")
		b.WriteString("Value\n")
		b.WriteString(m.value.View())
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("enter: save • esc: change name"))
		return b.String()

	case manageConfirmDelete:
		rec := m.records[m.order[m.cursor]]
		b.WriteString(errorStyle.Render("Delete this record? This cannot be undone."))
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf("  %s  %s  %s\n\n", rec.Type, rec.Name, rec.Value))
		b.WriteString(helpStyle.Render("enter: delete • esc: cancel"))
		return b.String()
	}

	// manageBrowsing
	if m.loadErr != nil {
		b.WriteString(errorStyle.Render("Failed to load records: " + m.loadErr.Error()))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("esc: back"))
		return b.String()
	}

	if m.opErr != nil {
		b.WriteString(errorStyle.Render("Save failed: " + m.opErr.Error()))
		b.WriteString("\n\n")
	}

	if len(m.order) == 0 {
		b.WriteString(subtitleStyle.Render("No records yet — create one from the main menu."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("esc: back"))
		return b.String()
	}

	nameWidth := 0
	for _, r := range m.records {
		if w := len(r.Name); w > nameWidth {
			nameWidth = w
		}
	}
	if nameWidth > 40 {
		nameWidth = 40
	}

	currentType := ""
	for pos, idx := range m.order {
		rec := m.records[idx]
		if rec.Type != currentType {
			if currentType != "" {
				b.WriteByte('\n')
			}
			b.WriteString(groupStyle.Render(rec.Type))
			b.WriteByte('\n')
			currentType = rec.Type
		}

		line := fmt.Sprintf("%-*s  %s", nameWidth, rec.Name, rec.Value)
		if pos == m.cursor {
			b.WriteString(selectedItemStyle.Render("> " + line))
		} else {
			b.WriteString(itemStyle.Render("  " + line))
		}
		b.WriteByte('\n')
	}

	b.WriteByte('\n')
	b.WriteString(helpStyle.Render("↑/↓: navigate • e: edit • d: delete • esc: back"))
	return b.String()
}
