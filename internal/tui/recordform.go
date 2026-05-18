package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/rmmorrison/dnssie/internal/store"
)

// formAction is what a recordForm asks its host screen to do after a keypress.
type formAction int

const (
	formNone   formAction = iota // stay in the form
	formSubmit                   // fields validated; persist the record
	formCancel                   // leave the form without saving
)

// recordForm field indices, top to bottom.
const (
	fldType = iota
	fldName
	fldValue
	fldTTL
	fldErratic
	fldCount
)

var fieldLabels = [fldCount]string{
	fldType:    "Type",
	fldName:    "Name",
	fldValue:   "Value",
	fldTTL:     "TTL",
	fldErratic: "Erratic %",
}

// recordForm is a single-screen, multi-field editor for a DNS record, shared
// by the create and edit flows. Type is a ◀ ▶ cycler when adding a record and
// a fixed value when editing an existing one (records are keyed by type).
type recordForm struct {
	typeIdx    int  // index into supportedTypes
	typeLocked bool // editing: the type can't change
	name       textinput.Model
	value      textinput.Model
	ttl        textinput.Model
	erratic    textinput.Model
	focus      int    // which field is focused
	errMsg     string // validation error from the last submit attempt
	st         styles
}

// newFormInputs builds the four text inputs with their limits and hints.
func newFormInputs() (name, value, ttl, erratic textinput.Model) {
	name = textinput.New()
	name.CharLimit = 253
	name.Prompt = ""
	name.Placeholder = "www.example.com. or *.app.test."

	value = textinput.New()
	value.CharLimit = 512
	value.Prompt = ""

	ttl = textinput.New()
	ttl.CharLimit = 10
	ttl.Prompt = ""
	ttl.Placeholder = ttlPlaceholder

	erratic = textinput.New()
	erratic.CharLimit = 3
	erratic.Prompt = ""
	erratic.Placeholder = erraticPlaceholder
	return
}

// newRecordForm builds the form for adding a new record: every field is
// editable and focus starts on the type cycler.
func newRecordForm(st styles, width int) recordForm {
	name, value, ttl, erratic := newFormInputs()
	f := recordForm{
		name: name, value: value, ttl: ttl, erratic: erratic,
		focus: fldType,
		st:    st,
	}
	f.value.Placeholder = supportedTypes[0].hint
	f.setWidth(width)
	return f
}

// editRecordForm builds the form for editing rec: the type is shown but
// locked, the other fields are prefilled, and focus starts on the name. The
// returned command starts the focused input's cursor blink.
func editRecordForm(rec store.Record, st styles, width int) (recordForm, tea.Cmd) {
	name, value, ttl, erratic := newFormInputs()
	f := recordForm{
		typeIdx:    typeRank(rec.Type), // clamped on read
		typeLocked: true,
		name:       name, value: value, ttl: ttl, erratic: erratic,
		focus: fldName,
		st:    st,
	}
	f.value.Placeholder = f.recordType().hint
	f.name.SetValue(rec.Name)
	f.name.CursorEnd()
	f.value.SetValue(rec.Value)
	f.ttl.SetValue(ttlFieldValue(rec))
	f.erratic.SetValue(erraticFieldValue(rec))
	f.setWidth(width)
	return f, f.name.Focus()
}

// recordType is the currently selected record type, clamped so a record with
// an unknown stored type can't index out of range.
func (f recordForm) recordType() recordType {
	i := f.typeIdx
	if i < 0 || i >= len(supportedTypes) {
		i = 0
	}
	return supportedTypes[i]
}

// firstFocus is the topmost focusable field: the type cycler when adding,
// the name when editing (the type is locked).
func (f recordForm) firstFocus() int {
	if f.typeLocked {
		return fldName
	}
	return fldType
}

func (f *recordForm) setStyles(st styles) { f.st = st }

func (f *recordForm) setWidth(w int) {
	// Leave room for the focus marker and the label column.
	iw := w - 12
	if iw < 8 {
		iw = 8
	}
	f.name.SetWidth(iw)
	f.value.SetWidth(iw)
	f.ttl.SetWidth(iw)
	f.erratic.SetWidth(iw)
}

// setFocus moves focus to field i (clamped to the focusable range), blurs
// every input, focuses the one now selected, and returns its blink command.
func (f *recordForm) setFocus(i int) tea.Cmd {
	if lo := f.firstFocus(); i < lo {
		i = lo
	}
	if i > fldErratic {
		i = fldErratic
	}
	f.focus = i
	f.name.Blur()
	f.value.Blur()
	f.ttl.Blur()
	f.erratic.Blur()
	switch i {
	case fldName:
		return f.name.Focus()
	case fldValue:
		return f.value.Focus()
	case fldTTL:
		return f.ttl.Focus()
	case fldErratic:
		return f.erratic.Focus()
	}
	return nil // fldType has no text input
}

