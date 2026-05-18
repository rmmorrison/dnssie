package tui

import (
	"testing"

	"github.com/rmmorrison/dnssie/internal/store"
)

func TestRecordFormFocusClampAndWrap(t *testing.T) {
	f := newRecordForm(newStyles(true), 60)
	if f.focus != fldType {
		t.Fatalf("new form focus = %d, want fldType", f.focus)
	}

	// Arrows clamp at the ends.
	(&f).moveFocus(-1, false)
	if f.focus != fldType {
		t.Errorf("up at top: focus = %d, want fldType (clamped)", f.focus)
	}
	for i := 0; i < 10; i++ {
		(&f).moveFocus(+1, false)
	}
	if f.focus != fldErratic {
		t.Errorf("down past end: focus = %d, want fldErratic (clamped)", f.focus)
	}

	// Tab wraps around the whole range.
	(&f).moveFocus(+1, true)
	if f.focus != fldType {
		t.Errorf("tab past end: focus = %d, want fldType (wrapped)", f.focus)
	}
	(&f).moveFocus(-1, true)
	if f.focus != fldErratic {
		t.Errorf("shift+tab at start: focus = %d, want fldErratic (wrapped)", f.focus)
	}
}

func TestRecordFormEditLocksTypeAndSkipsIt(t *testing.T) {
	rec := store.Record{Type: "TXT", Name: "a.test.", Value: "hi"}
	f, _ := editRecordForm(rec, newStyles(true), 60)

	if !f.typeLocked || f.recordType().name != "TXT" {
		t.Fatalf("editRecordForm: locked=%v type=%q, want true/TXT", f.typeLocked, f.recordType().name)
	}
	if f.firstFocus() != fldName || f.focus != fldName {
		t.Errorf("locked form firstFocus=%d focus=%d, want fldName", f.firstFocus(), f.focus)
	}

	// Navigation never lands on the locked type row.
	(&f).moveFocus(-1, false)
	if f.focus != fldName {
		t.Errorf("up from name (locked): focus = %d, want fldName", f.focus)
	}
	(&f).moveFocus(-1, true) // wrap should skip fldType
	if f.focus != fldErratic {
		t.Errorf("wrap-up from name (locked): focus = %d, want fldErratic", f.focus)
	}
}

func TestRecordFormBuildValidationOrder(t *testing.T) {
	f := newRecordForm(newStyles(true), 60)

	if _, msg, bad := f.build(); bad != fldName || msg == "" {
		t.Errorf("blank name: bad=%d msg=%q, want fldName/non-empty", bad, msg)
	}
	f.name.SetValue("a.test")
	if _, _, bad := f.build(); bad != fldValue {
		t.Errorf("blank value: bad=%d, want fldValue", bad)
	}
	f.value.SetValue("1.2.3.4")
	f.ttl.SetValue("nope")
	if _, _, bad := f.build(); bad != fldTTL {
		t.Errorf("bad ttl: bad=%d, want fldTTL", bad)
	}
	f.ttl.SetValue("30")
	f.erratic.SetValue("999")
	if _, _, bad := f.build(); bad != fldErratic {
		t.Errorf("bad erratic: bad=%d, want fldErratic", bad)
	}
	f.erratic.SetValue("")
	rec, msg, bad := f.build()
	if msg != "" || bad != -1 {
		t.Fatalf("valid form: msg=%q bad=%d, want ok", msg, bad)
	}
	if rec.Name != "a.test." || rec.Value != "1.2.3.4" || rec.TTL == nil || *rec.TTL != 30 ||
		rec.ErraticPct != nil {
		t.Errorf("built record = %+v, want a.test./1.2.3.4/ttl 30/erratic off", rec)
	}
}
