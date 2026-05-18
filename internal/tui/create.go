package tui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/rmmorrison/dnssie/internal/store"
)

// recordSavedMsg reports the outcome of persisting a record.
type recordSavedMsg struct {
	err error
}

// saveRecordCmd persists r to the default store off the UI goroutine.
func saveRecordCmd(r store.Record) tea.Cmd {
	return func() tea.Msg {
		st, err := store.Default()
		if err != nil {
			return recordSavedMsg{err: err}
		}
		if err := st.Add(r); err != nil {
			return recordSavedMsg{err: err}
		}
		return recordSavedMsg{}
	}
}

// fqdn canonicalizes a record name by trimming surrounding whitespace and
// ensuring a single trailing dot, so stored names match incoming queries.
func fqdn(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || strings.HasSuffix(name, ".") {
		return name
	}
	return name + "."
}

// parseTTL interprets a TTL field. A blank value means "use the default"
// (nil, valid). Otherwise it must be a non-negative integer that fits a
// uint32; anything else is rejected so a typo can't silently become 0.
func parseTTL(s string) (*uint32, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, true
	}
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return nil, false
	}
	v := uint32(n)
	return &v, true
}

// ttlPlaceholder is the hint shown in TTL inputs across the create/edit flows.
var ttlPlaceholder = "blank = default (" + strconv.FormatUint(uint64(store.DefaultTTL), 10) + ")"

// parseErratic interprets the erratic-mode field: blank (or "0") disables it;
// otherwise a whole number 0–100, the percentage of matching queries that
// should fail. Out-of-range or non-numeric input is rejected so a typo can't
// silently change the failure rate.
func parseErratic(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, true
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 || n > 100 {
		return 0, false
	}
	return n, true
}

// erraticPtr maps a parsed erratic percentage to the stored representation:
// nil when off (so it's omitted from records.toml), a pointer otherwise.
func erraticPtr(pct int) *int {
	if pct <= 0 {
		return nil
	}
	return &pct
}

// erraticPlaceholder is the hint shown in erratic-mode inputs.
var erraticPlaceholder = "0–100, blank = off"

// recordType is a DNS record type the user can create, with an example shown
// as placeholder text for the value input.
type recordType struct {
	name string
	hint string
}

// supportedTypes lists the record types dnssie can create. These are the most
// common types; more can be added later.
var supportedTypes = []recordType{
	{"A", "IPv4 address, e.g. 192.0.2.1"},
	{"AAAA", "IPv6 address, e.g. 2001:db8::1"},
	{"CNAME", "canonical name, e.g. example.com."},
	{"PTR", "hostname, e.g. host.example.com."},
	{"NS", "name server, e.g. ns1.example.com."},
	{"MX", "priority and host, e.g. 10 mail.example.com."},
	{"SOA", "primary ns, contact, serial, refresh, retry, expire, minimum"},
	{"TXT", `text value, e.g. "v=spf1 -all"`},
}

// createStep tracks where the user is in the create-record flow.
type createStep int

const (
	stepForm createStep = iota
	stepSaving
	stepDone
)

// createRecord is the screen for adding a new DNS record. It presents every
// field at once via the shared recordForm.
type createRecord struct {
	step    createStep
	form    recordForm
	pending store.Record // the submitted record, echoed on the done screen
	saveErr error
	st      styles
	width   int
	height  int
}

func newCreateRecord() createRecord {
	st := newStyles(true)
	return createRecord{
		step: stepForm,
		form: newRecordForm(st, 0),
		st:   st,
	}
}

func (m createRecord) Init() tea.Cmd { return nil }

func (m createRecord) Update(msg tea.Msg) (createRecord, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.form.setWidth(min(msg.Width-8, 60))
		return m, nil

	case themeMsg:
		m.st = msg.st
		m.form.setStyles(msg.st)
		return m, nil

	case recordSavedMsg:
		if msg.err != nil {
			// Keep the filled-in form so the user can adjust and retry.
			m.saveErr = msg.err
			m.step = stepForm
			return m, nil
		}
		m.step = stepDone
		return m, nil

	case tea.KeyPressMsg:
		switch m.step {
		case stepForm:
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			var action formAction
			var cmd tea.Cmd
			m.form, action, cmd = m.form.handleKey(msg)
			switch action {
			case formCancel:
				return m, changeScreen(screenMenu)
			case formSubmit:
				rec, _, _ := m.form.build()
				m.pending = rec
				m.saveErr = nil
				m.step = stepSaving
				return m, saveRecordCmd(rec)
			}
			return m, cmd

		case stepSaving:
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil

		case stepDone:
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "enter", "esc", "q":
				return m, changeScreen(screenMenu)
			}
			return m, nil
		}
	}

	// Non-key messages (e.g. cursor blink) go to the focused input.
	if m.step == stepForm {
		var cmd tea.Cmd
		m.form, cmd = m.form.updateInput(msg)
		return m, cmd
	}
	return m, nil
}

func (m createRecord) View() string {
	var b strings.Builder

	b.WriteString(m.st.title.Render("Create a new record"))
	b.WriteString("\n\n")

	switch m.step {
	case stepForm:
		b.WriteString(m.form.View())
		if m.saveErr != nil {
			b.WriteString("\n\n")
			b.WriteString(m.st.danger.Render("Save failed: " + m.saveErr.Error()))
		}

	case stepSaving:
		b.WriteString(m.st.subtitle.Render("Saving record…"))

	case stepDone:
		b.WriteString(m.st.success.Render("Record saved"))
		b.WriteString("\n\n")
		for _, kv := range [][2]string{
			{"Type:    ", m.pending.Type},
			{"Name:    ", m.pending.Name},
			{"Value:   ", m.pending.Value},
			{"TTL:     ", ttlDisplay(m.pending)},
			{"Erratic: ", erraticDisplay(m.pending)},
		} {
			b.WriteString(m.st.subtitle.Render(kv[0]))
			b.WriteString(kv[1])
			b.WriteByte('\n')
		}
	}

	return b.String()
}

func (m createRecord) footer() string {
	switch m.step {
	case stepForm:
		return m.form.footerHint()
	case stepDone:
		return "enter back to menu"
	}
	return ""
}
