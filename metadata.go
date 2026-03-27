package main

import (
	"sync"
	"time"
)

// Metadata holds cached file attributes without the actual content.
type Metadata struct {
	Size    int64     // File size in bytes
	ModTime time.Time // Last modification time
	ETag    string    // ETag for cache validation
}

// metadataEntry wraps Metadata with a timestamp for TTL tracking.
type metadataEntry struct {
	Metadata  Metadata
	FetchedAt time.Time
}

// MetadataCache provides thread-safe caching of file metadata with TTL-based expiration.
// Unlike CacheManager which caches file content, this only caches file attributes.
type MetadataCache struct {
	mu       sync.RWMutex
	entries  map[string]*metadataEntry
	ttl      time.Duration
	hits     uint64
	misses   uint64
	expired  uint64
}

// NewMetadataCache creates a new metadata cache with the specified TTL.
// TTL should typically be short (10-30 seconds) since metadata changes infrequently
// but we want to detect changes reasonably quickly.
func NewMetadataCache(ttl time.Duration) *MetadataCache {
	return &MetadataCache{
		entries: make(map[string]*metadataEntry),
		ttl:     ttl,
	}
}

// Get retrieves metadata for a URL.
// Returns:
//   - metadata: the cached metadata (valid only if found is true and expired is false)
//   - found: true if an entry exists for this URL
//   - expired: true if the entry exists but has exceeded its TTL
func (c *MetadataCache) Get(url string) (Metadata, bool, bool) {
	c.mu.RLock()
	entry, exists := c.entries[url]
	c.mu.RUnlock()

	if !exists {
		c.mu.Lock()
		c.misses++
		c.mu.Unlock()
		return Metadata{}, false, false
	}

	isExpired := time.Since(entry.FetchedAt) >= c.ttl

	c.mu.Lock()
	if isExpired {
		c.expired++
	} else {
		c.hits++
	}
	c.mu.Unlock()

	return entry.Metadata, true, isExpired
}

// Set stores metadata for a URL with the current timestamp.
func (c *MetadataCache) Set(url string, metadata Metadata) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[url] = &metadataEntry{
		Metadata:  metadata,
		FetchedAt: time.Now(),
	}
}

// Delete removes a metadata entry for a URL.
func (c *MetadataCache) Delete(url string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, url)
}

// CacheStats holds statistics about cache operations.
type CacheStats struct {
	Hits    uint64        // Number of cache hits (non-expired entries)
	Misses  uint64        // Number of cache misses (entries not found)
	Expired uint64        // Number of expired entries found
	Size    int           // Number of entries currently in cache
	TTL     time.Duration // Current TTL setting
}

// Stats returns current cache statistics.
func (c *MetadataCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return CacheStats{
		Hits:    c.hits,
		Misses:  c.misses,
		Expired: c.expired,
		Size:    len(c.entries),
		TTL:     c.ttl,
	}
}

// Clear removes all entries from the cache and resets statistics.
func (c *MetadataCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*metadataEntry)
	c.hits = 0
	c.misses = 0
	c.expired = 0
}

// SetTTL updates the TTL for the cache.
// Existing entries are not affected by this change.
func (c *MetadataCache) SetTTL(ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ttl = ttl
}

// GetTTL returns the current TTL setting.
func (c *MetadataCache) GetTTL() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.ttl
}

// cleanupExpired removes all expired entries from the cache.
// This is useful for periodic maintenance to prevent memory growth.
func (c *MetadataCache) cleanupExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for url, entry := range c.entries {
		if now.Sub(entry.FetchedAt) >= c.ttl {
			delete(c.entries, url)
		}
	}
}
