package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"

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
// the manage tabs follow a familiar order. Unknown types sort last.
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
	manageEditing
	manageConfirmDelete
	manageSaving
)

// ttlDisplay formats a record's TTL for the records table: an explicit value
// as its number, an unset one as "default".
func ttlDisplay(r store.Record) string {
	if r.TTL == nil {
		return "default"
	}
	return strconv.FormatUint(uint64(*r.TTL), 10)
}

// ttlFieldValue formats a record's TTL for prefilling the edit input: empty
// when unset (so the placeholder explains the default), the number otherwise.
func ttlFieldValue(r store.Record) string {
	if r.TTL == nil {
		return ""
	}
	return strconv.FormatUint(uint64(*r.TTL), 10)
}

// erraticDisplay formats a record's erratic-mode setting for the records
// table: a quiet dash when off, "<n>%" when fault injection is enabled.
func erraticDisplay(r store.Record) string {
	if r.Erratic() <= 0 {
		return "—"
	}
	return strconv.Itoa(r.Erratic()) + "%"
}

// erraticFieldValue formats a record's erratic percentage for prefilling the
// edit input: empty when off, the number otherwise.
func erraticFieldValue(r store.Record) string {
	if r.Erratic() <= 0 {
		return ""
	}
	return strconv.Itoa(r.Erratic())
}

// manage is the screen for browsing persisted records. Records are split into
// one tab per record type; each tab shows its records in a table that can be
// navigated to edit or (with confirmation) delete the highlighted record.
type manage struct {
	step      manageStep
	records   []store.Record // canonical set as loaded from disk
	activeTab int            // index into supportedTypes
	cursor    int            // row within the active tab
	scroll    int            // top row of the visible table window
	editIdx   int            // index into records being edited/deleted
	form      recordForm     // the edit form (active while step == manageEditing)
	loadErr   error
	opErr     error // error from the last edit/delete save
	st        styles
	width     int
	height    int
}

func newManage() manage {
	return manage{step: manageLoading, st: newStyles(true)}
}

func (m manage) Init() tea.Cmd {
	return loadRecordsCmd()
}

// tabType is the record type shown on the active tab.
func (m manage) tabType() string {
	return supportedTypes[m.activeTab].name
}

// tabIndices returns the indices into m.records belonging to the active tab,
// sorted by name then value so the table order is stable.
func (m manage) tabIndices() []int {
	t := m.tabType()
	var idx []int
	for i, r := range m.records {
		if r.Type == t {
			idx = append(idx, i)
		}
	}
	sort.SliceStable(idx, func(a, b int) bool {
		ra, rb := m.records[idx[a]], m.records[idx[b]]
		if ra.Name != rb.Name {
			return ra.Name < rb.Name
		}
		return ra.Value < rb.Value
	})
	return idx
}

// rebuild clamps the cursor to the active tab's row count and keeps the
// scroll window consistent.
func (m *manage) rebuild() {
	n := len(m.tabIndices())
	if m.cursor >= n {
		m.cursor = max(n-1, 0)
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.followCursor()
}

// visibleRows is how many record rows fit in the scrollable table window. It
// is derived from the fixed region so the card never resizes with the data.
func (m manage) visibleRows() int {
	// regionHeight budget minus table chrome (top/bottom border, header,
	// header separator = 4) and the scroll status line (1).
	v := m.regionHeight() - 5
	if v < 1 {
		v = 1
	}
	return v
}

// clampedScroll returns the table's top-row offset, adjusted so the cursor
// stays visible and the window never runs past the end of the list.
func (m manage) clampedScroll() int {
	vis := m.visibleRows()
	n := len(m.tabIndices())
	s := m.scroll
	if m.cursor < s {
		s = m.cursor
	}
	if m.cursor >= s+vis {
		s = m.cursor - vis + 1
	}
	if maxScroll := n - vis; s > maxScroll {
		s = maxScroll
	}
	if s < 0 {
		s = 0
	}
	return s
}

// followCursor persists the scroll offset so navigation moves the window only
// when the cursor would otherwise leave it.
func (m *manage) followCursor() { m.scroll = m.clampedScroll() }

// selected returns the highlighted record's index into m.records, or false if
// there is nothing to select on the active tab.
func (m manage) selected() (int, bool) {
	if m.step == manageLoading {
		return 0, false
	}
	ti := m.tabIndices()
	if m.cursor < 0 || m.cursor >= len(ti) {
		return 0, false
	}
	return ti[m.cursor], true
}

func (m manage) Update(msg tea.Msg) (manage, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.step == manageEditing {
			m.form.setWidth(min(msg.Width-8, 60))
		}
		return m, nil

	case themeMsg:
		m.st = msg.st
		m.form.setStyles(msg.st)
		return m, nil

	case recordsLoadedMsg:
		if msg.err != nil {
			m.loadErr = msg.err
			m.records = nil
			m.step = manageBrowsing
			return m, nil
		}
		m.loadErr = nil
		m.records = msg.records
		m.rebuild()
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
		case manageEditing:
			return m.updateEditing(msg)
		case manageConfirmDelete:
			return m.updateConfirmDelete(msg)
		case manageLoading, manageSaving:
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}
	}

	// Keep the focused form input ticking (e.g. cursor blink).
	if m.step == manageEditing {
		var cmd tea.Cmd
		m.form, cmd = m.form.updateInput(msg)
		return m, cmd
	}
	return m, nil
}

