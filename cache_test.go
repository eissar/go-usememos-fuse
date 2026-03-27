package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCacheManager_Get_NotFound(t *testing.T) {
	c := NewCacheManager(time.Minute, 1024*1024)

	// Try to get non-existent entry
	data, found := c.Get("http://example.com/file", 0, 100)
	if found {
		t.Error("Expected cache miss")
	}
	if data != nil {
		t.Error("Expected nil data on cache miss")
	}
}

func TestCacheManager_FetchWithCache(t *testing.T) {
	// Create test server
	content := "Hello, World!"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Support Range requests
		rangeHeader := r.Header.Get("Range")
		if rangeHeader != "" {
			w.Header().Set("Content-Range", "bytes 0-12/13")
			w.WriteHeader(http.StatusPartialContent)
		}
		w.Write([]byte(content))
	}))
	defer server.Close()

	c := NewCacheManager(time.Minute, 1024*1024)
	client := &http.Client{Timeout: 10 * time.Second}

	ctx := context.Background()

	// First fetch - cache miss
	data, err := c.FetchWithCache(ctx, client, server.URL, 0, len(content))
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if string(data) != content {
		t.Errorf("Expected %q, got %q", content, string(data))
	}

	// Check cache stats
	entries, size := c.Stats()
	if entries != 1 {
		t.Errorf("Expected 1 entry, got %d", entries)
	}
	if size != int64(len(content)) {
		t.Errorf("Expected size %d, got %d", len(content), size)
	}

	// Second fetch - should hit cache
	data2, err := c.FetchWithCache(ctx, client, server.URL, 0, len(content))
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if string(data2) != content {
		t.Errorf("Expected %q, got %q", content, string(data2))
	}
}

func TestCacheManager_Invalidate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("test data"))
	}))
	defer server.Close()

	c := NewCacheManager(time.Minute, 1024*1024)
	client := &http.Client{Timeout: 10 * time.Second}

	ctx := context.Background()
	c.FetchWithCache(ctx, client, server.URL, 0, 9)

	// Verify entry exists
	entries, _ := c.Stats()
	if entries != 1 {
		t.Fatalf("Expected 1 entry, got %d", entries)
	}

	// Invalidate
	c.Invalidate(server.URL)

	// Verify entry removed
	entries, _ = c.Stats()
	if entries != 0 {
		t.Errorf("Expected 0 entries after invalidate, got %d", entries)
	}
}

func TestCacheManager_MaxSize(t *testing.T) {
	// Test that cache respects max size
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("small"))
	}))
	defer server.Close()

	// Very small cache
	c := NewCacheManager(time.Hour, 10)
	client := &http.Client{Timeout: 10 * time.Second}
	ctx := context.Background()

	// Add entries that exceed limit - should trigger eviction
	c.FetchWithCache(ctx, client, server.URL+"/1", 0, 5)
	c.FetchWithCache(ctx, client, server.URL+"/2", 0, 5)
	c.FetchWithCache(ctx, client, server.URL+"/3", 0, 5)

	// After evictions, size should be limited
	_, size := c.Stats()
	if size > 10 {
		t.Errorf("Cache size %d exceeds max 10", size)
	}
}

func TestCacheManager_Expire(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("expire test"))
	}))
	defer server.Close()

	// Very short TTL
	c := NewCacheManager(1, 1024*1024)
	client := &http.Client{Timeout: 10 * time.Second}
	ctx := context.Background()

	c.FetchWithCache(ctx, client, server.URL, 0, 11)
	
	// Wait for expiration
	time.Sleep(2 * time.Millisecond)

	// Should be expired
	data, found := c.Get(server.URL, 0, 11)
	if found {
		t.Error("Expected expired entry to be treated as not found")
	}
	if data != nil {
		t.Error("Expected nil data for expired entry")
	}
}

// TestCacheManager_PartialHit tests that when a small file is cached (e.g., 2074 bytes)
// and the user requests a larger size (e.g., 4096 bytes), the cache returns the
// available data instead of treating it as a cache miss.
func TestCacheManager_PartialHit(t *testing.T) {
	// Create a server that returns 2074 bytes regardless of range request
	content := make([]byte, 2074)
	for i := range content {
		content[i] = byte('A' + byte(i%26))
	}
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", "bytes 0-2073/2074")
		w.WriteHeader(http.StatusPartialContent)
		w.Write(content)
	}))
	defer server.Close()

	c := NewCacheManager(time.Minute, 1024*1024)
	client := &http.Client{Timeout: 10 * time.Second}
	ctx := context.Background()

	// First fetch: request 65536 bytes, server returns 2074 (the whole file)
	data, err := c.FetchWithCache(ctx, client, server.URL, 0, 65536)
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if len(data) != 2074 {
		t.Errorf("Expected 2074 bytes from fetch, got %d", len(data))
	}
	
	// Cache stats
	entries, size := c.Stats()
	if entries != 1 {
		t.Errorf("Expected 1 cache entry, got %d", entries)
	}
	if size != 2074 { // 2074 bytes + overhead
		t.Errorf("Expected cache size 2074, got %d", size)
	}

	// BUG FIX: Requesting 4096 bytes at offset 0 should return a partial cache hit
	// with the 2074 bytes that are cached, instead of treating it as a cache miss
	data2, found := c.Get(server.URL, 0, 4096)
	if !found {
		t.Error("BUG: Expected partial cache hit, got cache miss. The cached entry should be returned even if smaller than requested.")
	}
	if len(data2) != 2074 {
		t.Errorf("Expected 2074 bytes from cache (partial hit), got %d", len(data2))
	}
	
	// Verify the data content is correct
	for i, b := range data2 {
		if b != content[i] {
			t.Errorf("Data mismatch at index %d, expected %d, got %d", i, content[i], b)
			break
		}
	}
}
