package main

import (
	"sync"
	"testing"
	"time"
)

func TestMetadataCache_Basic(t *testing.T) {
	cache := NewMetadataCache(30 * time.Second)

	// Test Get on empty cache - should be a miss
	meta, found, expired := cache.Get("http://example.com/file1")
	if found {
		t.Error("Expected cache miss for non-existent entry")
	}
	if expired {
		t.Error("Expired should be false when entry not found")
	}

	// Set some metadata
	testMeta := Metadata{
		Size:    1024,
		ModTime: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		ETag:    `"abc123"`,
	}
	cache.Set("http://example.com/file1", testMeta)

	// Get should now return the metadata
	meta, found, expired = cache.Get("http://example.com/file1")
	if !found {
		t.Error("Expected cache hit after Set")
	}
	if expired {
		t.Error("Expected non-expired entry immediately after Set")
	}
	if meta.Size != testMeta.Size {
		t.Errorf("Expected Size %d, got %d", testMeta.Size, meta.Size)
	}
	if !meta.ModTime.Equal(testMeta.ModTime) {
		t.Errorf("Expected ModTime %v, got %v", testMeta.ModTime, meta.ModTime)
	}
	if meta.ETag != testMeta.ETag {
		t.Errorf("Expected ETag %q, got %q", testMeta.ETag, meta.ETag)
	}

	// Stats should show 1 hit and 1 miss
	stats := cache.Stats()
	if stats.Hits != 1 {
		t.Errorf("Expected 1 hit, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.Misses)
	}
	if stats.Size != 1 {
		t.Errorf("Expected 1 entry, got %d", stats.Size)
	}
}

func TestMetadataCache_TTLExpiration(t *testing.T) {
	// Use a very short TTL for testing
	cache := NewMetadataCache(50 * time.Millisecond)

	testMeta := Metadata{
		Size:    2048,
		ModTime: time.Now(),
		ETag:    `"expire-test"`,
	}
	cache.Set("http://example.com/file2", testMeta)

	// Should be found and not expired immediately
	_, found, expired := cache.Get("http://example.com/file2")
	if !found {
		t.Error("Expected cache hit immediately after Set")
	}
	if expired {
		t.Error("Expected non-expired entry immediately after Set")
	}

	// Wait for TTL to expire
	time.Sleep(100 * time.Millisecond)

	// Should now be found but expired
	_, found, expired = cache.Get("http://example.com/file2")
	if !found {
		t.Error("Expected entry to still be found (just expired)")
	}
	if !expired {
		t.Error("Expected entry to be expired after TTL")
	}

	// Stats should show 1 expired
	stats := cache.Stats()
	if stats.Expired != 1 {
		t.Errorf("Expected 1 expired, got %d", stats.Expired)
	}
}

func TestMetadataCache_Stats(t *testing.T) {
	cache := NewMetadataCache(10 * time.Second)

	// Verify initial stats
	stats := cache.Stats()
	if stats.Hits != 0 {
		t.Errorf("Expected 0 initial hits, got %d", stats.Hits)
	}
	if stats.Misses != 0 {
		t.Errorf("Expected 0 initial misses, got %d", stats.Misses)
	}
	if stats.Expired != 0 {
		t.Errorf("Expected 0 initial expired, got %d", stats.Expired)
	}
	if stats.Size != 0 {
		t.Errorf("Expected 0 initial entries, got %d", stats.Size)
	}
	if stats.TTL != 10*time.Second {
		t.Errorf("Expected TTL %v, got %v", 10*time.Second, stats.TTL)
	}

	// Add multiple entries
	for i := 0; i < 5; i++ {
		cache.Set("http://example.com/file"+string(rune('A'+i)), Metadata{Size: int64(i)})
	}

	// Generate hits and misses
	cache.Get("http://example.com/fileA") // hit
	cache.Get("http://example.com/fileA") // hit
	cache.Get("http://example.com/fileB") // hit
	cache.Get("http://example.com/fileZ") // miss
	cache.Get("http://example.com/fileY") // miss

	stats = cache.Stats()
	if stats.Hits != 3 {
		t.Errorf("Expected 3 hits, got %d", stats.Hits)
	}
	if stats.Misses != 2 {
		t.Errorf("Expected 2 misses, got %d", stats.Misses)
	}
	if stats.Size != 5 {
		t.Errorf("Expected 5 entries, got %d", stats.Size)
	}
}

func TestMetadataCache_Delete(t *testing.T) {
	cache := NewMetadataCache(10 * time.Second)

	testMeta := Metadata{Size: 1024, ETag: `"delete-me"`}
	cache.Set("http://example.com/to-delete", testMeta)

	// Verify it exists
	_, found, _ := cache.Get("http://example.com/to-delete")
	if !found {
		t.Fatal("Expected entry to exist before deletion")
	}

	// Delete it
	cache.Delete("http://example.com/to-delete")

	// Verify it's gone
	_, found, _ = cache.Get("http://example.com/to-delete")
	if found {
		t.Error("Expected entry to be deleted")
	}

	// Stats should show 1 miss from the second Get
	stats := cache.Stats()
	if stats.Misses != 1 {
		t.Errorf("Expected 1 miss after delete, got %d", stats.Misses)
	}
}

func TestMetadataCache_Clear(t *testing.T) {
	cache := NewMetadataCache(10 * time.Second)

	// Add entries and generate stats
	cache.Set("http://example.com/file1", Metadata{Size: 100})
	cache.Set("http://example.com/file2", Metadata{Size: 200})
	cache.Get("http://example.com/file1") // hit
	cache.Get("http://example.com/file3") // miss

	// Verify entries exist
	stats := cache.Stats()
	if stats.Size != 2 {
		t.Fatalf("Expected 2 entries before clear, got %d", stats.Size)
	}

	// Clear the cache
	cache.Clear()

	// Verify everything is reset
	stats = cache.Stats()
	if stats.Hits != 0 {
		t.Errorf("Expected 0 hits after clear, got %d", stats.Hits)
	}
	if stats.Misses != 0 {
		t.Errorf("Expected 0 misses after clear, got %d", stats.Misses)
	}
	if stats.Expired != 0 {
		t.Errorf("Expected 0 expired after clear, got %d", stats.Expired)
	}
	if stats.Size != 0 {
		t.Errorf("Expected 0 entries after clear, got %d", stats.Size)
	}

	// Verify entries are actually gone
	_, found, _ := cache.Get("http://example.com/file1")
	if found {
		t.Error("Expected file1 to be cleared")
	}
}

func TestMetadataCache_SetAndGetTTL(t *testing.T) {
	cache := NewMetadataCache(10 * time.Second)

	// Verify initial TTL
	if ttl := cache.GetTTL(); ttl != 10*time.Second {
		t.Errorf("Expected TTL 10s, got %v", ttl)
	}

	// Change TTL
	cache.SetTTL(5 * time.Minute)

	// Verify new TTL
	if ttl := cache.GetTTL(); ttl != 5*time.Minute {
		t.Errorf("Expected TTL 5m, got %v", ttl)
	}

	// Stats should reflect new TTL
	stats := cache.Stats()
	if stats.TTL != 5*time.Minute {
		t.Errorf("Expected stats TTL 5m, got %v", stats.TTL)
	}
}

func TestMetadataCache_UpdateExisting(t *testing.T) {
	cache := NewMetadataCache(10 * time.Second)

	// Set initial metadata
	cache.Set("http://example.com/file", Metadata{Size: 100, ETag: `"v1"`})

	meta, found, _ := cache.Get("http://example.com/file")
	if !found {
		t.Fatal("Expected to find entry")
	}
	if meta.Size != 100 {
		t.Errorf("Expected Size 100, got %d", meta.Size)
	}
	if meta.ETag != `"v1"` {
		t.Errorf("Expected ETag v1, got %s", meta.ETag)
	}

	// Update with new metadata
	cache.Set("http://example.com/file", Metadata{Size: 200, ETag: `"v2"`})

	meta, found, _ = cache.Get("http://example.com/file")
	if !found {
		t.Fatal("Expected to find updated entry")
	}
	if meta.Size != 200 {
		t.Errorf("Expected updated Size 200, got %d", meta.Size)
	}
	if meta.ETag != `"v2"` {
		t.Errorf("Expected updated ETag v2, got %s", meta.ETag)
	}

	// Should still only have 1 entry
	stats := cache.Stats()
	if stats.Size != 1 {
		t.Errorf("Expected 1 entry, got %d", stats.Size)
	}
}

func TestMetadataCache_ConcurrentAccess(t *testing.T) {
	cache := NewMetadataCache(1 * time.Minute)

	var wg sync.WaitGroup
	numGoroutines := 50
	numOperations := 50

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				cache.Set("http://example.com/file", Metadata{Size: int64(id*numOperations + j)})
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				cache.Get("http://example.com/file")
				cache.Get("http://example.com/nonexistent")
			}
		}(i)
	}

	// Concurrent stats reads
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				_ = cache.Stats()
			}
		}()
	}

	wg.Wait()

	// Verify final state is consistent
	stats := cache.Stats()
	if stats.Size != 1 {
		t.Errorf("Expected 1 entry, got %d", stats.Size)
	}
}