func (m manage) updateBrowsing(msg tea.KeyPressMsg) (manage, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		return m, changeScreen(screenMenu)
	case "left", "h", "shift+tab":
		m.activeTab = (m.activeTab - 1 + len(supportedTypes)) % len(supportedTypes)
		m.cursor = 0
		m.scroll = 0
		m.opErr = nil
	case "right", "l", "tab":
		m.activeTab = (m.activeTab + 1) % len(supportedTypes)
		m.cursor = 0
		m.scroll = 0
		m.opErr = nil
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.followCursor()
		}
	case "down", "j":
		if m.cursor < len(m.tabIndices())-1 {
			m.cursor++
			m.followCursor()
		}
	case "e":
		idx, ok := m.selected()
		if !ok {
			return m, nil
		}
		m.opErr = nil
		m.editIdx = idx
		var cmd tea.Cmd
		m.form, cmd = editRecordForm(m.records[idx], m.st, min(m.width-8, 60))
		m.step = manageEditing
		return m, cmd
	case "d":
		idx, ok := m.selected()
		if !ok {
			return m, nil
		}
		m.opErr = nil
		m.editIdx = idx
		m.step = manageConfirmDelete
		return m, nil
	}
	return m, nil
}

// editTarget reports the record being edited/deleted, guarding against an
// index left stale by a concurrent reload.
func (m manage) editTarget() (store.Record, bool) {
	if m.editIdx < 0 || m.editIdx >= len(m.records) {
		return store.Record{}, false
	}
	return m.records[m.editIdx], true
}

// updateEditing delegates to the shared form. Nothing is mutated until the
// form is submitted, so cancelling simply returns to browsing.
func (m manage) updateEditing(msg tea.KeyPressMsg) (manage, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	var action formAction
	var cmd tea.Cmd
	m.form, action, cmd = m.form.handleKey(msg)
	switch action {
	case formCancel:
		m.step = manageBrowsing
		return m, nil
	case formSubmit:
		if _, ok := m.editTarget(); !ok {
			m.step = manageBrowsing
			return m, nil
		}
		rec, _, _ := m.form.build()
		m.records[m.editIdx] = rec
		m.step = manageSaving
		return m, saveRecordsCmd(m.records)
	}
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
		if _, ok := m.editTarget(); !ok {
			m.step = manageBrowsing
			return m, nil
		}
		m.records = append(m.records[:m.editIdx], m.records[m.editIdx+1:]...)
		m.step = manageSaving
		return m, saveRecordsCmd(m.records)
	}
	return m, nil
}

// bodyWidth is the usable text width inside the card, after subtracting the
// rounded border (2) and the box's horizontal padding (4).
func (m manage) bodyWidth() int {
	w := contentWidth(m.width) - 6
	if w < 16 {
		w = 16
	}
	return w
}

// headerLines is how many body lines precede the records region: the title
// and its blank, the tab strip, its rule, and the blank after it — plus the
// two-line error banner when one is shown.
func (m manage) headerLines() int {
	n := 5
	if m.opErr != nil {
		n += 2
	}
	return n
}

// regionHeight is the stable height reserved for the records area. It leaves
// the rest of the card (header + chrome) plus a one-line bottom margin within
// the terminal, so the frame never overruns the screen or resizes per tab.
func (m manage) regionHeight() int {
	if m.height <= 0 {
		return 10
	}
	// Chrome (8): app padding, card border + padding, footer.
	// + 1 safety margin so the frame never reaches the last terminal row.
	h := m.height - 8 - 1 - m.headerLines()
	if h < 3 {
		h = 3
	}
	return h
}

