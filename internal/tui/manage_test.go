package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/rmmorrison/dnssie/internal/store"
)

// key builds a KeyPressMsg whose String() matches what the handlers switch on.
func key(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	default: // single printable rune, e.g. "e", "d", "q"
		r := []rune(s)[0]
		return tea.KeyPressMsg{Code: r, Text: s}
	}
}

// browsing returns a manage model populated with records and ready to browse.
func browsing(records []store.Record) manage {
	m := newManage()
	m.records = records
	m.rebuildOrder()
	m.step = manageBrowsing
	return m
}

func TestTypeRank(t *testing.T) {
	if typeRank("A") != 0 {
		t.Errorf("typeRank(A) = %d, want 0", typeRank("A"))
	}
	if typeRank("TXT") <= typeRank("A") {
		t.Errorf("TXT should rank after A")
	}
	if got, want := typeRank("WAT"), len(supportedTypes); got != want {
		t.Errorf("typeRank(unknown) = %d, want %d", got, want)
	}
}

func TestRebuildOrderGroupsAndSorts(t *testing.T) {
	m := browsing([]store.Record{
		{Type: "TXT", Name: "z.example.com.", Value: "b"},
		{Type: "A", Name: "b.example.com.", Value: "2"},
		{Type: "A", Name: "a.example.com.", Value: "1"},
		{Type: "TXT", Name: "z.example.com.", Value: "a"},
	})

	// Expected: A group first (sorted by name), then TXT (name then value).
	want := []store.Record{
		{Type: "A", Name: "a.example.com.", Value: "1"},
		{Type: "A", Name: "b.example.com.", Value: "2"},
		{Type: "TXT", Name: "z.example.com.", Value: "a"},
		{Type: "TXT", Name: "z.example.com.", Value: "b"},
	}
	if len(m.order) != len(want) {
		t.Fatalf("order has %d entries, want %d", len(m.order), len(want))
	}
	for pos, idx := range m.order {
		if got := m.records[idx]; got != want[pos] {
			t.Errorf("order[%d] = %+v, want %+v", pos, got, want[pos])
		}
	}
}

func TestRebuildOrderClampsCursor(t *testing.T) {
	m := newManage()
	m.records = []store.Record{{Type: "A", Name: "a.", Value: "1"}}
	m.cursor = 5
	m.rebuildOrder()
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0 after clamp", m.cursor)
	}

	m.records = nil
	m.cursor = 3
	m.rebuildOrder()
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0 for empty records", m.cursor)
	}
}

func TestSelected(t *testing.T) {
	if _, ok := newManage().selected(); ok {
		t.Error("selected() should be false while loading")
	}
	if _, ok := browsing(nil).selected(); ok {
		t.Error("selected() should be false with no records")
	}

	m := browsing([]store.Record{
		{Type: "A", Name: "a.", Value: "1"},
		{Type: "A", Name: "b.", Value: "2"},
	})
	m.cursor = 1
	idx, ok := m.selected()
	if !ok || m.records[idx].Name != "b." {
		t.Errorf("selected() = (%d,%v), want index of b.", idx, ok)
	}
}

func TestBrowsingNavigationClamps(t *testing.T) {
	m := browsing([]store.Record{
		{Type: "A", Name: "a.", Value: "1"},
		{Type: "A", Name: "b.", Value: "2"},
	})

	m, _ = m.updateBrowsing(key("up")) // already at top
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (clamped at top)", m.cursor)
	}
	m, _ = m.updateBrowsing(key("down"))
	m, _ = m.updateBrowsing(key("down")) // past bottom
	if m.cursor != 1 {
		t.Errorf("cursor = %d, want 1 (clamped at bottom)", m.cursor)
	}
}

