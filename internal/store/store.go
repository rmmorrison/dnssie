// Package store persists dnssie's DNS records as TOML on disk.
//
// Records live in a single records.toml file inside dnssie's configuration
// directory:
//
//	Linux/macOS: ~/.config/dnssie/records.toml   (honoring $XDG_CONFIG_HOME)
//	Windows:     %AppData%\dnssie\records.toml
//
// The directory and file are created on the first save; a missing file is
// treated as "no records yet" rather than an error.
package store

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/rmmorrison/dnssie/internal/paths"
)

const recordsFile = "records.toml"

// DefaultTTL is the time-to-live (seconds) served for records that don't set
// one of their own. It is the single source of truth for that default; the
// server and UI both reference it.
const DefaultTTL uint32 = 300

// Record is a single DNS record managed by dnssie.
type Record struct {
	Type  string `toml:"type"`
	Name  string `toml:"name"`
	Value string `toml:"value"`
	// TTL is the record's time-to-live in seconds. A nil pointer means "use
	// the default" (and is omitted from records.toml, so files written by
	// older versions keep their behavior); a non-nil value — including 0 — is
	// served verbatim.
	TTL *uint32 `toml:"ttl,omitempty"`
}

// TTLOr returns the record's configured TTL, or def when it doesn't set one.
func (r Record) TTLOr(def uint32) uint32 {
	if r.TTL == nil {
		return def
	}
	return *r.TTL
}

// document is the on-disk shape of records.toml.
type document struct {
	Records []Record `toml:"record"`
}

// Store reads and writes records under a configuration directory.
type Store struct {
	dir string
}

// New returns a Store rooted at the given directory. It's mainly useful for
// tests; production code should use Default.
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

// Path is the absolute path to the records file.
func (s *Store) Path() string {
	return filepath.Join(s.dir, recordsFile)
}

// Load returns all persisted records. A missing file yields no records and no
// error, so callers can treat first run and "no records" the same way.
func (s *Store) Load() ([]Record, error) {
	var doc document
	if _, err := toml.DecodeFile(s.Path(), &doc); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", s.Path(), err)
	}
	return doc.Records, nil
}

// Save writes records, creating the configuration directory and file if they
// don't exist yet. The write is atomic: a temp file is written and then
// renamed over the target so a crash can't leave a half-written file.
func (s *Store) Save(records []Record) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", s.dir, err)
	}

	data, err := toml.Marshal(document{Records: records})
	if err != nil {
		return fmt.Errorf("encoding records: %w", err)
	}

	tmp, err := os.CreateTemp(s.dir, recordsFile+".*.tmp")
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

// Add appends a record and persists the updated set.
func (s *Store) Add(r Record) error {
	records, err := s.Load()
	if err != nil {
		return err
	}
	return s.Save(append(records, r))
}
