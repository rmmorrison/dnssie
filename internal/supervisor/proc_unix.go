//go:build linux || darwin

package supervisor

import (
	"errors"
	"os"
	"syscall"
)

// detachSysProcAttr puts the server in its own session so it survives the
// TUI exiting (it is reparented to init/launchd).
func detachSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

// processAlive reports whether pid refers to a live process. Signal 0 probes
// existence without affecting the target; EPERM means it exists but is owned
// by another user.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

// signalStop asks the process to shut down gracefully.
func signalStop(p *os.Process) error {
	return p.Signal(syscall.SIGTERM)
}
