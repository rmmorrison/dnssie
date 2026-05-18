// Package supervisor starts, inspects, and stops the dnssie DNS server as a
// process detached from the TUI, so the server keeps running after the TUI
// exits. State lives in server.toml beside records.toml/config.toml.
package supervisor

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/rmmorrison/dnssie/internal/config"
	"github.com/rmmorrison/dnssie/internal/paths"
)

const (
	stateFile = "server.toml"
	logFile   = "server.log"
)

// ErrAlreadyRunning is returned by Start when a server is already running.
var ErrAlreadyRunning = errors.New("dnssie server is already running")

// osExecutable resolves the dnssie binary to spawn. It's a variable so tests
// can point it at a freshly built binary instead of the test runner.
var osExecutable = os.Executable

// State is the persisted record of the running server (TOML, like the rest
// of dnssie's on-disk files).
type State struct {
	PID     int       `toml:"pid"`
	Addr    string    `toml:"addr"`
	Port    int       `toml:"port"`
	Started time.Time `toml:"started"`
}

// Info reports the running server's details.
type Info struct {
	State
}

func statePath() (string, error) {
	dir, err := paths.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, stateFile), nil
}

func logFilePath() (string, error) {
	dir, err := paths.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, logFile), nil
}

func readState() (State, error) {
	var st State
	path, err := statePath()
	if err != nil {
		return st, err
	}
	_, err = toml.DecodeFile(path, &st)
	return st, err
}

func writeState(st State) error {
	dir, err := paths.ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	data, err := toml.Marshal(st)
	if err != nil {
		return fmt.Errorf("encoding state: %w", err)
	}
	tmp, err := os.CreateTemp(dir, stateFile+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, filepath.Join(dir, stateFile))
}

func removeState() {
	if path, err := statePath(); err == nil {
		_ = os.Remove(path)
	}
}

// Status reports whether the server is running. A state file referencing a
// dead PID is treated as stopped and cleaned up.
func Status() (bool, Info, error) {
	st, err := readState()
	if err != nil {
		if os.IsNotExist(err) {
			return false, Info{}, nil
		}
		return false, Info{}, err
	}
	if !processAlive(st.PID) {
		removeState() // stale
		return false, Info{}, nil
	}
	return true, Info{st}, nil
}

// Start launches `dnssie serve` as a detached child process and records its
// state. It is a no-op error (ErrAlreadyRunning) if one is already running.
func Start(cfg config.Config) error {
	if running, _, err := Status(); err != nil {
		return err
	} else if running {
		return ErrAlreadyRunning
	}

	exe, err := osExecutable()
	if err != nil {
		return err
	}
	dir, err := paths.ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	lp, err := logFilePath()
	if err != nil {
		return err
	}
	lf, err := os.OpenFile(lp, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer lf.Close() // the child keeps its own inherited descriptor

	cmd := exec.Command(exe, "serve")
	cmd.Stdout = lf
	cmd.Stderr = lf
	cmd.Stdin = nil
	cmd.Dir = dir
	cmd.SysProcAttr = detachSysProcAttr()
	// Inherit the environment so the child resolves the same config dir
	// (XDG_CONFIG_HOME etc.) as the TUI.

	if err := cmd.Start(); err != nil {
		return err
	}
	pid := cmd.Process.Pid
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(cfg.Port))

	if err := writeState(State{PID: pid, Addr: addr, Port: cfg.Port, Started: time.Now()}); err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	// Reap the child so a stopped server doesn't linger as a zombie while
	// the TUI is still alive. If the TUI exits first this goroutine simply
	// dies and the (detached) child is reparented to init/launchd.
	go func() { _ = cmd.Wait() }()

	// Startup probe: if the child dies immediately (port in use, privileged
	// port), surface the tail of its log instead of a silent failure.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			removeState()
			return fmt.Errorf("server exited on startup:\n%s", logTail(lp))
		}
		if c, derr := net.DialTimeout("tcp", addr, 200*time.Millisecond); derr == nil {
			c.Close()
			return nil // confirmed listening
		}
		time.Sleep(150 * time.Millisecond)
	}
	if !processAlive(pid) {
		removeState()
		return fmt.Errorf("server exited on startup:\n%s", logTail(lp))
	}
	return nil // alive; may still be binding
}

// Stop signals the running server to exit and waits briefly for it to do so,
// escalating to a kill if needed. It is idempotent.
func Stop() error {
	running, info, err := Status()
	if err != nil {
		return err
	}
	if !running {
		removeState()
		return nil
	}

	proc, err := os.FindProcess(info.PID)
	if err != nil {
		removeState()
		return nil
	}
	if err := signalStop(proc); err != nil {
		return err
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(info.PID) {
			removeState()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = proc.Kill() // escalate
	removeState()
	return nil
}

// logTail returns roughly the last 2KB of the server log for diagnostics.
func logTail(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "(no log available)"
	}
	const max = 2048
	if len(data) > max {
		data = data[len(data)-max:]
	}
	if len(data) == 0 {
		return "(log empty)"
	}
	return string(data)
}
