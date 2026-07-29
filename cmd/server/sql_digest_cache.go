package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// sqlDigestFulltextEntry caches the longest known SQL text for a digest.
type sqlDigestFulltextEntry struct {
	SQL       string `json:"sql"`
	Source    string `json:"source,omitempty"`
	UpdatedAt int64  `json:"updated_at"`
	Chars     int    `json:"chars"`
}

// sqlDigestFulltextCache persists full SQL samples keyed by connection + digest
// under the slow-SQL data dir (file-backed, no DB migration required).
type sqlDigestFulltextCache struct {
	mu   sync.Mutex
	dir  string
	data map[string]map[string]sqlDigestFulltextEntry // connID -> digest -> entry
}

func newSQLDigestFulltextCache(dir string) *sqlDigestFulltextCache {
	c := &sqlDigestFulltextCache{
		dir:  dir,
		data: map[string]map[string]sqlDigestFulltextEntry{},
	}
	c.load()
	return c
}

func (c *sqlDigestFulltextCache) path(connID string) string {
	return filepath.Join(c.dir, "digest-fulltext-"+sanitizeFilePart(connID)+".json")
}

func (c *sqlDigestFulltextCache) load() {
	if c == nil || c.dir == "" {
		return
	}
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "digest-fulltext-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(c.dir, name))
		if err != nil {
			continue
		}
		var m map[string]sqlDigestFulltextEntry
		if json.Unmarshal(b, &m) != nil || len(m) == 0 {
			continue
		}
		connID := strings.TrimSuffix(strings.TrimPrefix(name, "digest-fulltext-"), ".json")
		c.data[connID] = m
	}
}

func (c *sqlDigestFulltextCache) saveConnLocked(connID string) {
	if c == nil || c.dir == "" || connID == "" {
		return
	}
	m := c.data[connID]
	if m == nil {
		m = map[string]sqlDigestFulltextEntry{}
	}
	_ = os.MkdirAll(c.dir, 0o750)
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return
	}
	tmp := c.path(connID) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, c.path(connID))
}

func (c *sqlDigestFulltextCache) Get(connID, digest string) (sqlDigestFulltextEntry, bool) {
	if c == nil {
		return sqlDigestFulltextEntry{}, false
	}
	connID = strings.TrimSpace(connID)
	digest = strings.TrimSpace(digest)
	if connID == "" || digest == "" {
		return sqlDigestFulltextEntry{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	m := c.data[connID]
	if m == nil {
		return sqlDigestFulltextEntry{}, false
	}
	e, ok := m[digest]
	if !ok || strings.TrimSpace(e.SQL) == "" {
		return sqlDigestFulltextEntry{}, false
	}
	return e, true
}

// Put stores sql when it is longer than the cached sample (or clears placeholders).
func (c *sqlDigestFulltextCache) Put(connID, digest, sqlText, source string) {
	if c == nil {
		return
	}
	connID = strings.TrimSpace(connID)
	digest = strings.TrimSpace(digest)
	sqlText = strings.TrimSpace(sqlText)
	if connID == "" || digest == "" || sqlText == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data[connID] == nil {
		c.data[connID] = map[string]sqlDigestFulltextEntry{}
	}
	cur, ok := c.data[connID][digest]
	if ok && !shouldPreferRecoveredSQL(cur.SQL, sqlText) {
		return
	}
	c.data[connID][digest] = sqlDigestFulltextEntry{
		SQL:       sqlText,
		Source:    source,
		UpdatedAt: time.Now().Unix(),
		Chars:     len(sqlText),
	}
	// Cap per connection to avoid unbounded growth.
	const maxPerConn = 500
	if len(c.data[connID]) > maxPerConn {
		c.pruneOldestLocked(connID, maxPerConn/2)
	}
	c.saveConnLocked(connID)
}

func (c *sqlDigestFulltextCache) pruneOldestLocked(connID string, keep int) {
	m := c.data[connID]
	if len(m) <= keep {
		return
	}
	type pair struct {
		d string
		t int64
	}
	list := make([]pair, 0, len(m))
	for d, e := range m {
		list = append(list, pair{d: d, t: e.UpdatedAt})
	}
	// Simple selection: drop oldest until keep.
	for len(list) > keep {
		minI := 0
		for i := 1; i < len(list); i++ {
			if list[i].t < list[minI].t {
				minI = i
			}
		}
		delete(m, list[minI].d)
		list[minI] = list[len(list)-1]
		list = list[:len(list)-1]
	}
}
