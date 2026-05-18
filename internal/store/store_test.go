package store

import (
	"os"
	"path/filepath"
	"testing"
)

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
