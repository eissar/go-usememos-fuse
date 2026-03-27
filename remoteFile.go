package main

import (
	"context"
	"net/http"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// RemoteFile implements a read‑only file whose contents are fetched from a
// remote HTTP URL. It embeds fs.Inode and uses caching for performance.
type RemoteFile struct {
	fs.Inode
	url         string
	client      *http.Client
	cache       *CacheManager
	inode       uint64
	contentSize uint64 // known size, 0 if unknown
}

// Ensure RemoteFile satisfies the needed interfaces.
var (
	_ fs.NodeGetattrer = (*RemoteFile)(nil)
	_ fs.NodeOpener      = (*RemoteFile)(nil)
	_ fs.FileReader      = (*RemoteFile)(nil)
)

// NewRemoteFile creates a new RemoteFile with proper initialization
func NewRemoteFile(url string, client *http.Client, cache *CacheManager, inode uint64) *RemoteFile {
	return &RemoteFile{
		url:     url,
		client:  client,
		cache:   cache,
		inode:   inode,
	}
}

// Getattr reports file attributes.
func (r *RemoteFile) Getattr(ctx context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Attr.Mode = 0o444 // read‑only regular file
	out.Attr.Size = r.contentSize
	out.Attr.Ino = r.inode
	return 0
}

// Open returns a file handle.
func (r *RemoteFile) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	// Disallow write flags.
	if flags&uint32(syscall.O_WRONLY|syscall.O_RDWR) != 0 {
		return nil, 0, syscall.EPERM
	}
	return nil, fuse.FOPEN_KEEP_CACHE, 0
}

// Read fetches the requested range via HTTP Range requests with caching.
func (r *RemoteFile) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	size := len(dest)
	if size == 0 {
		return fuse.ReadResultData(nil), 0
	}

	// Fetch with caching
	data, err := r.cache.FetchWithCache(ctx, r.client, r.url, off, size)
	if err != nil {
		return nil, syscall.EIO
	}

	// Copy data to destination buffer
	if len(data) > size {
		data = data[:size]
	}
	copy(dest, data)

	return fuse.ReadResultData(data), 0
}

// SetSize updates the known content size (from HEAD request or similar)
func (r *RemoteFile) SetSize(size uint64) {
	r.contentSize = size
}

// GetCacheStats returns statistics about the cache for this URL
func (r *RemoteFile) GetCacheStats() (entries int, size int64) {
	return r.cache.Stats()
}