// tabBar renders the per-type tab strip with a rule beneath it. Tabs with
// records show a count so populated tabs are discoverable at a glance.
func (m manage) tabBar(width int) string {
	counts := make(map[string]int, len(supportedTypes))
	for _, r := range m.records {
		counts[r.Type]++
	}

	cells := make([]string, len(supportedTypes))
	for i, rt := range supportedTypes {
		label := rt.name
		if n := counts[rt.name]; n > 0 {
			label = fmt.Sprintf("%s·%d", rt.name, n)
		}
		if i == m.activeTab {
			cells[i] = m.st.activeTab.Render(label)
		} else {
			cells[i] = m.st.tab.Render(label)
		}
	}

	bar := lipgloss.JoinHorizontal(lipgloss.Top, cells...)
	rule := m.st.tabRule.Render(strings.Repeat("─", width))
	return bar + "\n" + rule
}

// recordsTable renders the given window of rows as a styled table, with the
// row at offset sel (within the window) highlighted.
func (m manage) recordsTable(width int, window []int, sel int) string {
	rows := make([][]string, len(window))
	for i, idx := range window {
		r := m.records[idx]
		rows[i] = []string{r.Name, r.Value, ttlDisplay(r), erraticDisplay(r)}
	}

	return table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(m.st.accent)).
		Headers("NAME", "VALUE", "TTL", "FAIL%").
		Width(width).
		Rows(rows...).
		StyleFunc(func(row, _ int) lipgloss.Style {
			switch {
			case row == table.HeaderRow:
				return m.st.tableHead
			case row == sel:
				return m.st.tableSel
			default:
				return m.st.tableCell
			}
		}).
		Render()
}

// padBlock grows s to at least h lines so a shorter table (or empty state)
// still occupies the reserved region; it never truncates a taller block.
func padBlock(s string, h int) string {
	n := strings.Count(s, "\n") + 1
	if n >= h {
		return s
	}
	return s + strings.Repeat("\n", h-n)
}

func (m manage) View() string {
	var b strings.Builder

	b.WriteString(m.st.title.Render("Manage records"))
	b.WriteString("\n\n")

	switch m.step {
	case manageLoading:
		b.WriteString(m.st.subtitle.Render("Loading records…"))
		return b.String()

	case manageSaving:
		b.WriteString(m.st.subtitle.Render("Saving…"))
		return b.String()

	case manageEditing:
		b.WriteString(m.st.subtitle.Render("Editing record"))
		b.WriteString("\n\n")
		b.WriteString(m.form.View())
		return b.String()

	case manageConfirmDelete:
		rec, _ := m.editTarget()
		b.WriteString(m.st.danger.Render("Delete this record? This cannot be undone."))
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf("  %s  %s  %s  TTL %s  fail %s",
			rec.Type, rec.Name, rec.Value, ttlDisplay(rec), erraticDisplay(rec)))
		return b.String()
	}

	// manageBrowsing
	if m.loadErr != nil {
		b.WriteString(m.st.danger.Render("Failed to load records: " + m.loadErr.Error()))
		return b.String()
	}

	width := m.bodyWidth()
	b.WriteString(m.tabBar(width))
	b.WriteString("\n\n")

	if m.opErr != nil {
		b.WriteString(m.st.danger.Render("Save failed: " + m.opErr.Error()))
		b.WriteString("\n\n")
	}

	ti := m.tabIndices()
	if len(ti) == 0 {
		msg := m.st.subtitle.Render(fmt.Sprintf("No %s records yet.", m.tabType())) +
			"\n" + m.st.subtitle.Render("Create one from the main menu.")
		b.WriteString(lipgloss.Place(width, m.regionHeight(),
			lipgloss.Center, lipgloss.Center, msg))
		return b.String()
	}

	// Show only a fixed-height window of rows; the table scrolls with the
	// cursor instead of growing the card.
	vis := m.visibleRows()
	scroll := m.clampedScroll()
	end := min(scroll+vis, len(ti))
	window := ti[scroll:end]

	tbl := m.recordsTable(width, window, m.cursor-scroll)

	status := fmt.Sprintf("rows %d–%d of %d", scroll+1, end, len(ti))
	if scroll > 0 {
		status = "↑ " + status
	}
	if end < len(ti) {
		status += " ↓"
	}
	block := tbl + "\n" + m.st.subtitle.Render("  "+status)

	b.WriteString(padBlock(block, m.regionHeight()))
	return b.String()
}

func (m manage) footer() string {
	switch m.step {
	case manageLoading, manageSaving:
		return ""
	case manageEditing:
		return m.form.footerHint()
	case manageConfirmDelete:
		return "enter delete · esc cancel"
	}
	// manageBrowsing
	if m.loadErr != nil {
		return "esc back"
	}
	if len(m.tabIndices()) == 0 {
		return "←/→ tabs · esc back"
	}
	return "←/→ tabs · ↑/↓ navigate · e edit · d delete · esc back"
}
