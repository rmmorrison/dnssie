package tui

import "testing"

func TestParseTTL(t *testing.T) {
	if v, ok := parseTTL(""); !ok || v != nil {
		t.Errorf(`parseTTL("") = (%v,%v), want (nil,true)`, v, ok)
	}
	if v, ok := parseTTL("   "); !ok || v != nil {
		t.Errorf(`parseTTL(spaces) = (%v,%v), want (nil,true)`, v, ok)
	}
	if v, ok := parseTTL("0"); !ok || v == nil || *v != 0 {
		t.Errorf(`parseTTL("0") = (%v,%v), want (*0,true)`, v, ok)
	}
	if v, ok := parseTTL("3600"); !ok || v == nil || *v != 3600 {
		t.Errorf(`parseTTL("3600") = (%v,%v), want (*3600,true)`, v, ok)
	}
	for _, bad := range []string{"-1", "abc", "12.5", "99999999999999999999"} {
		if v, ok := parseTTL(bad); ok || v != nil {
			t.Errorf("parseTTL(%q) = (%v,%v), want (nil,false)", bad, v, ok)
		}
	}
}

func TestParseErratic(t *testing.T) {
	if n, ok := parseErratic(""); !ok || n != 0 {
		t.Errorf(`parseErratic("") = (%d,%v), want (0,true)`, n, ok)
	}
	if n, ok := parseErratic("  "); !ok || n != 0 {
		t.Errorf(`parseErratic(spaces) = (%d,%v), want (0,true)`, n, ok)
	}
	if n, ok := parseErratic("0"); !ok || n != 0 {
		t.Errorf(`parseErratic("0") = (%d,%v), want (0,true)`, n, ok)
	}
	if n, ok := parseErratic("100"); !ok || n != 100 {
		t.Errorf(`parseErratic("100") = (%d,%v), want (100,true)`, n, ok)
	}
	for _, bad := range []string{"-1", "101", "abc", "12.5", "50%"} {
		if n, ok := parseErratic(bad); ok || n != 0 {
			t.Errorf("parseErratic(%q) = (%d,%v), want (0,false)", bad, n, ok)
		}
	}
}

func TestCreateFormSubmits(t *testing.T) {
	m := newCreateRecord()
	if m.step != stepForm {
		t.Fatalf("step = %v, want stepForm", m.step)
	}

	// The type cycler starts focused; ←/→ changes the type.
	if m.form.recordType().name != "A" {
		t.Fatalf("default type = %q, want A", m.form.recordType().name)
	}
	m, _ = m.Update(key("right"))
	if m.form.recordType().name != "AAAA" {
		t.Errorf("after →, type = %q, want AAAA", m.form.recordType().name)
	}
	m, _ = m.Update(key("left"))
	if m.form.recordType().name != "A" {
		t.Errorf("after ←, type = %q, want A", m.form.recordType().name)
	}

	m.form.name.SetValue("app.test") // no trailing dot; fqdn() adds it
	m.form.value.SetValue("127.0.0.1")
	m.form.ttl.SetValue("90")
	m.form.erratic.SetValue("25")
	m, cmd := m.Update(key("enter"))
	if m.step != stepSaving {
		t.Fatalf("step = %v, want stepSaving", m.step)
	}
	if cmd == nil {
		t.Error("expected a save command")
	}
	if m.pending.Type != "A" || m.pending.Name != "app.test." || m.pending.Value != "127.0.0.1" {
		t.Errorf("pending = %+v, want A app.test. 127.0.0.1", m.pending)
	}
	if m.pending.TTL == nil || *m.pending.TTL != 90 {
		t.Errorf("pending TTL = %v, want 90", m.pending.TTL)
	}
	if m.pending.Erratic() != 25 {
		t.Errorf("pending erratic = %d, want 25", m.pending.Erratic())
	}

	if m, _ = m.Update(recordSavedMsg{}); m.step != stepDone {
		t.Errorf("step = %v, want stepDone after save", m.step)
	}
}

func TestCreateFormRejectsBadInput(t *testing.T) {
	m := newCreateRecord()

	// Blank name -> stays on the form, error shown, focus moved there.
	m.form.value.SetValue("127.0.0.1")
	m, _ = m.Update(key("enter"))
	if m.step != stepForm || m.form.errMsg == "" || m.form.focus != fldName {
		t.Fatalf("blank name: step=%v err=%q focus=%d", m.step, m.form.errMsg, m.form.focus)
	}

	// Bad TTL -> same, focus on the TTL field.
	m.form.name.SetValue("app.test")
	m.form.ttl.SetValue("oops")
	m, _ = m.Update(key("enter"))
	if m.step != stepForm || m.form.errMsg == "" || m.form.focus != fldTTL {
		t.Fatalf("bad TTL: step=%v err=%q focus=%d", m.step, m.form.errMsg, m.form.focus)
	}
}

func TestCreateFormCancel(t *testing.T) {
	m := newCreateRecord()
	m, cmd := m.Update(key("esc"))
	if cmd == nil {
		t.Error("esc should emit a screen-change command")
	}
	if m.step != stepForm {
		t.Errorf("step = %v, want stepForm (the app owns navigation)", m.step)
	}
}

func TestFQDN(t *testing.T) {
	cases := map[string]string{
		"www.example.com":  "www.example.com.",
		"www.example.com.": "www.example.com.",
		"example.com":      "example.com.",
		"  example.com  ":  "example.com.",
		"":                 "",
		"   ":              "",
	}
	for in, want := range cases {
		if got := fqdn(in); got != want {
			t.Errorf("fqdn(%q) = %q, want %q", in, got, want)
		}
	}
}
