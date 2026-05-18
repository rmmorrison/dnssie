package paths

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestConfigDirOverride(t *testing.T) {
	// The override takes precedence on every platform, even over XDG.
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	t.Setenv("DNSSIE_CONFIG_DIR", filepath.Join("custom", "place"))

	got, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir: %v", err)
	}
	if want := filepath.Join("custom", "place"); got != want {
		t.Errorf("ConfigDir() = %q, want %q (exact override, no suffix)", got, want)
	}
}

func TestConfigDirHonorsXDG(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("XDG_CONFIG_HOME is not used on Windows")
	}
	t.Setenv("DNSSIE_CONFIG_DIR", "")
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
	t.Setenv("DNSSIE_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	got, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir: %v", err)
	}
	if !strings.HasSuffix(got, filepath.Join(".config", "dnssie")) {
		t.Errorf("ConfigDir() = %q, want it to end with .config/dnssie", got)
	}
}
