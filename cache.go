package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// CacheEntry represents a cached file segment
type CacheEntry struct {
	Data      []byte
	Offset    int64
	FetchedAt time.Time
}

// CacheManager handles in-memory caching of remote file contents
type CacheManager struct {
	mu          sync.RWMutex
	entries     map[string][]CacheEntry // key: URL, value: cached segments
	maxAge      time.Duration
	maxSize     int64
	currentSize int64
}

// NewCacheManager creates a new cache manager with specified limits
func NewCacheManager(maxAge time.Duration, maxSize int64) *CacheManager {
	return &CacheManager{
		entries: make(map[string][]CacheEntry),
		maxAge:  maxAge,
		maxSize: maxSize,
	}
}

// Get retrieves cached data for a URL at the given offset and size.
// Returns the data and true if found, nil and false otherwise.
func (c *CacheManager) Get(url string, offset int64, size int) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entries, exists := c.entries[url]
	if !exists {
		return nil, false
	}

	for _, entry := range entries {
		if entry.Offset <= offset && entry.Offset+int64(len(entry.Data)) >= offset+int64(size) {
			// Check if entry is still fresh
			if time.Since(entry.FetchedAt) < c.maxAge {
				start := offset - entry.Offset
				end := start + int64(size)
				if end > int64(len(entry.Data)) {
					end = int64(len(entry.Data))
				}
				return entry.Data[start:end], true
			}
		}
	}
	return nil, false
}

// FetchWithCache fetches data with caching support.
// Uses HTTP Range requests and caches the result.
func (c *CacheManager) FetchWithCache(ctx context.Context, client *http.Client, url string, offset int64, size int) ([]byte, error) {
	// Try cache first
	if data, found := c.Get(url, offset, size); found {
		return data, nil
	}

	// Calculate range request
	end := offset + int64(size) - 1
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, end))

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading body: %w", err)
	}

	// Check cache size limits before storing
	c.mu.Lock()
	if int64(len(data)) > c.maxSize {
		// Data too large to cache, return without caching
		c.mu.Unlock()
		return data, nil
	}

	// Simple eviction: if adding this would exceed max size, clear old entries
	if c.currentSize+int64(len(data)) > c.maxSize {
		c.entries = make(map[string][]CacheEntry) // Simple clear-all eviction
		c.currentSize = 0
	}

	c.entries[url] = append(c.entries[url], CacheEntry{
		Data:      data,
		Offset:    offset,
		FetchedAt: time.Now(),
	})
	c.currentSize += int64(len(data))
	c.mu.Unlock()

	return data, nil
}

// Invalidate removes all cached entries for a URL
func (c *CacheManager) Invalidate(url string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entries, exists := c.entries[url]; exists {
		for _, e := range entries {
			c.currentSize -= int64(len(e.Data))
		}
		delete(c.entries, url)
	}
}

// Stats returns cache statistics
func (c *CacheManager) Stats() (entries int, size int64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries), c.currentSize
}
