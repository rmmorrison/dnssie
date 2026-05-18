package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
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
	stepChooseType createStep = iota
	stepEnterName
	stepEnterValue
	stepSaving
	stepDone
)

// createRecord is the screen for adding a new DNS record: pick a type, name
// it, then enter the value. Persisting the record isn't wired up yet.
type createRecord struct {
	step    createStep
	cursor  int
	chosen  recordType
	name    textinput.Model
	value   textinput.Model
	saveErr error
	width   int
	height  int
}

func newCreateRecord() createRecord {
	name := textinput.New()
	name.CharLimit = 253
	name.Placeholder = "full name, e.g. www.example.com. or example.com."

	value := textinput.New()
	value.CharLimit = 512

	return createRecord{
		step:  stepChooseType,
		name:  name,
		value: value,
	}
}

func (m createRecord) Init() tea.Cmd {
	return nil
}

func (m createRecord) Update(msg tea.Msg) (createRecord, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		w := min(msg.Width-8, 60)
		m.name.SetWidth(w)
		m.value.SetWidth(w)
		return m, nil

	case recordSavedMsg:
		if msg.err != nil {
			// Surface the error and let the user retry from the value step.
			m.saveErr = msg.err
			m.step = stepEnterValue
			return m, m.value.Focus()
		}
		m.step = stepDone
		return m, nil

	case tea.KeyPressMsg:
		switch m.step {
		case stepChooseType:
			return m.updateChooseType(msg)
		case stepEnterName:
			return m.updateEnterName(msg)
		case stepEnterValue:
			return m.updateEnterValue(msg)
		case stepSaving:
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		case stepDone:
			return m.updateDone(msg)
		}
	}

	// Keep the focused text input ticking (e.g. cursor blink).
	var cmd tea.Cmd
	switch m.step {
	case stepEnterName:
		m.name, cmd = m.name.Update(msg)
	case stepEnterValue:
		m.value, cmd = m.value.Update(msg)
	}
	return m, cmd
}

func (m createRecord) updateChooseType(msg tea.KeyPressMsg) (createRecord, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		return m, changeScreen(screenMenu)
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(supportedTypes)-1 {
			m.cursor++
		}
	case "enter", "space":
		m.chosen = supportedTypes[m.cursor]
		m.value.Placeholder = m.chosen.hint
		m.step = stepEnterName
		return m, m.name.Focus()
	}
	return m, nil
}

func (m createRecord) updateEnterName(msg tea.KeyPressMsg) (createRecord, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		// Go back to type selection.
		m.name.Blur()
		m.step = stepChooseType
		return m, nil
	case "enter":
		if strings.TrimSpace(m.name.Value()) == "" {
			return m, nil
		}
		m.name.Blur()
		m.step = stepEnterValue
		return m, m.value.Focus()
	}

	var cmd tea.Cmd
	m.name, cmd = m.name.Update(msg)
	return m, cmd
}

func (m createRecord) updateEnterValue(msg tea.KeyPressMsg) (createRecord, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		// Go back to the name field.
		m.value.Blur()
		m.saveErr = nil
		m.step = stepEnterName
		return m, m.name.Focus()
	case "enter":
		if strings.TrimSpace(m.value.Value()) == "" {
			return m, nil
		}
		m.value.Blur()
		m.saveErr = nil
		m.step = stepSaving
		return m, saveRecordCmd(store.Record{
			Type:  m.chosen.name,
			Name:  fqdn(m.name.Value()),
			Value: m.value.Value(),
		})
	}

	var cmd tea.Cmd
	m.value, cmd = m.value.Update(msg)
	return m, cmd
}

func (m createRecord) updateDone(msg tea.KeyPressMsg) (createRecord, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "enter", "esc", "q":
		return m, changeScreen(screenMenu)
	}
	return m, nil
}

func (m createRecord) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Create a new record"))
	b.WriteString("\n\n")

	switch m.step {
	case stepChooseType:
		b.WriteString(subtitleStyle.Render("Choose a record type"))
		b.WriteString("\n\n")
		for i, rt := range supportedTypes {
			if i == m.cursor {
				b.WriteString(selectedItemStyle.Render("▌ " + rt.name))
			} else {
				b.WriteString(itemStyle.Render("  " + rt.name))
			}
			b.WriteByte('\n')
		}

	case stepEnterName:
		b.WriteString(subtitleStyle.Render("Type: "))
		b.WriteString(selectedItemStyle.Render(m.chosen.name))
		b.WriteString("\n\n")
		b.WriteString("Name (fully-qualified)\n")
		b.WriteString(m.name.View())

	case stepEnterValue:
		b.WriteString(subtitleStyle.Render("Type: "))
		b.WriteString(selectedItemStyle.Render(m.chosen.name))
		b.WriteString(subtitleStyle.Render("   Name: "))
		b.WriteString(m.name.Value())
		b.WriteString("\n\n")
		b.WriteString("Value\n")
		b.WriteString(m.value.View())
		if m.saveErr != nil {
			b.WriteString("\n\n")
			b.WriteString(errorStyle.Render("Save failed: " + m.saveErr.Error()))
		}

	case stepSaving:
		b.WriteString(subtitleStyle.Render("Saving record…"))

	case stepDone:
		b.WriteString(statusStyle.Render("Record saved"))
		b.WriteString("\n\n")
		b.WriteString(subtitleStyle.Render("Type:  "))
		b.WriteString(m.chosen.name)
		b.WriteByte('\n')
		b.WriteString(subtitleStyle.Render("Name:  "))
		b.WriteString(m.name.Value())
		b.WriteByte('\n')
		b.WriteString(subtitleStyle.Render("Value: "))
		b.WriteString(m.value.Value())
	}

	return b.String()
}

func (m createRecord) footer() string {
	switch m.step {
	case stepChooseType:
		return "↑/↓ navigate · enter select · esc back"
	case stepEnterName:
		return "enter continue · esc change type"
	case stepEnterValue:
		return "enter save · esc change name"
	case stepSaving:
		return ""
	case stepDone:
		return "enter back to menu"
	}
	return ""
}
