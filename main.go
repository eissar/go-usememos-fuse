// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This program exposes memos from a remote Memos server via gRPC
// as a FUSE filesystem.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
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
	client       *MemoClient
	memoInfos    []MemoInfo
}

// Ensure FileSystemRoot implements necessary interfaces
var _ fs.NodeGetattrer = (*FileSystemRoot)(nil)

// NewFileSystemRoot creates a new filesystem root with initialized managers
func NewFileSystemRoot(client *MemoClient) (*FileSystemRoot, error) {
	return &FileSystemRoot{
		inodeManager: NewInodeManager(2), // Start from 2 (1 is reserved for root)
		client:       client,
	}, nil
}

// OnAdd populates the filesystem with memos from the MemoClient
func (r *FileSystemRoot) OnAdd(ctx context.Context) {
	// List memos from the server
	memos, err := r.client.ListMemos(ctx, nil)
	if err != nil {
		log.Printf("Warning: Failed to list memos: %v", err)
		// Create a placeholder file with error message
		fileIno := r.inodeManager.Allocate()
		ch := r.NewPersistentInode(
			ctx,
			&fs.MemRegularFile{
				Data: []byte(fmt.Sprintf("Error connecting to memos server: %v\n", err)),
				Attr: fuse.Attr{
					Mode: 0o644,
				},
			},
			fs.StableAttr{Ino: fileIno})
		r.AddChild("error.txt", ch, false)
		log.Printf("Created error.txt with inode %d", fileIno)
		return
	}

	r.memoInfos = memos

	// Create files for each memo
	for _, memo := range memos {
		fileIno := r.inodeManager.Allocate()
		
		// Create a file with memo content
		ch := r.NewPersistentInode(
			ctx,
			&fs.MemRegularFile{
				Data: []byte(memo.Content),
				Attr: fuse.Attr{
					Mode: 0o644,
				},
			},
			fs.StableAttr{Ino: fileIno})
		
		// Use memo ID or name as filename (replace slashes to avoid subdirectory creation)
		filename := fmt.Sprintf("memo-%s.txt", strings.ReplaceAll(memo.Name, "/", "-"))
		r.AddChild(filename, ch, false)
		log.Printf("Created %s with inode %d", filename, fileIno)
	}

	log.Printf("Filesystem initialized: %d memos mounted as files", len(memos))
}

// Getattr returns attributes for the root directory
func (r *FileSystemRoot) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = 0o755
	return 0
}

// Stats returns current filesystem statistics
func (r *FileSystemRoot) Stats() (inodeCount int, memoCount int) {
	return r.inodeManager.Count(), len(r.memoInfos)
}

func main() {
	debug := flag.Bool("debug", false, "print debug data")
	serverURL := flag.String("server", "", "Memos server URL (e.g., http://localhost:5230)")
	accessToken := flag.String("token", "", "Access token for authentication (optional)")
	flag.Parse()

	if len(flag.Args()) < 1 {
		log.Fatal("Usage:\n  memos-fuse [flags] MOUNTPOINT")
	}

	if *serverURL == "" {
		log.Fatal("Server URL is required. Use -server flag to specify memos server URL.")
	}

	// Create MemoClient
	config := ClientConfig{
		BaseURL:     *serverURL,
		AccessToken: *accessToken,
		HTTPTimeout: 30 * time.Second,
	}
	
	client, err := NewMemoClient(config)
	if err != nil {
		log.Fatalf("Failed to create MemoClient: %v", err)
	}
	defer client.Close()

	log.Printf("Connected to memos server at %s", *serverURL)

	// Create filesystem root
	root, err := NewFileSystemRoot(client)
	if err != nil {
		log.Fatalf("Failed to create filesystem root: %v", err)
	}

	opts := &fs.Options{}
	opts.Debug = *debug

	server, err := fs.Mount(flag.Arg(0), root, opts)
	if err != nil {
		log.Fatalf("Mount fail: %v\n", err)
	}

	log.Printf("Mounted memos filesystem at %s", flag.Arg(0))

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
