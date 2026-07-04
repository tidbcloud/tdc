//go:build !windows

package fs

import (
	"container/list"
	"strings"
	"sync"
	"time"
)

type fuseReadCache struct {
	mu      sync.Mutex
	items   map[string]*fuseReadCacheEntry
	order   *list.List
	size    int64
	maxSize int64
	maxFile int64
	ttl     time.Duration
}

type fuseReadCacheEntry struct {
	path    string
	data    []byte
	version fuseObjectVersion
	expires time.Time
	elem    *list.Element
}

func newFuseReadCache(maxSize, maxFile int64, ttl time.Duration) *fuseReadCache {
	if maxSize <= 0 {
		maxSize = defaultFuseReadCacheSizeBytes
	}
	if maxFile <= 0 {
		maxFile = defaultFuseReadCacheMaxFileBytes
	}
	if ttl == 0 {
		ttl = defaultFuseReadCacheTTL
	}
	return &fuseReadCache{
		items:   map[string]*fuseReadCacheEntry{},
		order:   list.New(),
		maxSize: maxSize,
		maxFile: maxFile,
		ttl:     ttl,
	}
}

func (c *fuseReadCache) get(remotePath string, version fuseObjectVersion) ([]byte, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.items[remotePath]
	if !ok {
		return nil, false
	}
	if !entry.version.matches(version) {
		return nil, false
	}
	if c.ttl > 0 && time.Now().After(entry.expires) {
		c.evict(entry)
		return nil, false
	}
	c.order.MoveToFront(entry.elem)
	return append([]byte(nil), entry.data...), true
}

func (c *fuseReadCache) put(remotePath string, data []byte, version fuseObjectVersion) {
	if c == nil || int64(len(data)) > c.maxFile {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	stored := append([]byte(nil), data...)
	now := time.Now()
	if entry, ok := c.items[remotePath]; ok {
		c.size -= int64(len(entry.data))
		entry.data = stored
		entry.version = version
		entry.expires = now.Add(c.ttl)
		c.size += int64(len(stored))
		c.order.MoveToFront(entry.elem)
	} else {
		entry := &fuseReadCacheEntry{
			path:    remotePath,
			data:    stored,
			version: version,
			expires: now.Add(c.ttl),
		}
		entry.elem = c.order.PushFront(entry)
		c.items[remotePath] = entry
		c.size += int64(len(stored))
	}

	for c.size > c.maxSize && c.order.Len() > 0 {
		tail := c.order.Back()
		if tail == nil {
			break
		}
		c.evict(tail.Value.(*fuseReadCacheEntry))
	}
}

func (c *fuseReadCache) invalidate(remotePath string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.items[remotePath]; ok {
		c.evict(entry)
	}
}

func (c *fuseReadCache) invalidatePrefix(prefix string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for remotePath, entry := range c.items {
		if remotePath == prefix || strings.HasPrefix(remotePath, treePrefix(prefix)) {
			c.evict(entry)
		}
	}
}

func (c *fuseReadCache) evict(entry *fuseReadCacheEntry) {
	delete(c.items, entry.path)
	c.order.Remove(entry.elem)
	c.size -= int64(len(entry.data))
}
