# gRPC/usememos Connection Implementation

## Overview

This document describes the implementation of gRPC connectivity to usememos servers using the Connect protocol (connectrpc/connect-go). The implementation allows the FUSE filesystem to connect to both local and remote memos servers and enumerate memos via gRPC.

## Architecture

### Protocol Stack
- **Transport**: HTTP/1.1 or HTTP/2
- **Protocol**: Connect (gRPC-compatible)
- **Serialization**: Protocol Buffers (protobuf) or JSON
- **Authentication**: Bearer token (optional)

### Key Components

1. **MemoClient** - Main client wrapper for Connect protocol
2. **ClientConfig** - Configuration for server connection
3. **SDKError** - Error handling wrapper
4. **Service Clients** - Generated Connect clients for each service

## Connection URL Format

### Correct Endpoint Pattern
Connect protocol endpoints follow this pattern:
```
http://host:port/memos.api.v1.ServiceName/MethodName
```

### Examples
- ListMemos: `http://localhost:5230/memos.api.v1.MemoService/ListMemos`
- GetMemo: `http://mini-server.galago-arcturus.ts.net:5230/memos.api.v1.MemoService/GetMemo`

### Important Note
The base URL should **NOT** include `/api/v1` suffix. The Connect endpoints are at the root path.

## Implementation Details

### Client Configuration
```go
type ClientConfig struct {
    BaseURL     string        // e.g., "http://localhost:5230"
    AccessToken string        // Optional authentication
    HTTPTimeout time.Duration // Default: 30 seconds
}
```

### URL Construction (Fixed)
The key fix was removing the incorrect `/api/v1` suffix:

**Before (incorrect):**
```go
baseURL := strings.TrimRight(parsedURL.String(), "/") + "/api/v1"
```

**After (correct):**
```go
baseURL := strings.TrimRight(parsedURL.String(), "/")
```

### Service Clients
- `apiv1connect.NewMemoServiceClient()` - Memo operations
- `apiv1connect.NewAuthServiceClient()` - Authentication
- `apiv1connect.NewAttachmentServiceClient()` - File attachments

## Testing and Validation

### Successful Endpoints Tested
1. **Local Server**: `http://localhost:5230`
2. **Remote Server**: `http://mini-server.galago-arcturus.ts.net:5230`

### Test Results
- ✅ DNS resolution working for both endpoints
- ✅ gRPC/Connect protocol communication established
- ✅ ListMemos operation successful
- ✅ Authentication support available
- ✅ Error handling implemented

## Error Handling

### SDKError Wrapper
Custom error wrapper provides:
- Standardized error codes
- Detailed error messages
- Root cause preservation

### Error Codes
- `UNAVAILABLE` - Connection issues
- `UNAUTHORIZED` - Authentication failures
- `NOT_FOUND` - Resource not found
- `INVALID_REQUEST` - Bad request parameters
- `UNKNOWN` - Unclassified errors

## Configuration Examples

### Basic Connection
```go
config := ClientConfig{
    BaseURL:     "http://mini-server.galago-arcturus.ts.net:5230",
    HTTPTimeout: 30 * time.Second,
}
client, err := NewMemoClient(config)
```

### Authenticated Connection
```go
config := ClientConfig{
    BaseURL:     "http://localhost:5230",
    AccessToken: "your-token-here",
    HTTPTimeout: 30 * time.Second,
}
client, err := NewMemoClient(config)
```

## Troubleshooting

### Common Issues

1. **"404 Not Found" Errors**
   - Cause: Incorrect URL construction (appending `/api/v1`)
   - Fix: Use root path for base URL

2. **DNS Resolution Issues**
   - Cause: Network configuration or server unreachable
   - Fix: Verify endpoint accessibility via ping/curl

3. **Authentication Failures**
   - Cause: Invalid or missing access token
   - Fix: Verify token validity and server authentication requirements

### Debugging Steps

1. **Test DNS Resolution**
   ```bash
   ping mini-server.galago-arcturus.ts.net
   ```

2. **Test HTTP Connectivity**
   ```bash
   curl http://mini-server.galago-arcturus.ts.net:5230/api/v1/memos
   ```

3. **Test Connect Protocol**
   ```bash
   curl -X POST http://mini-server.galago-arcturus.ts.net:5230/memos.api.v1.MemoService/ListMemos \
        -H "Content-Type: application/json" -d '{}'
   ```

## Integration with FUSE Filesystem

### Current Integration
- MemoClient used for memo enumeration
- File metadata retrieval via gRPC
- Authentication support for private memos

### Future Enhancements
- Real-time memo updates via gRPC streaming
- File upload/download via attachment service
- Caching layer for improved performance

## Dependencies

### Required Packages
- `connectrpc.com/connect` - Connect protocol implementation
- `github.com/usememos/memos/proto/gen/api/v1` - Memos protobuf definitions
- `github.com/usememos/memos/proto/gen/api/v1/apiv1connect` - Generated Connect clients

### Version Compatibility
- Connect protocol: v1.19.1+
- Memos API: v0.26.2+

## Security Considerations

### Authentication
- Bearer token authentication supported
- Token passed via Authorization header
- Optional for public endpoints

### Transport Security
- HTTP/HTTPS supported
- No built-in TLS configuration
- Recommend HTTPS for production use

## Performance Characteristics

### Connection Pooling
- HTTP client reuse for multiple requests
- Connection keep-alive supported
- Timeout configuration available

### Protocol Efficiency
- Protobuf serialization for compact messages
- HTTP/2 multiplexing for concurrent requests
- Binary protocol for reduced bandwidth

## Monitoring and Logging

### Error Tracking
- Structured error handling
- Detailed error messages with context
- Root cause preservation

### Metrics
- Connection success/failure rates
- Request/response timing
- Error frequency by type

## Conclusion

The gRPC/usememos connection implementation provides robust connectivity to memos servers using the Connect protocol. The key insight was understanding the correct endpoint pattern (`/memos.api.v1.*`) and avoiding the incorrect `/api/v1` suffix in URL construction.

The implementation supports both local and remote servers, authentication, error handling, and integration with the FUSE filesystem for memo enumeration and file operations.

---

**Last Updated**: March 29, 2026  
**Tested With**: 
- Local Server: http://localhost:5230
- Remote Server: http://mini-server.galago-arcturus.ts.net:5230
- Protocol: Connect/gRPC
- Memos Version: v0.26.2
