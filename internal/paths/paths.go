// Package paths resolves dnssie's on-disk locations.
package paths

import (
	"os"
	"path/filepath"
	"runtime"
)

// ConfigDir resolves dnssie's configuration directory per platform:
//
//	Linux/macOS: ~/.config/dnssie   (honoring $XDG_CONFIG_HOME)
//	Windows:     %AppData%\dnssie
//
// It does not create the directory.
func ConfigDir() (string, error) {
	if runtime.GOOS == "windows" {
		base, err := os.UserConfigDir() // %AppData%
		if err != nil {
			return "", err
		}
		return filepath.Join(base, "dnssie"), nil
	}

	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "dnssie"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "dnssie"), nil
}
