// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This program exposes files from a remote HTTP server with
// inode management and in-memory caching.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// FileSystemRoot is the root node of our filesystem.
// It manages inode allocation and caching for all nodes.
type FileSystemRoot struct {
	fs.Inode
	inodeManager *InodeManager
	cache        *CacheManager
	client       *http.Client
}

// Ensure FileSystemRoot implements necessary interfaces
var _ fs.NodeGetattrer = (*FileSystemRoot)(nil)

// NewFileSystemRoot creates a new filesystem root with initialized managers
func NewFileSystemRoot() *FileSystemRoot {
	return &FileSystemRoot{
		inodeManager: NewInodeManager(2), // Start from 2 (1 is reserved for root)
		cache:        NewCacheManager(5*time.Minute, 64*1024*1024), // 5 min TTL, 64MB max
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// OnAdd populates the filesystem with initial files
func (r *FileSystemRoot) OnAdd(ctx context.Context) {
	// Add a regular memfs file
	fileIno := r.inodeManager.Allocate()
	ch := r.NewPersistentInode(
		ctx,
		&fs.MemRegularFile{
			Data: []byte("Hello from FUSE with caching!\n"),
			Attr: fuse.Attr{
				Mode: 0o644,
			},
		},
		fs.StableAttr{Ino: fileIno})
	r.AddChild("hello.txt", ch, false)
	log.Printf("Created hello.txt with inode %d", fileIno)

	// Add remote file with caching
	remoteIno := r.inodeManager.Allocate()
	remoteURL := "https://gist.githubusercontent.com/eissar/493f49146f50c723056700780ffc70e8/raw/6f74ca90e256a0647141b8454c51436cdd7532a7/t.sh"
	remoteFile := NewRemoteFile(remoteURL, r.client, r.cache, remoteIno)
	remoteInode := r.NewPersistentInode(ctx, remoteFile, fs.StableAttr{Ino: remoteIno})
	r.AddChild("remote.bin", remoteInode, false)
	log.Printf("Created remote.bin with inode %d (caching enabled)", remoteIno)

	log.Printf("Filesystem initialized: %d inodes, cache: 5min TTL/64MB max",
		r.inodeManager.Count())
}

// Getattr returns attributes for the root directory
func (r *FileSystemRoot) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = 0o755
	return 0
}

// Stats returns current filesystem statistics
func (r *FileSystemRoot) Stats() (inodeCount int, cacheEntries int, cacheSize int64) {
	entries, size := r.cache.Stats()
	return r.inodeManager.Count(), entries, size
}

func main() {
	debug := flag.Bool("debug", false, "print debug data")
	flag.Parse()

	if len(flag.Args()) < 1 {
		log.Fatal("Usage:\n  hello MOUNTPOINT")
	}

	opts := &fs.Options{}
	opts.Debug = *debug

	root := NewFileSystemRoot()
	server, err := fs.Mount(flag.Arg(0), root, opts)
	if err != nil {
		log.Fatalf("Mount fail: %v\n", err)
	}

	log.Printf("Mounted filesystem at %s", flag.Arg(0))
	server.Wait()
}
