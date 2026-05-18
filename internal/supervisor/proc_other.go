//go:build !linux && !darwin && !windows

package supervisor

import (
	"os"
	"syscall"
)

// These platforms have no TUI (bubbletea is unsupported there) so this path
// is effectively unreachable at runtime; the stubs exist only so the package
// stays cross-compilable, mirroring config's resolvers_other.go.

func detachSysProcAttr() *syscall.SysProcAttr { return nil }

func processAlive(int) bool { return false }

func signalStop(p *os.Process) error { return p.Kill() }
