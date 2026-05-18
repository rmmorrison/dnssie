package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDefaults(t *testing.T) {
	d := Defaults()
	if d.Port != 1053 {
		t.Errorf("default port = %d, want 1053", d.Port)
	}
	if d.Resolvers.Mode != ModeSystem {
		t.Errorf("default mode = %q, want %q", d.Resolvers.Mode, ModeSystem)
	}
	if len(d.Resolvers.Upstream) != 0 {
		t.Errorf("default upstream = %v, want empty", d.Resolvers.Upstream)
	}
}

func TestLoadMissingReturnsDefaults(t *testing.T) {
	got, err := New(t.TempDir()).Load()
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if !reflect.DeepEqual(got, Defaults()) {
		t.Errorf("Load() = %+v, want defaults %+v", got, Defaults())
	}
}

func TestSaveReloadRoundTrip(t *testing.T) {
	s := New(t.TempDir())
	want := Config{
		Port: 5353,
		Resolvers: Resolvers{
			Mode:     ModeManual,
			Upstream: []string{"1.1.1.1:53", "8.8.8.8:53"},
		},
	}
	if err := s.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(s.Path()); err != nil {
		t.Fatalf("expected %s to exist: %v", s.Path(), err)
	}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}

func TestSaveCreatesNestedDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b", "dnssie")
	s := New(dir)
	if err := s.Save(Defaults()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(s.Path()); err != nil {
		t.Fatalf("expected %s to exist: %v", s.Path(), err)
	}
}

func TestNormalizeUpstream(t *testing.T) {
	cases := map[string]string{
		"1.1.1.1":                   "1.1.1.1:53",
		"1.1.1.1:5353":              "1.1.1.1:5353",
		"  8.8.8.8  ":               "8.8.8.8:53",
		"dns.example.com":           "dns.example.com:53",
		"2001:4860:4860::8888":      "[2001:4860:4860::8888]:53",
		"[2001:4860:4860::8888]:53": "[2001:4860:4860::8888]:53",
		"":                          "",
		"   ":                       "",
	}
	for in, want := range cases {
		if got := NormalizeUpstream(in); got != want {
			t.Errorf("NormalizeUpstream(%q) = %q, want %q", in, got, want)
		}
	}
}
