// Package server implements dnssie's DNS server: it answers queries from the
// stored records and forwards everything else to the configured upstreams.
package server

import (
	"os"
	"sync"
	"time"

	"github.com/rmmorrison/dnssie/internal/store"
)

// recordCache serves records from store.Store, reloading from disk only when
// records.toml's mtime changes. store.Save writes atomically (temp + rename),
// so a reader never observes a partially written file.
type recordCache struct {
	st *store.Store

	mu     sync.RWMutex
	mod    time.Time
	loaded bool
	recs   []store.Record
}

func newRecordCache(st *store.Store) *recordCache {
	return &recordCache{st: st}
}

// snapshot returns the current records, reloading first if the file changed.
// A missing or unreadable file means "no records" — never fatal to a running
// server.
func (c *recordCache) snapshot() []store.Record {
	fi, err := os.Stat(c.st.Path())
	if err != nil {
		c.mu.Lock()
		c.loaded, c.recs = false, nil
		c.mu.Unlock()
		return nil
	}
	mod := fi.ModTime()

	c.mu.RLock()
	if c.loaded && mod.Equal(c.mod) {
		recs := c.recs
		c.mu.RUnlock()
		return recs
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loaded && mod.Equal(c.mod) { // another goroutine may have reloaded
		return c.recs
	}
	recs, err := c.st.Load()
	if err != nil {
		return c.recs // keep the last good snapshot on a transient error
	}
	c.recs, c.mod, c.loaded = recs, mod, true
	return c.recs
}
