//go:build windows

package supervisor

import (
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

// stillActive is the GetExitCodeProcess code for a process that is still
// running (STILL_ACTIVE / STATUS_PENDING).
const stillActive = 259

// detachSysProcAttr starts the server detached from the TUI's console so it
// keeps running after the TUI exits.
func detachSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
	}
}

// processAlive reports whether pid refers to a live process.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}

// signalStop terminates the process. Windows has no SIGTERM delivery to a
// detached, console-less process, so this is an ungraceful kill (documented
// limitation; the Windows path is compile-checked but not runtime-tested).
func signalStop(p *os.Process) error {
	return p.Kill()
}
