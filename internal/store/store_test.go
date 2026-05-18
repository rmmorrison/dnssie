package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func u32(v uint32) *uint32 { return &v }

func TestLoadMissingFileReturnsEmpty(t *testing.T) {
	s := New(t.TempDir())
	records, err := s.Load()
	if err != nil {
		t.Fatalf("Load on missing file: unexpected error: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("Load on missing file: got %d records, want 0", len(records))
	}
}

func TestSaveCreatesDirAndFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dnssie")
	s := New(dir)

	if err := s.Save([]Record{{Type: "A", Name: "www", Value: "192.0.2.1"}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(s.Path()); err != nil {
		t.Fatalf("expected %s to exist: %v", s.Path(), err)
	}
}

func TestRecordTTLOr(t *testing.T) {
	if got := (Record{}).TTLOr(300); got != 300 {
		t.Errorf("unset TTLOr = %d, want 300", got)
	}
	if got := (Record{TTL: u32(0)}).TTLOr(300); got != 0 {
		t.Errorf("explicit 0 TTLOr = %d, want 0", got)
	}
	if got := (Record{TTL: u32(60)}).TTLOr(300); got != 60 {
		t.Errorf("explicit 60 TTLOr = %d, want 60", got)
	}
}

func TestTTLRoundTrip(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Save([]Record{
		{Type: "A", Name: "a.", Value: "1", TTL: nil},      // inherit default
		{Type: "A", Name: "b.", Value: "2", TTL: u32(0)},   // explicit zero
		{Type: "A", Name: "c.", Value: "3", TTL: u32(120)}, // explicit value
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d records, want 3", len(got))
	}
	if got[0].TTL != nil {
		t.Errorf("unset TTL round-tripped as %d, want nil", *got[0].TTL)
	}
	if got[1].TTL == nil || *got[1].TTL != 0 {
		t.Errorf("explicit 0 TTL round-tripped as %v, want 0 (non-nil)", got[1].TTL)
	}
	if got[2].TTL == nil || *got[2].TTL != 120 {
		t.Errorf("explicit 120 TTL round-tripped as %v, want 120", got[2].TTL)
	}

	// A record that inherits the default must not write a ttl key, so files
	// from older dnssie versions keep behaving exactly as before.
	data, err := os.ReadFile(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if c := strings.Count(string(data), "ttl"); c != 2 {
		t.Errorf("ttl key count = %d, want 2 (only the explicit records)\n%s", c, data)
	}
}

func TestAddAppendsAndRoundTrips(t *testing.T) {
	s := New(t.TempDir())

	want := []Record{
		{Type: "A", Name: "www", Value: "192.0.2.1"},
		{Type: "TXT", Name: "@", Value: "v=spf1 -all"},
	}
	for _, r := range want {
		if err := s.Add(r); err != nil {
			t.Fatalf("Add(%v): %v", r, err)
		}
	}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d records, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("record %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}
