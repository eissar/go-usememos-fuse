// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This program exposes files from a remote HTTP server with
// inode management (no caching).
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// FileSystemRoot is the root node of our filesystem.
// It manages inode allocation for all nodes.
type FileSystemRoot struct {
	fs.Inode
	inodeManager *InodeManager
	client       *http.Client
}

// Ensure FileSystemRoot implements necessary interfaces
var _ fs.NodeGetattrer = (*FileSystemRoot)(nil)

// NewFileSystemRoot creates a new filesystem root with initialized managers
func NewFileSystemRoot() *FileSystemRoot {
	return &FileSystemRoot{
		inodeManager: NewInodeManager(2), // Start from 2 (1 is reserved for root)
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
			Data: []byte("Hello from FUSE!\n"),
			Attr: fuse.Attr{
				Mode: 0o644,
			},
		},
		fs.StableAttr{Ino: fileIno})
	r.AddChild("hello.txt", ch, false)
	log.Printf("Created hello.txt with inode %d", fileIno)

	// Add remote file (no caching, fetches from remote on every read)
	remoteIno := r.inodeManager.Allocate()
	remoteURL := "https://gist.githubusercontent.com/eissar/493f49146f50c723056700780ffc70e8/raw/6f74ca90e256a0647141b8454c51436cdd7532a7/t.sh"
	remoteFile := NewRemoteFile(remoteURL, r.client, remoteIno)
	remoteInode := r.NewPersistentInode(ctx, remoteFile, fs.StableAttr{Ino: remoteIno})
	r.AddChild("remote.bin", remoteInode, false)
	log.Printf("Created remote.bin with inode %d (no caching)", remoteIno)

	log.Printf("Filesystem initialized: %d inodes",
		r.inodeManager.Count())
}

// Getattr returns attributes for the root directory
func (r *FileSystemRoot) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = 0o755
	return 0
}

// Stats returns current filesystem statistics
func (r *FileSystemRoot) Stats() (inodeCount int) {
	return r.inodeManager.Count()
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

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("Received signal %v, unmounting gracefully...", sig)
		if err := server.Unmount(); err != nil {
			log.Printf("Unmount error: %v", err)
		}
	}()

	server.Wait()
	log.Printf("Filesystem unmounted cleanly")
}