func TestDeleteRemovesSelectedAfterConfirm(t *testing.T) {
	m := browsing([]store.Record{
		{Type: "A", Name: "a.example.com.", Value: "1"},
		{Type: "A", Name: "b.example.com.", Value: "2"},
	})
	m.cursor = 1
	target := m.records[m.order[m.cursor]]

	m, _ = m.updateBrowsing(key("d"))
	if m.step != manageConfirmDelete {
		t.Fatalf("step = %v, want manageConfirmDelete", m.step)
	}

	m, cmd := m.updateConfirmDelete(key("enter"))
	if m.step != manageSaving {
		t.Fatalf("step = %v, want manageSaving", m.step)
	}
	if cmd == nil {
		t.Error("expected a save command")
	}
	if len(m.records) != 1 {
		t.Fatalf("records len = %d, want 1", len(m.records))
	}
	if m.records[0] == target {
		t.Errorf("deleted record %+v still present", target)
	}
}

func TestDeleteCancelKeepsRecords(t *testing.T) {
	m := browsing([]store.Record{{Type: "A", Name: "a.", Value: "1"}})

	m, _ = m.updateBrowsing(key("d"))
	m, _ = m.updateConfirmDelete(key("esc"))

	if m.step != manageBrowsing {
		t.Errorf("step = %v, want manageBrowsing after cancel", m.step)
	}
	if len(m.records) != 1 {
		t.Errorf("records len = %d, want 1 (unchanged)", len(m.records))
	}
}

func TestEditUpdatesSelectedRecord(t *testing.T) {
	m := browsing([]store.Record{
		{Type: "A", Name: "old.example.com.", Value: "1.1.1.1"},
	})
	idx := m.order[m.cursor]

	m, _ = m.updateBrowsing(key("e"))
	if m.step != manageEditingName {
		t.Fatalf("step = %v, want manageEditingName", m.step)
	}
	if m.name.Value() != "old.example.com." {
		t.Errorf("name field = %q, want prefilled with current name", m.name.Value())
	}

	// Enter a new name without a trailing dot; fqdn() should add it.
	m.name.SetValue("new.example.com")
	m, _ = m.updateEditName(key("enter"))
	if m.step != manageEditingValue {
		t.Fatalf("step = %v, want manageEditingValue", m.step)
	}

	m.value.SetValue("2.2.2.2")
	m, cmd := m.updateEditValue(key("enter"))
	if m.step != manageSaving {
		t.Fatalf("step = %v, want manageSaving", m.step)
	}
	if cmd == nil {
		t.Error("expected a save command")
	}
	got := m.records[idx]
	if got.Name != "new.example.com." || got.Value != "2.2.2.2" || got.Type != "A" {
		t.Errorf("record = %+v, want name new.example.com. value 2.2.2.2 type A", got)
	}
}

func TestEditCancelKeepsRecord(t *testing.T) {
	original := store.Record{Type: "A", Name: "a.example.com.", Value: "1.1.1.1"}
	m := browsing([]store.Record{original})
	idx := m.order[m.cursor]

	m, _ = m.updateBrowsing(key("e"))
	m.name.SetValue("changed.example.com")
	m, _ = m.updateEditName(key("esc")) // cancel

	if m.step != manageBrowsing {
		t.Errorf("step = %v, want manageBrowsing", m.step)
	}
	if m.records[idx] != original {
		t.Errorf("record = %+v, want unchanged %+v", m.records[idx], original)
	}
}

func TestLoadedMessagePopulatesAndSorts(t *testing.T) {
	m := newManage()
	m, _ = m.Update(recordsLoadedMsg{records: []store.Record{
		{Type: "TXT", Name: "t.", Value: "x"},
		{Type: "A", Name: "a.", Value: "1"},
	}})

	if m.step != manageBrowsing {
		t.Fatalf("step = %v, want manageBrowsing", m.step)
	}
	if got := m.records[m.order[0]]; got.Type != "A" {
		t.Errorf("first ordered record type = %q, want A", got.Type)
	}
}

func TestLoadErrorIsSurfaced(t *testing.T) {
	m := newManage()
	m, _ = m.Update(recordsLoadedMsg{err: errFake})

	if m.loadErr == nil {
		t.Error("loadErr should be set")
	}
	if m.step != manageBrowsing {
		t.Errorf("step = %v, want manageBrowsing (so the error renders)", m.step)
	}
}

type fakeErr struct{}

func (fakeErr) Error() string { return "boom" }

var errFake = fakeErr{}
