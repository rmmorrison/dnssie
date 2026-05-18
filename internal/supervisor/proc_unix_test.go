//go:build linux || darwin

package supervisor

import (
	"os"
	"testing"
)

func TestDetachSysProcAttrSetsid(t *testing.T) {
	if a := detachSysProcAttr(); a == nil || !a.Setsid {
		t.Errorf("detachSysProcAttr() = %+v, want Setsid true", a)
	}
}

func TestProcessAlive(t *testing.T) {
	if !processAlive(os.Getpid()) {
		t.Error("processAlive(self) = false, want true")
	}
	if processAlive(-1) {
		t.Error("processAlive(-1) = true, want false")
	}
	if processAlive(1 << 30) {
		t.Error("processAlive(huge bogus pid) = true, want false")
	}
}
