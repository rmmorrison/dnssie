package server

import (
	"os"
	"testing"
	"time"

	"github.com/rmmorrison/dnssie/internal/store"
)

func TestRecordCacheMissingFile(t *testing.T) {
	c := newRecordCache(store.New(t.TempDir()))
	if got := c.snapshot(); got != nil {
		t.Errorf("snapshot on missing file = %v, want nil", got)
	}
}

func TestRecordCacheReloadsOnChange(t *testing.T) {
	dir := t.TempDir()
	st := store.New(dir)
	if err := st.Save([]store.Record{{Type: "A", Name: "a.", Value: "1.1.1.1"}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	c := newRecordCache(st)

	if got := c.snapshot(); len(got) != 1 || got[0].Value != "1.1.1.1" {
		t.Fatalf("first snapshot = %v", got)
	}

	if err := st.Save([]store.Record{
		{Type: "A", Name: "a.", Value: "1.1.1.1"},
		{Type: "A", Name: "b.", Value: "2.2.2.2"},
	}); err != nil {
		t.Fatalf("Save 2: %v", err)
	}
	// Force a newer mtime so the change is observed regardless of FS
	// timestamp granularity.
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(st.Path(), future, future); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	if got := c.snapshot(); len(got) != 2 {
		t.Errorf("snapshot after change = %v, want 2 records", got)
	}
}
