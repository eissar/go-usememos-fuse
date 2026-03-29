package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

// MemoFileSystemRoot is a specialized filesystem root that integrates with MemoClient
type MemoFileSystemRoot struct {
	fs.Inode
	inodeManager *InodeManager
	client       *MemoClient
	memoInfos    []MemoInfo
}

// NewMemoFileSystemRoot creates a new filesystem root with memo client integration
func NewMemoFileSystemRoot(client *MemoClient) (*MemoFileSystemRoot, error) {
	return &MemoFileSystemRoot{
		inodeManager: NewInodeManager(2),
		client:       client,
	}, nil
}

// OnAdd populates the filesystem with memos from the MemoClient
func (r *MemoFileSystemRoot) OnAdd(ctx context.Context) {
	// List memos from the server
	memos, err := r.client.ListMemos(ctx, nil)
	if err != nil {
		fmt.Printf("Warning: Failed to list memos: %v\n", err)
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
	}

	fmt.Printf("Mounted %d memos to filesystem\n", len(memos))
}

// Getattr returns attributes for the root directory
func (r *MemoFileSystemRoot) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = 0o755
	return 0
}

// Stats returns current filesystem statistics
func (r *MemoFileSystemRoot) Stats() (inodeCount int, memoCount int) {
	return r.inodeManager.Count(), len(r.memoInfos)
}

// TestSimpleIntegration tests the complete integration flow
func TestSimpleIntegration(t *testing.T) {
	// Skip if no memos server is available
	if os.Getenv("MEMOS_SERVER_URL") == "" {
		t.Skip("Skipping integration test: MEMOS_SERVER_URL not set")
	}

	// Configuration
	mountPoint := "/tmp/memos-test"
	serverURL := os.Getenv("MEMOS_SERVER_URL")
	accessToken := os.Getenv("MEMOS_ACCESS_TOKEN") // Optional

	// Check if already mounted and non-empty
	if isMountedAndNonEmpty(mountPoint) {
		t.Logf("Mount point %s is already mounted and non-empty", mountPoint)
		// We'll proceed with the test anyway
	}

	// Create mount point if it doesn't exist
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		t.Fatalf("Failed to create mount point: %v", err)
	}

	// Clean up mount point after test
	defer func() {
		if err := os.RemoveAll(mountPoint); err != nil {
			t.Logf("Warning: Failed to clean up mount point: %v", err)
		}
	}()

	// Initialize timing measurements
	var (
		clientCreationTime time.Duration
		memoListingTime    time.Duration
		mountingTime      time.Duration
		fileReadingTime   time.Duration
	)

	// Step 1: Create MemoClient with gRPC connection
	clientStart := time.Now()
	config := ClientConfig{
		BaseURL:     serverURL,
		AccessToken: accessToken,
		HTTPTimeout: 30 * time.Second,
	}
	
	client, err := NewMemoClient(config)
	if err != nil {
		t.Fatalf("Failed to create MemoClient: %v", err)
	}
	defer client.Close()
	clientCreationTime = time.Since(clientStart)
	t.Logf("✓ MemoClient created in %v", clientCreationTime)

	// Step 2: Test gRPC connection by listing memos
	listingStart := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	memos, err := client.ListMemos(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list memos: %v", err)
	}
	memoListingTime = time.Since(listingStart)
	t.Logf("✓ Listed %d memos in %v", len(memos), memoListingTime)

	if len(memos) == 0 {
		t.Skip("Skipping test: No memos found on server")
	}

	// Step 3: Mount filesystem
	mountStart := time.Now()
	root, err := NewMemoFileSystemRoot(client)
	if err != nil {
		t.Fatalf("Failed to create filesystem root: %v", err)
	}

	opts := &fs.Options{}
	opts.Debug = testing.Verbose()

	server, err := fs.Mount(mountPoint, root, opts)
	if err != nil {
		t.Fatalf("Failed to mount filesystem: %v", err)
	}
	mountingTime = time.Since(mountStart)
	t.Logf("✓ Filesystem mounted in %v", mountingTime)

	// Ensure unmount happens
	defer func() {
		if err := server.Unmount(); err != nil {
			t.Logf("Warning: Failed to unmount filesystem: %v", err)
		}
	}()

	// Wait a moment for filesystem to be ready
	time.Sleep(500 * time.Millisecond)

	// Step 4: Ensure memos are listed in the directory
	dirEntries, err := ioutil.ReadDir(mountPoint)
	if err != nil {
		t.Fatalf("Failed to read mount point directory: %v", err)
	}

	if len(dirEntries) == 0 {
		t.Error("No files found in mounted directory")
	} else {
		t.Logf("✓ Found %d files in mounted directory", len(dirEntries))
	}

	// Step 5: Ensure we can retrieve text content from a memo
	if len(dirEntries) > 0 {
		// Try to read the first file
		firstFile := filepath.Join(mountPoint, dirEntries[0].Name())
		
		readingStart := time.Now()
		content, err := ioutil.ReadFile(firstFile)
		fileReadingTime = time.Since(readingStart)
		
		if err != nil {
			t.Errorf("Failed to read file %s: %v", firstFile, err)
		} else {
			t.Logf("✓ Read %d bytes from %s in %v", len(content), dirEntries[0].Name(), fileReadingTime)
			
			// Verify content is not empty
			if len(content) == 0 {
				t.Errorf("File %s is empty", firstFile)
			} else {
				t.Logf("✓ File content preview: %s", string(firstN(content, 100)))
			}
		}
	}

	// Step 6: Capture and report timings
	t.Logf("=== Integration Test Timings ===")
	t.Logf("Client creation: %v", clientCreationTime)
	t.Logf("Memo listing:    %v", memoListingTime)
	t.Logf("Mounting:        %v", mountingTime)
	t.Logf("File reading:    %v", fileReadingTime)
	t.Logf("Total:           %v", clientCreationTime+memoListingTime+mountingTime+fileReadingTime)

	// Verify filesystem statistics
	inodeCount, memoCount := root.Stats()
	if memoCount != len(memos) {
		t.Errorf("Mismatch in memo count: filesystem has %d, server has %d", memoCount, len(memos))
	} else {
		t.Logf("✓ Filesystem stats: %d inodes, %d memos", inodeCount, memoCount)
	}
}

// Helper function to check if directory is mounted and non-empty
func isMountedAndNonEmpty(path string) bool {
	// Check if path exists and is a directory
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}

	// Try to read directory contents
	entries, err := ioutil.ReadDir(path)
	if err != nil {
		return false
	}

	return len(entries) > 0
}

// Helper function to get first N bytes of content
func firstN(content []byte, n int) []byte {
	if len(content) <= n {
		return content
	}
	return content[:n]
}

// TestIntegrationWithMock tests integration with a mock server
func TestIntegrationWithMock(t *testing.T) {
	// This test uses environment variables to determine if it should run
	if os.Getenv("RUN_MOCK_INTEGRATION") != "true" {
		t.Skip("Skipping mock integration test: RUN_MOCK_INTEGRATION not set to true")
	}

	t.Log("Running integration test with mock setup")
	// This would be extended to use a test memos server
}
