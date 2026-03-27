# Caching Architecture

This document describes the caching strategy used in `go-usememos-fuse`, a FUSE filesystem that exposes remote HTTP files with intelligent caching.

> **GitHub Issue:** [#5](https://github.com/eissar/go-usememos-fuse/issues/5)

## Overview

The filesystem implements a **two-tier approach** to caching:

1. **File Content Cache** - In-memory caching of remote file data with TTL and size limits
2. **No Metadata Cache** - File attributes (size, permissions, inode) are always served from memory, not cached from remote

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         FUSE Filesystem Layer                            │
│                                                                          │
│   ┌─────────────┐      ┌─────────────┐      ┌─────────────────────┐     │
│   │   getattr   │      │    open     │      │        read         │     │
│   │   (sync)    │      │   (sync)    │      │   (cached/async)    │     │
│   └──────┬──────┘      └──────┬──────┘      └──────────┬──────────┘     │
│          │                    │                        │                │
│          ▼                    ▼                        ▼                │
│   ┌─────────────┐      ┌─────────────┐      ┌─────────────────────┐     │
│   │   Inode     │      │   Access    │      │   CacheManager      │     │
│   │   Manager   │      │   Control   │      │   ┌─────────────┐   │     │
│   │  (inodes)   │      │  (perms)    │      │   │  In-Memory  │   │     │
│   └─────────────┘      └─────────────┘      │   │   Segments  │   │     │
│                                             │   └─────────────┘   │     │
│                                             │                     │     │
│                                             │   TTL: 5 minutes    │     │
│                                             │   Max: 64MB         │     │
│                                             └─────────────────────┘     │
│                                                    │                     │
└────────────────────────────────────────────────────┼─────────────────────┘
                                                     │
                              ┌──────────────────────┼──────────────────────┐
                              │                      ▼                      │
                              │        ┌─────────────────────┐              │
                              │        │   HTTP Range GET    │              │
                              │        │   (fallback cache)  │              │
                              │        └─────────────────────┘              │
                              │                      │                      │
                              ▼                      ▼                      ▼
                       ┌─────────────┐      ┌─────────────┐        ┌─────────────┐
                       │   Cache     │      │   Cache     │        │   Remote    │
                       │    Hit      │      │    Miss     │        │   Server    │
                       │  (fast)     │      │  (network)  │        │  (source)   │
                       └─────────────┘      └─────────────┘        └─────────────┘
```

## Flow Diagram: Metadata vs Content

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         Request Flow                                     │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌──────────────────┐         ┌──────────────────┐                      │
│  │  ls -la /mnt/    │         │  cat /mnt/file   │                      │
│  └────────┬─────────┘         └────────┬─────────┘                      │
│           │                            │                                │
│           ▼                            ▼                                │
│  ┌──────────────────┐         ┌──────────────────┐                      │
│  │   gettattr()     │         │     read()       │                      │
│  │   ───────────    │         │   ───────────    │                      │
│  │   InodeManager   │         │   CacheManager   │                      │
│  │   (in-memory)    │         │   (with TTL)     │                      │
│  │                  │         │                  │                      │
│  │   ✓ Instant      │         │   Check cache    │                      │
│  │   ✓ No network   │         │        │         │                      │
│  │   ✓ Always fresh │         │    ┌───┴───┐     │                      │
│  │                  │         │    ▼       ▼     │                      │
│  │   METADATA       │         │   Hit     Miss   │                      │
│  │   NOT CACHED     │         │    │       │     │                      │
│  │   (local source) │         │    ▼       ▼     │                      │
│  │                  │         │  Return  HTTP    │                      │
│  └──────────────────┘         │  data    Range   │                      │
│                               │           │      │                      │
│                               │           ▼      │                      │
│                               │        Store     │                      │
│                               │        Return    │                      │
│                               │                  │                      │
│                               │  CONTENT CACHED  │                      │
│                               │  (with network)  │                      │
│                               └──────────────────┘                      │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Design Decisions

### Why Content IS Cached

File content is cached for several performance reasons:

| Reason | Explanation |
|--------|-------------|
| **Network Latency** | Remote HTTP requests can take 50-500ms. Cache hits are <1ms. |
| **Sequential Reads** | Applications often read files in chunks; caching prevents repeated round-trips. |
| **Random Access** | The cache supports partial hits for different file offsets. |
| **Bandwidth Conservation** | Reduces repeated downloads of the same data segments. |

### Why Metadata is NOT Cached

File metadata (inode numbers, file sizes, permissions) is **not cached from remote** because it is managed locally:

1. **Source of Truth**: Metadata is computed locally by the FUSE filesystem, not fetched from remote
2. **Consistency**: Inode numbers must remain stable for the filesystem session
3. **Simplicity**: No cache invalidation needed for directory listings or file attributes
4. **Low Cost**: Metadata is lightweight and stored in memory anyway

```go
// From remoteFile.go - attributes are set synchronously, no cache lookup
func (r *RemoteFile) Getattr(ctx context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
    out.Attr.Mode = 0o444    // read-only (computed)
    out.Attr.Size = r.contentSize  // known size (from memory)
    out.Attr.Ino = r.inode   // stable inode (from InodeManager)
    return 0
}
```

---

## Cache Configuration

### Default Parameters

```go
// From main.go
NewCacheManager(5*time.Minute, 64*1024*1024)  // 5 min TTL, 64MB max
```

| Parameter | Default Value | Rationale |
|-----------|---------------|-----------|
| **TTL** | 5 minutes | Balance between freshness and performance. Long enough for batch operations, short enough to notice remote changes. |
| **Max Size** | 64 MB | Prevents unbounded memory growth while allowing multiple files to be cached. |

### TTL Rationale

The 5-minute TTL was chosen based on these trade-offs:

```
┌────────────────────────────────────────────────────────────────┐
│                      TTL Trade-offs                             │
├────────────────────────────────────────────────────────────────┤
│  Too Short (< 30s)   │  Too Long (> 10min)                     │
│  • Excessive network │  • Stale content                        │
│    traffic           │  • Users see outdated files             │
│  • Poor performance  │  • Memory pressure                      │
│  • Battery drain     │  • Confusion when remote changes        │
├────────────────────────────────────────────────────────────────┤
│  Sweet Spot: 5 minutes                                          │
│  • Handles typical editor/viewer sessions                       │
│  • Allows batch operations (grep, copy, backup)                 │
│  • Reasonable freshness guarantee                               │
└────────────────────────────────────────────────────────────────┘
```

---

## Cache Architecture

### Segment-Based Storage

The cache stores file data in **segments** keyed by URL:

```go
// From cache.go
type CacheEntry struct {
    Data      []byte     // Raw file content
    Offset    int64      // Byte offset in file
    FetchedAt time.Time  // For TTL validation
}

type CacheManager struct {
    entries     map[string][]CacheEntry  // URL → segments
    maxAge      time.Duration
    maxSize     int64
    currentSize int64
}
```

### Partial Cache Hits

The cache intelligently handles partial overlaps:

```
File: [0---------------------------------10240]
                       ▲
Request: offset=5120, size=4096
                       │
Cache State:           │
┌──────────┐  ┌────────┴──────┐  ┌──────────┐
│ 0-2047   │  │  4096-8191    │  │ 8192+    │
│ (cached) │  │   (cached)    │  │ (miss)   │
└──────────┘  └────────┬──────┘  └──────────┘
                       │
Result: Partial hit - returns bytes 5120-6143 from cache segment
```

### Eviction Policy

When the cache exceeds `maxSize`, a **clear-all** eviction is performed:

```go
// Simple but effective for typical workloads
if c.currentSize+int64(len(data)) > c.maxSize {
    c.entries = make(map[string][]CacheEntry)  // Clear all
    c.currentSize = 0
}
```

This approach works well because:
- Typical usage patterns involve working with a few files at a time
- 64MB is sufficient for most text/config files
- Eviction is rare in practice for the target use case

---

## How `remote.bin` Behaves

### First Access (Cold Cache)

```
User: cat /mnt/remote.bin

1. FUSE receives read request: offset=0, size=4096
2. Cache miss - entry doesn't exist
3. HTTP Range request: GET /file Range: bytes=0-4095
4. Response received, stored in cache
5. Data returned to user

Time: ~50-300ms (network dependent)
```

### Subsequent Access (Warm Cache)

```
User: cat /mnt/remote.bin  (within 5 minutes)

1. FUSE receives read request: offset=0, size=4096
2. Cache hit - entry exists and is fresh
3. Data returned directly from memory

Time: <1ms
```

### After TTL Expiration

```
User: cat /mnt/remote.bin  (after 5+ minutes)

1. FUSE receives read request
2. Cache entry exists but is stale (time.Since(entry.FetchedAt) > 5min)
3. Treated as cache miss
4. Fresh HTTP request made
5. New data stored in cache

Time: ~50-300ms (same as cold cache)
```

### Large File Behavior

For files larger than the cache size limit:

```
File size: 100 MB
Cache max: 64 MB

Behavior:
1. First 64MB read → Cached
2. Remaining 36MB → NOT cached (exceeds limit check)
3. Subsequent reads of first 64MB → Cache hit
4. Subsequent reads of remaining 36MB → Always network fetch
```

---

## Cache Statistics

Monitor cache performance via the filesystem root:

```go
// From main.go - accessible in Stats()
func (r *FileSystemRoot) Stats() (inodeCount int, cacheEntries int, cacheSize int64) {
    entries, size := r.cache.Stats()
    return r.inodeManager.Count(), entries, size
}
```

The `GetCacheStats()` method on RemoteFile provides per-file visibility:

```go
entries, size := remoteFile.GetCacheStats()
fmt.Printf("Cache: %d entries, %d bytes\n", entries, size)
```

---

## Best Practices

### For Users

1. **Sequential Access**: Reading files sequentially maximizes cache efficiency
2. **Batch Operations**: Complete related work within the 5-minute TTL window
3. **File Size Awareness**: Files >64MB will have partial caching
4. **Freshness**: Use direct HTTP access if you need guaranteed fresh content

### For Developers

1. **Tuning TTL**: Adjust `NewCacheManager(ttl, maxSize)` for your use case
2. **Invalidation**: Use `cache.Invalidate(url)` to force refresh specific files
3. **Monitoring**: Log cache stats to optimize parameters
4. **Testing**: Use `cache_test.go` patterns for validation

---

## Future Improvements

Potential enhancements to consider:

- [ ] LRU eviction instead of clear-all
- [ ] Persistent disk cache for large files
- [ ] Configurable TTL per file type
- [ ] Cache warming/pre-fetch for sequential access
- [ ] Compression for cached segments

---

## References

- [cache.go](cache.go) - Cache implementation
- [remoteFile.go](remoteFile.go) - File node with caching
- [cache_test.go](cache_test.go) - Test cases and examples