func TestMetadataCache_ZeroTTL(t *testing.T) {
	cache := NewMetadataCache(0)

	cache.Set("http://example.com/file", Metadata{Size: 100})

	// With zero TTL, entry should immediately be expired
	_, found, expired := cache.Get("http://example.com/file")
	if !found {
		t.Error("Expected entry to be found")
	}
	if !expired {
		t.Error("Expected entry with zero TTL to be expired")
	}
}

func TestMetadataCache_MultipleURLs(t *testing.T) {
	cache := NewMetadataCache(10 * time.Second)

	urls := []string{
		"http://example.com/file1",
		"http://example.com/file2",
		"http://example.com/file3",
		"http://example.com/path/to/file4",
	}

	// Set metadata for each URL
	for i, url := range urls {
		cache.Set(url, Metadata{
			Size:    int64(100 * (i + 1)),
			ModTime: time.Now().Add(time.Duration(i) * time.Hour),
			ETag:    `"etag-` + string(rune('A'+i)) + `"`,
		})
	}

	// Verify all entries exist and have correct data
	for i, url := range urls {
		meta, found, expired := cache.Get(url)
		if !found {
			t.Errorf("Expected to find entry for %s", url)
			continue
		}
		if expired {
			t.Errorf("Expected entry for %s to not be expired", url)
		}
		if meta.Size != int64(100*(i+1)) {
			t.Errorf("Expected Size %d for %s, got %d", 100*(i+1), url, meta.Size)
		}
	}

	// Verify stats
	stats := cache.Stats()
	if stats.Size != len(urls) {
		t.Errorf("Expected %d entries, got %d", len(urls), stats.Size)
	}
	if stats.Hits != uint64(len(urls)) {
		t.Errorf("Expected %d hits, got %d", len(urls), stats.Hits)
	}
}
