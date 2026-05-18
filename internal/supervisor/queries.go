package supervisor

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/rmmorrison/dnssie/internal/paths"
)

// queryLogFile mirrors internal/server's recent-lookups file name. It's a
// stable filename, intentionally duplicated rather than shared, so the two
// packages stay decoupled.
const queryLogFile = "queries.log"

// RecentQueries returns up to the last n lines the running server logged
// (most recent last). A missing log yields no lines and no error.
func RecentQueries(n int) ([]string, error) {
	dir, err := paths.ConfigDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, queryLogFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	trimmed := strings.TrimRight(string(data), "\n")
	if trimmed == "" {
		return nil, nil
	}
	lines := strings.Split(trimmed, "\n")
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}
