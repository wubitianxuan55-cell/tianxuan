package agent

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// toolCache caches read-only tool results (file reads) to avoid redundant disk IO
// within a turn. Write operations invalidate related entries. Thread-safe.
// Cache keys include path + offset so different read ranges have separate entries.
type toolCache struct {
	mu    sync.RWMutex
	items map[string]*cacheItem
	ttl   time.Duration
}

type cacheItem struct {
	content string
	mtime   time.Time
	cached  time.Time
}

// newToolCache creates a cache with the given TTL. Zero or negative TTL means
// no expiry (entries live until invalidated by a write or clear).
func newToolCache(ttl time.Duration) *toolCache {
	return &toolCache{
		items: make(map[string]*cacheItem),
		ttl:   ttl,
	}
}

// readKey builds a cache key from path and offset.
func readKey(path string, offset int) string {
	if offset == 0 {
		return path
	}
	return fmt.Sprintf("%s@%d", path, offset)
}

// get returns the cached content for a read_file(path, offset). Returns
// ("", false) on miss or stale entry.
func (c *toolCache) get(path string, offset int) (string, bool) {
	key := readKey(path, offset)
	c.mu.RLock()
	ci, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return "", false
	}
	// Fast path: TTL not expired and mtime still matches.
	if (c.ttl <= 0 || time.Since(ci.cached) <= c.ttl) {
		fi, err := os.Stat(path)
		if err != nil || !fi.ModTime().Equal(ci.mtime) {
			c.invalidatePath(path)
			return "", false
		}
		return ci.content, true
	}
	// Slow path: TTL expired. Re-check under write lock to avoid TOCTOU with
	// a concurrent set() that refreshed the entry between our RUnlock and Lock.
	c.mu.Lock()
	ci2, ok2 := c.items[key]
	if ok2 && ci2 == ci {
		// Same entry we read under RLock — safe to delete.
		delete(c.items, key)
	}
	c.mu.Unlock()
	return "", false
}

// set caches content for a read_file(path, offset).
func (c *toolCache) set(path string, offset int, content string) {
	key := readKey(path, offset)
	// Read mtime immediately for accurate invalidation
	var mtime time.Time
	if fi, err := os.Stat(path); err == nil {
		mtime = fi.ModTime()
	}
	c.mu.Lock()
	c.items[key] = &cacheItem{
		content: content,
		mtime:   mtime,
		cached:  time.Now(),
	}
	c.mu.Unlock()
}

// invalidatePath removes all cache entries for a given file path.
func (c *toolCache) invalidatePath(path string) {
	c.mu.Lock()
	prefix := path
	for k := range c.items {
		if k == prefix || (len(k) > len(prefix) && k[:len(prefix)] == prefix && k[len(prefix)] == '@') {
			delete(c.items, k)
		}
	}
	c.mu.Unlock()
}

// clear removes all cache entries. Called at the start of each turn.
func (c *toolCache) clear() {
	c.mu.Lock()
	clear(c.items)
	c.mu.Unlock()
}
