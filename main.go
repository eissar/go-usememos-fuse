// Copyright 2016 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This program is the analogon of libfuse's hello.c, a a program that
// exposes a single file "file.txt" in the root directory.
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

type HelloRoot struct {
	fs.Inode
	fs.FileGetattrer
}

// OnAdd populate inodes
func (r *HelloRoot) OnAdd(ctx context.Context) {
	ch := r.NewPersistentInode(
		ctx, &fs.MemRegularFile{
			Data: []byte("file.txt"),
			Attr: fuse.Attr{
				Mode: 0o644,
			},
		}, fs.StableAttr{Ino: 2})
	r.AddChild("file.txt", ch, false)
	remote := &RemoteFile{
		url:    "https://gist.githubusercontent.com/eissar/493f49146f50c723056700780ffc70e8/raw/6f74ca90e256a0647141b8454c51436cdd7532a7/t.sh",
		client: &http.Client{Timeout: 10 * time.Second},
	}
	remoteInode := r.NewPersistentInode(ctx, remote, fs.StableAttr{Ino: 3})
	r.AddChild("remote.bin", remoteInode, false)
}

func (r *HelloRoot) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = 0o755
	return 0
}

func main() {
	debug := flag.Bool("debug", false, "print debug data")
	flag.Parse()

	if len(flag.Args()) < 1 {
		log.Fatal("Usage:\n  hello MOUNTPOINT")
	}

	opts := &fs.Options{}
	opts.Debug = *debug

	server, err := fs.Mount(flag.Arg(0), &HelloRoot{}, opts)
	if err != nil {
		log.Fatalf("Mount fail: %v\n", err)
	}
	server.Wait()
}
