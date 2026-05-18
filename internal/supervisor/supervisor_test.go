package supervisor

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"

	"github.com/rmmorrison/dnssie/internal/config"
	"github.com/rmmorrison/dnssie/internal/store"
)

func TestStaleStateIsCleaned(t *testing.T) {
	t.Setenv("DNSSIE_CONFIG_DIR", t.TempDir())
	if err := writeState(State{PID: 1 << 30, Addr: "127.0.0.1:5353", Port: 5353, Started: time.Now()}); err != nil {
		t.Fatalf("writeState: %v", err)
	}
	running, _, err := Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if running {
		t.Error("Status() = running, want stopped for a dead PID")
	}
	sp, _ := statePath()
	if _, err := os.Stat(sp); !os.IsNotExist(err) {
		t.Errorf("stale state file not removed (stat err = %v)", err)
	}
}

func TestStatusNoStateFile(t *testing.T) {
	t.Setenv("DNSSIE_CONFIG_DIR", t.TempDir())
	running, _, err := Status()
	if err != nil || running {
		t.Errorf("Status() = (%v, err=%v), want (false, nil)", running, err)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatalf("go env GOMOD: %v", err)
	}
	return filepath.Dir(strings.TrimSpace(string(out)))
}

func TestLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary and spawns a process")
	}

	bin := filepath.Join(t.TempDir(), "dnssie")
	if runtime.GOOS == "windows" {
		bin += ".exe" // Windows won't exec a file without a known extension
	}
	build := exec.Command("go", "build", "-o", bin, "./cmd/dnssie")
	build.Dir = moduleRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build dnssie: %v\n%s", err, out)
	}
	old := osExecutable
	osExecutable = func() (string, error) { return bin, nil }
	t.Cleanup(func() { osExecutable = old })

	cfgDir := t.TempDir()
	t.Setenv("DNSSIE_CONFIG_DIR", cfgDir)

	port := freePort(t)
	if err := store.New(cfgDir).Save([]store.Record{
		{Type: "A", Name: "seed.test.", Value: "203.0.113.9"},
	}); err != nil {
		t.Fatalf("seed records: %v", err)
	}
	cfg := config.Config{Port: port, Resolvers: config.Resolvers{Mode: config.ModeManual}}
	if err := config.New(cfgDir).Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	if err := Start(cfg); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = Stop() })

	running, info, err := Status()
	if err != nil || !running {
		t.Fatalf("Status after Start = (%v, err=%v), want running", running, err)
	}
	if info.Port != port {
		t.Errorf("info.Port = %d, want %d", info.Port, port)
	}

	if err := Start(cfg); err != ErrAlreadyRunning {
		t.Errorf("second Start = %v, want ErrAlreadyRunning", err)
	}

	m := new(dns.Msg)
	m.SetQuestion("seed.test.", dns.TypeA)
	resp, _, err := (&dns.Client{Timeout: 2 * time.Second}).Exchange(m, net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("exchange against detached server: %v", err)
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("answers = %d, want 1", len(resp.Answer))
	}
	if a, ok := resp.Answer[0].(*dns.A); !ok || a.A.String() != "203.0.113.9" {
		t.Errorf("answer = %v, want 203.0.113.9", resp.Answer[0])
	}

	if err := Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	running, _, _ = Status()
	if running {
		t.Error("Status after Stop = running, want stopped")
	}
	sp, _ := statePath()
	if _, err := os.Stat(sp); !os.IsNotExist(err) {
		t.Errorf("state file not removed after Stop (stat err = %v)", err)
	}
}
