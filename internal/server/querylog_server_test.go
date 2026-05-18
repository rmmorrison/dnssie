package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"

	"github.com/rmmorrison/dnssie/internal/config"
	"github.com/rmmorrison/dnssie/internal/store"
)

// waitForLog polls queries.log under dir until it contains want, or fails.
func waitForLog(t *testing.T, dir, want string) {
	t.Helper()
	path := filepath.Join(dir, "queries.log")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(path); err == nil && strings.Contains(string(data), want) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	data, _ := os.ReadFile(path)
	t.Fatalf("queries.log never contained %q; got:\n%s", want, data)
}

func TestServerLogsLocalAndForwarded(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	dir := filepath.Join(xdg, "dnssie")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := store.New(dir).Save([]store.Record{
		{Type: "A", Name: "local.test.", Value: "1.2.3.4"},
	}); err != nil {
		t.Fatalf("seed records: %v", err)
	}
	stub := startStubUpstream(t, "203.0.113.5")
	if err := config.New(dir).Save(config.Config{
		Port:      5353,
		Resolvers: config.Resolvers{Mode: config.ModeManual, Upstream: []string{stub}},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	addr := runServer(t, dir, dir)

	query(t, addr, "local.test", dns.TypeA)       // -> local
	query(t, addr, "miss.example.org", dns.TypeA) // -> forwarded NOERROR (stub)

	waitForLog(t, dir, "local.test.")
	waitForLog(t, dir, "miss.example.org.")

	data, _ := os.ReadFile(filepath.Join(dir, "queries.log"))
	logs := string(data)
	for _, want := range []string{"A     local", "A     forwarded NOERROR"} {
		if !strings.Contains(logs, want) {
			t.Errorf("queries.log missing %q; got:\n%s", want, logs)
		}
	}
}

func TestServerLogsServfail(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	dir := filepath.Join(xdg, "dnssie")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Manual mode with no upstreams -> unmatched queries SERVFAIL.
	if err := config.New(dir).Save(config.Config{
		Port:      5353,
		Resolvers: config.Resolvers{Mode: config.ModeManual},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	addr := runServer(t, dir, dir)
	query(t, addr, "nowhere.example.org", dns.TypeA)

	waitForLog(t, dir, "nowhere.example.org.")
	data, _ := os.ReadFile(filepath.Join(dir, "queries.log"))
	if !strings.Contains(string(data), "servfail") {
		t.Errorf("expected a servfail entry; got:\n%s", data)
	}
}

func TestServerResetsLogOnStart(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	dir := filepath.Join(xdg, "dnssie")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A leftover log from a previous run.
	if err := os.WriteFile(filepath.Join(dir, "queries.log"), []byte("STALE ENTRY\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	runServer(t, dir, dir) // Run() calls qlog.reset()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(filepath.Join(dir, "queries.log"))
		if !strings.Contains(string(data), "STALE ENTRY") {
			return // reset cleared it
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("queries.log still contained the stale entry after server start")
}