// moveFocus shifts focus by delta. When wrap is true (Tab) it cycles around
// the focusable range; otherwise (arrows) it stops at the ends.
func (f *recordForm) moveFocus(delta int, wrap bool) tea.Cmd {
	lo := f.firstFocus()
	span := fldErratic - lo + 1
	i := f.focus + delta
	if wrap {
		i = lo + ((i-lo)%span+span)%span
	}
	return f.setFocus(i)
}

func (f *recordForm) cycleType(delta int) {
	n := len(supportedTypes)
	f.typeIdx = (f.typeIdx + delta%n + n) % n
	f.value.Placeholder = f.recordType().hint
}

// build validates the fields and assembles a record. On failure it returns a
// message and the index of the offending field; on success errMsg is "".
func (f recordForm) build() (rec store.Record, errMsg string, badField int) {
	if strings.TrimSpace(f.name.Value()) == "" {
		return rec, "Name is required.", fldName
	}
	if strings.TrimSpace(f.value.Value()) == "" {
		return rec, "Value is required.", fldValue
	}
	ttl, ok := parseTTL(f.ttl.Value())
	if !ok {
		return rec, "TTL must be a whole number of seconds (or blank for the default).", fldTTL
	}
	pct, ok := parseErratic(f.erratic.Value())
	if !ok {
		return rec, "Erratic % must be a whole number 0–100 (or blank for off).", fldErratic
	}
	return store.Record{
		Type:       f.recordType().name,
		Name:       fqdn(f.name.Value()),
		Value:      f.value.Value(),
		TTL:        ttl,
		ErraticPct: erraticPtr(pct),
	}, "", -1
}

// handleKey processes one keypress, returning the (possibly mutated) form, an
// action for the host screen, and any input command to run.
func (f recordForm) handleKey(msg tea.KeyPressMsg) (recordForm, formAction, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return f, formCancel, nil
	case "tab":
		return f, formNone, f.moveFocus(+1, true)
	case "shift+tab":
		return f, formNone, f.moveFocus(-1, true)
	case "up":
		return f, formNone, f.moveFocus(-1, false)
	case "down":
		return f, formNone, f.moveFocus(+1, false)
	case "left":
		if f.focus == fldType && !f.typeLocked {
			f.cycleType(-1)
			return f, formNone, nil
		}
	case "right":
		if f.focus == fldType && !f.typeLocked {
			f.cycleType(+1)
			return f, formNone, nil
		}
	case "enter":
		if _, errMsg, bad := f.build(); errMsg != "" {
			f.errMsg = errMsg
			return f, formNone, f.setFocus(bad)
		}
		f.errMsg = ""
		return f, formSubmit, nil
	}

	var cmd tea.Cmd
	f, cmd = f.updateInput(msg)
	return f, formNone, cmd
}

// updateInput forwards a message to the focused text input. It's also the path
// for non-key messages (e.g. cursor blink ticks).
func (f recordForm) updateInput(msg tea.Msg) (recordForm, tea.Cmd) {
	var cmd tea.Cmd
	switch f.focus {
	case fldName:
		f.name, cmd = f.name.Update(msg)
	case fldValue:
		f.value, cmd = f.value.Update(msg)
	case fldTTL:
		f.ttl, cmd = f.ttl.Update(msg)
	case fldErratic:
		f.erratic, cmd = f.erratic.Update(msg)
	}
	return f, cmd
}

// fieldRow renders one "marker label  content" line.
func (f recordForm) fieldRow(i int, content string) string {
	label := fmt.Sprintf("%-9s ", fieldLabels[i])
	if f.focus == i {
		return f.st.selected.Render("▌ "+label) + content + "\n"
	}
	return f.st.item.Render("  "+label) + content + "\n"
}

func (f recordForm) View() string {
	var b strings.Builder

	if f.typeLocked {
		b.WriteString(f.fieldRow(fldType,
			f.st.subtitle.Render(f.recordType().name+"  (type can't be changed)")))
	} else {
		b.WriteString(f.fieldRow(fldType, "◀ "+f.recordType().name+" ▶"))
	}
	b.WriteString(f.fieldRow(fldName, f.name.View()))
	b.WriteString(f.fieldRow(fldValue, f.value.View()))
	b.WriteString(f.fieldRow(fldTTL, f.ttl.View()))
	b.WriteString(f.fieldRow(fldErratic, f.erratic.View()))

	b.WriteByte('\n')
	b.WriteString(f.st.desc.Render(
		"Tip: name may be a wildcard (*.app.test.) · blank TTL = default · blank erratic = off"))
	if f.errMsg != "" {
		b.WriteString("\n\n")
		b.WriteString(f.st.danger.Render(f.errMsg))
	}
	return b.String()
}

func (f recordForm) footerHint() string {
	nav := "↑/↓ field · tab next"
	if f.focus == fldType && !f.typeLocked {
		nav += " · ←/→ type"
	}
	return nav + " · enter save · esc cancel"
}
