package paths

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestConfigDirHonorsXDG(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("XDG_CONFIG_HOME is not used on Windows")
	}
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")

	got, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir: %v", err)
	}
	if want := filepath.Join("/tmp/xdg", "dnssie"); got != want {
		t.Errorf("ConfigDir() = %q, want %q", got, want)
	}
}

func TestConfigDirFallsBackToHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("home fallback path differs on Windows")
	}
	t.Setenv("XDG_CONFIG_HOME", "")

	got, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir: %v", err)
	}
	if !strings.HasSuffix(got, filepath.Join(".config", "dnssie")) {
		t.Errorf("ConfigDir() = %q, want it to end with .config/dnssie", got)
	}
}
