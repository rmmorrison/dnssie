package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readLog(t *testing.T, q *queryLog) string {
	t.Helper()
	data, err := os.ReadFile(q.path)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatalf("read log: %v", err)
	}
	return string(data)
}

func TestQueryLogAddBounded(t *testing.T) {
	q := newQueryLog(t.TempDir(), 3)
	for _, l := range []string{"l1", "l2", "l3", "l4", "l5"} {
		q.add(l)
	}
	// Ring keeps only the last 3, oldest first.
	if got := readLog(t, q); got != "l3\nl4\nl5\n" {
		t.Errorf("log = %q, want last 3 lines", got)
	}
}

func TestQueryLogResetTruncates(t *testing.T) {
	q := newQueryLog(t.TempDir(), 10)
	q.add("before")
	q.reset()
	if got := readLog(t, q); got != "" {
		t.Errorf("after reset log = %q, want empty", got)
	}
	q.add("after")
	if got := readLog(t, q); got != "after\n" {
		t.Errorf("post-reset log = %q, want only the new line", got)
	}
}

func TestQueryLogAtomicWriteNoTempLeak(t *testing.T) {
	dir := t.TempDir()
	q := newQueryLog(dir, 10)
	q.add("a")
	q.add("b")

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
	if got := readLog(t, q); got != "a\nb\n" {
		t.Errorf("log = %q", got)
	}
}

func TestQueryLogNilSafe(t *testing.T) {
	var q *queryLog // a Server built without a resolvable config dir
	q.reset()
	q.add("should not panic")
}

func TestQueryLogDirCreated(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dnssie")
	q := newQueryLog(dir, 5)
	q.add("x")
	if got := readLog(t, q); got != "x\n" {
		t.Errorf("log = %q, want x written into freshly created dir", got)
	}
}
