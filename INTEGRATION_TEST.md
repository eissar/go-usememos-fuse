# Integration Test Documentation

## Overview

The `TestSimpleIntegration` test provides a comprehensive integration test that verifies the complete flow of mounting memos via gRPC connection to a FUSE filesystem.

## Test Requirements

### Environment Variables

To run the integration test, you need to set the following environment variables:

- `MEMOS_SERVER_URL`: The URL of your memos server (e.g., `http://localhost:5230` or `http://mini-server.galago-arcturus.ts.net:5230`)
- `MEMOS_ACCESS_TOKEN`: (Optional) Access token for authenticated servers

### Prerequisites

1. A running memos server accessible via the provided URL
2. FUSE filesystem support on your system
3. Appropriate permissions to mount filesystems

## Running the Test

### Basic Test Execution

```bash
# Set required environment variables
export MEMOS_SERVER_URL="http://localhost:5230"
# Optional: export MEMOS_ACCESS_TOKEN="your-token"

# Run the integration test
go test -v -run TestSimpleIntegration ./integration_test.go
```

### Verbose Output

For detailed timing information and step-by-step logging:

```bash
go test -v -run TestSimpleIntegration ./integration_test.go
```

### Skipping the Test

The test will automatically skip if:
- `MEMOS_SERVER_URL` is not set
- No memos are found on the server
- The mount point is already in use

## Test Flow

The integration test performs the following steps:

1. **Client Creation**: Creates a MemoClient with gRPC connection
2. **Memo Listing**: Lists memos from the server to verify connectivity
3. **Filesystem Mounting**: Mounts memos as files to `/tmp/memos-test`
4. **Directory Listing**: Verifies memos appear as files in the mounted directory
5. **Content Retrieval**: Reads content from a memo file to verify data integrity
6. **Timing Capture**: Measures and reports performance timings for each step

## Test Output

The test provides detailed output including:

- ✓ Checkmarks for successful operations
- Timing measurements for each step
- File content previews
- Filesystem statistics
- Error messages for failures

## Example Output

```
=== RUN   TestSimpleIntegration
✓ MemoClient created in 150ms
✓ Listed 5 memos in 300ms
✓ Filesystem mounted in 200ms
✓ Found 5 files in mounted directory
✓ Read 256 bytes from memo-memos-123.txt in 50ms
✓ File content preview: This is a test memo content...
=== Integration Test Timings ===
Client creation: 150ms
Memo listing:    300ms
Mounting:        200ms
File reading:    50ms
Total:           700ms
✓ Filesystem stats: 5 inodes, 5 memos
--- PASS: TestSimpleIntegration (1.20s)
```

## Troubleshooting

### Common Issues

1. **Permission Denied** when mounting:
   - Ensure user has FUSE mounting privileges
   - Check `/etc/fuse.conf` configuration

2. **Connection Refused** to memos server:
   - Verify server is running and accessible
   - Check firewall settings

3. **No memos found**:
   - Server may not have any memos
   - Check authentication requirements

### Debug Mode

For troubleshooting, run with verbose output:

```bash
go test -v -run TestSimpleIntegration ./integration_test.go
```

## Mock Testing

For development without a real memos server, use the mock test:

```bash
export RUN_MOCK_INTEGRATION=true
go test -v -run TestIntegrationWithMock ./integration_test.go
```

Note: The mock test currently needs to be extended with actual mock server implementation.

## Cleanup

The test automatically:
- Creates and removes the mount point `/tmp/memos-test`
- Unmounts the filesystem after test completion
- Closes the MemoClient connection

Manual cleanup may be needed if the test fails unexpectedly:

```bash
# Unmount if needed
fusermount -u /tmp/memos-test

# Remove mount point
rm -rf /tmp/memos-test
```
