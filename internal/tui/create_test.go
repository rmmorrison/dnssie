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

func TestTTLSummary(t *testing.T) {
	if got := ttlSummary(""); got != "default (300)" {
		t.Errorf(`ttlSummary("") = %q, want "default (300)"`, got)
	}
	if got := ttlSummary("bad"); got != "default (300)" {
		t.Errorf(`ttlSummary("bad") = %q, want "default (300)"`, got)
	}
	if got := ttlSummary("60"); got != "60" {
		t.Errorf(`ttlSummary("60") = %q, want "60"`, got)
	}
}

func TestCreateFlowTTLStep(t *testing.T) {
	m := newCreateRecord()
	m.step = stepEnterName
	m.chosen = supportedTypes[0] // A
	m.name.SetValue("app.test")
	m, _ = m.updateEnterName(key("enter"))
	if m.step != stepEnterValue {
		t.Fatalf("step = %v, want stepEnterValue", m.step)
	}

	m.value.SetValue("127.0.0.1")
	m, _ = m.updateEnterValue(key("enter"))
	if m.step != stepEnterTTL {
		t.Fatalf("step = %v, want stepEnterTTL", m.step)
	}

	// An invalid TTL keeps the step and flags the error.
	m.ttl.SetValue("oops")
	m, _ = m.updateEnterTTL(key("enter"))
	if m.step != stepEnterTTL || !m.ttlErr {
		t.Fatalf("invalid TTL: step=%v ttlErr=%v, want stepEnterTTL/true", m.step, m.ttlErr)
	}

	// A valid TTL clears the error and proceeds to save.
	m.ttl.SetValue("90")
	m, cmd := m.updateEnterTTL(key("enter"))
	if m.step != stepSaving || m.ttlErr {
		t.Fatalf("valid TTL: step=%v ttlErr=%v, want stepSaving/false", m.step, m.ttlErr)
	}
	if cmd == nil {
		t.Error("expected a save command")
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
