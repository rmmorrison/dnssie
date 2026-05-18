package server

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// queryLogFile is the bounded, display-ready log of recent lookups the TUI
// tails while the DNS server screen is open.
const queryLogFile = "queries.log"

// queryLog keeps the most recent lookups in memory and mirrors them to a
// small file, rewritten atomically. It is safe for concurrent use (miekg/dns
// dispatches each query on its own goroutine).
type queryLog struct {
	path string
	max  int

	mu   sync.Mutex
	ring []string
}

func newQueryLog(dir string, max int) *queryLog {
	_ = os.MkdirAll(dir, 0o755)
	return &queryLog{path: filepath.Join(dir, queryLogFile), max: max}
}

// reset clears the in-memory ring and truncates the file so a freshly started
// server doesn't surface a previous run's lookups.
func (q *queryLog) reset() {
	if q == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.ring = nil
	_ = os.WriteFile(q.path, nil, 0o644)
}

// add appends one display-ready line, keeping only the most recent max, and
// rewrites the file atomically (temp + rename) so the tailing TUI never reads
// a partial line.
func (q *queryLog) add(line string) {
	if q == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()

	q.ring = append(q.ring, line)
	if len(q.ring) > q.max {
		q.ring = q.ring[len(q.ring)-q.max:]
	}

	data := strings.Join(q.ring, "\n") + "\n"
	dir := filepath.Dir(q.path)
	tmp, err := os.CreateTemp(dir, queryLogFile+".*.tmp")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return
	}
	_ = os.Rename(tmpName, q.path)
}
