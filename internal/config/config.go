// Package config persists dnssie's DNS server settings as TOML on disk.
//
// Settings live in a config.toml file alongside records.toml in dnssie's
// configuration directory:
//
//	Linux/macOS: ~/.config/dnssie/config.toml   (honoring $XDG_CONFIG_HOME)
//	Windows:     %AppData%\dnssie\config.toml
//
// The directory and file are created on the first save; a missing file yields
// the built-in defaults rather than an error.
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/rmmorrison/dnssie/internal/paths"
)

const (
	configFile = "config.toml"

	// ModeSystem forwards unmatched queries to the OS-configured resolvers.
	ModeSystem = "system"
	// ModeManual forwards unmatched queries to Resolvers.Upstream.
	ModeManual = "manual"

	// defaultPort is unprivileged so dnssie runs without root/admin. It
	// avoids 5353, which collides with mDNS/Bonjour (mDNSResponder on
	// macOS, avahi on Linux). Users who can bind 53 may set it in the UI.
	defaultPort = 1053
	// defaultDNS is the standard DNS port, assumed for upstream resolvers
	// that don't specify one.
	defaultDNS = "53"
)

// ErrSystemResolversUnavailable is returned by SystemResolvers when dnssie
// can't determine the OS resolvers on the current platform.
var ErrSystemResolversUnavailable = errors.New("system resolvers are not available on this platform")

// Resolvers describes how unmatched lookups are forwarded.
type Resolvers struct {
	// Mode is ModeSystem or ModeManual.
	Mode string `toml:"mode"`
	// Upstream is the manually configured resolver list (host:port), used
	// when Mode is ModeManual.
	Upstream []string `toml:"upstream"`
}

// Config is dnssie's DNS server configuration.
type Config struct {
	// Port is the UDP/TCP port the server listens on.
	Port      int       `toml:"port"`
	Resolvers Resolvers `toml:"resolvers"`
}

// Defaults returns the configuration used when no config file exists yet.
func Defaults() Config {
	return Config{
		Port:      defaultPort,
		Resolvers: Resolvers{Mode: ModeSystem},
	}
}

// Store reads and writes the config file under a configuration directory.
type Store struct {
	dir string
}

// New returns a Store rooted at dir. Mainly for tests; production code should
// use Default.
func New(dir string) *Store {
	return &Store{dir: dir}
}

// Default returns a Store rooted at dnssie's standard configuration directory.
func Default() (*Store, error) {
	dir, err := paths.ConfigDir()
	if err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

// Path is the absolute path to the config file.
func (s *Store) Path() string {
	return filepath.Join(s.dir, configFile)
}

// Load returns the persisted configuration. A missing file yields Defaults
// and no error, so first run and "no config" behave the same.
func (s *Store) Load() (Config, error) {
	cfg := Defaults()
	if _, err := toml.DecodeFile(s.Path(), &cfg); err != nil {
		if os.IsNotExist(err) {
			return Defaults(), nil
		}
		return Defaults(), fmt.Errorf("reading %s: %w", s.Path(), err)
	}
	return cfg, nil
}

// Save writes cfg, creating the configuration directory and file if needed.
// The write is atomic: a temp file is written then renamed over the target.
func (s *Store) Save(cfg Config) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", s.dir, err)
	}

	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}

	tmp, err := os.CreateTemp(s.dir, configFile+".*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("writing %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, s.Path()); err != nil {
		return fmt.Errorf("saving %s: %w", s.Path(), err)
	}
	return nil
}

// NormalizeUpstream trims s and ensures it carries a port, defaulting to 53.
// It returns "" if s has no host part.
func NormalizeUpstream(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if _, _, err := net.SplitHostPort(s); err == nil {
		return s // already host:port (or [v6]:port)
	}
	// No port. Bracket bare IPv6 literals so the port is unambiguous.
	if strings.Count(s, ":") >= 2 {
		return "[" + s + "]:" + defaultDNS
	}
	return s + ":" + defaultDNS
}
