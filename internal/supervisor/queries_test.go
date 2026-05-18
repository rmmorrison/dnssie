package supervisor

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeQueryLog(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("DNSSIE_CONFIG_DIR", dir)
	if body != "" {
		if err := os.WriteFile(filepath.Join(dir, queryLogFile), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRecentQueriesMissingFile(t *testing.T) {
	t.Setenv("DNSSIE_CONFIG_DIR", t.TempDir())
	got, err := RecentQueries(10)
	if err != nil || got != nil {
		t.Errorf("RecentQueries() = (%v, %v), want (nil, nil)", got, err)
	}
}

func TestRecentQueriesEmptyFile(t *testing.T) {
	writeQueryLog(t, "\n\n")
	got, err := RecentQueries(10)
	if err != nil || got != nil {
		t.Errorf("RecentQueries() = (%v, %v), want (nil, nil)", got, err)
	}
}

func TestRecentQueriesTail(t *testing.T) {
	writeQueryLog(t, "l1\nl2\nl3\nl4\nl5\n")

	got, err := RecentQueries(3)
	if err != nil {
		t.Fatalf("RecentQueries: %v", err)
	}
	if want := []string{"l3", "l4", "l5"}; !reflect.DeepEqual(got, want) {
		t.Errorf("RecentQueries(3) = %v, want %v", got, want)
	}

	all := []string{"l1", "l2", "l3", "l4", "l5"}
	if got, _ := RecentQueries(10); !reflect.DeepEqual(got, all) {
		t.Errorf("RecentQueries(10) = %v, want all %v", got, all)
	}
	if got, _ := RecentQueries(0); !reflect.DeepEqual(got, all) {
		t.Errorf("RecentQueries(0) = %v, want all %v", got, all)
	}
}
