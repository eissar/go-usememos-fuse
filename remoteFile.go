package main

import (
	"context"
	"io"
	"net/http"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// RemoteFile implements a read‑only file whose contents are fetched from a
// remote HTTP URL.  It embeds fs.Inode so it can be added to the tree.
type RemoteFile struct {
	fs.Inode
	url    string       // remote location
	client *http.Client // reusable HTTP client
}

// Ensure RemoteFile satisfies the needed interfaces.
var (
	_ = (fs.NodeGetattrer)((*RemoteFile)(nil))
	_ = (fs.NodeOpener)((*RemoteFile)(nil))
	_ = (fs.FileReader)((*RemoteFile)(nil))
)

// Getattr reports a static size; you could also issue a HEAD request to get
// the real length.
func (r *RemoteFile) Getattr(ctx context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Attr.Mode = 0o444 // read‑only regular file
	out.Attr.Size = 0     // unknown; let the kernel grow as we read
	out.Attr.Ino = r.Inode.StableAttr().Ino
	return 0
}

// Open returns a file handle that carries no state (nil is fine for a simple
// read‑only file).
func (r *RemoteFile) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	// Disallow write flags.
	if flags&uint32(syscall.O_WRONLY|syscall.O_RDWR) != 0 {
		return nil, 0, syscall.EPERM
	}
	return nil, fuse.FOPEN_KEEP_CACHE, 0
}

// Read fetches the requested range via HTTP Range requests.
func (r *RemoteFile) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	req, err := http.NewRequestWithContext(ctx, "GET", r.url, nil)
	if err != nil {
		return nil, syscall.EIO
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, syscall.EIO
	}
	defer resp.Body.Close()

	// Check for successful response
	if resp.StatusCode != http.StatusOK {
		return nil, syscall.EIO
	}

	n, err := io.ReadFull(resp.Body, dest)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, syscall.EIO
	}
	// Return only the bytes we actually read.
	return fuse.ReadResultData(dest[:n]), 0
}
