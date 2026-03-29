package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/usememos/memos/proto/gen/api/v1"
	"github.com/usememos/memos/proto/gen/api/v1/apiv1connect"
)

// SDKError represents an error from the Memos SDK/API.
type SDKError struct {
	Code    string
	Message string
	Cause   error
}

func (e *SDKError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("SDK Error [%s]: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("SDK Error [%s]: %s", e.Code, e.Message)
}

func (e *SDKError) Unwrap() error {
	return e.Cause
}

// SDKErrorCode constants.
const (
	SDKErrorCodeUnavailable    = "UNAVAILABLE"
	SDKErrorCodeUnauthorized   = "UNAUTHORIZED"
	SDKErrorCodeNotFound       = "NOT_FOUND"
	SDKErrorCodeInvalidRequest = "INVALID_REQUEST"
	SDKErrorCodeUnknown        = "UNKNOWN"
)

// MemoClient wraps the official Memos SDK clients.
type MemoClient struct {
	memoClient       apiv1connect.MemoServiceClient
	authClient       apiv1connect.AuthServiceClient
	attachmentClient apiv1connect.AttachmentServiceClient
	baseURL          string
	httpClient       *http.Client
}

// ClientConfig holds configuration for the MemoClient.
type ClientConfig struct {
	BaseURL     string
	AccessToken string
	HTTPTimeout time.Duration
}

// NewMemoClient creates a new SDK client for the Memos API.
func NewMemoClient(config ClientConfig) (*MemoClient, error) {
	parsedURL, err := url.Parse(config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	baseURL := strings.TrimRight(parsedURL.String(), "/")

	timeout := config.HTTPTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	httpClient := &http.Client{
		Timeout: timeout,
	}

	var client connect.HTTPClient = httpClient
	if config.AccessToken != "" {
		client = &authHTTPClient{
			base:        httpClient,
			accessToken: config.AccessToken,
		}
	}

	memoClient := apiv1connect.NewMemoServiceClient(client, baseURL)
	authClient := apiv1connect.NewAuthServiceClient(client, baseURL)
	attachmentClient := apiv1connect.NewAttachmentServiceClient(client, baseURL)

	return &MemoClient{
		memoClient:       memoClient,
		authClient:       authClient,
		attachmentClient: attachmentClient,
		baseURL:          baseURL,
		httpClient:       httpClient,
	}, nil
}

// authHTTPClient wraps an HTTP client to add authentication headers.
type authHTTPClient struct {
	base        *http.Client
	accessToken string
}

func (c *authHTTPClient) Do(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	return c.base.Do(req)
}

// MemoInfo holds simplified memo information for the FUSE filesystem.
type MemoInfo struct {
	Name       string
	ID         string
	Content    string
	Snippet    string
	Tags       []string
	Visibility string
	Pinned     bool
	CreateTime time.Time
	UpdateTime time.Time
	HasFiles   bool
}

// FileInfo holds simplified attachment information.
type FileInfo struct {
	Name         string
	ID           string
	Filename     string
	Size         int64
	ContentType  string
	ExternalLink string
}

// ListMemosOptions provides filtering options for listing memos.
type ListMemosOptions struct {
	PageSize        int32
	Filter          string
	IncludeArchived bool
}

// ListMemos retrieves all memos using the official SDK.
func (c *MemoClient) ListMemos(ctx context.Context, opts *ListMemosOptions) ([]MemoInfo, error) {
	req := &apiv1.ListMemosRequest{}

	if opts != nil {
		if opts.PageSize > 0 {
			req.PageSize = opts.PageSize
		}
		if opts.Filter != "" {
			req.Filter = opts.Filter
		}
		if opts.IncludeArchived {
			req.ShowDeleted = true
		}
	}

	response, err := c.memoClient.ListMemos(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, wrapSDKError(err, "failed to list memos")
	}

	memos := make([]MemoInfo, 0, len(response.Msg.Memos))
	for _, memo := range response.Msg.Memos {
		memos = append(memos, convertAPIMemoToInfo(memo))
	}

	return memos, nil
}

// GetMemo retrieves a single memo by its name (ID).
func (c *MemoClient) GetMemo(ctx context.Context, name string) (*MemoInfo, error) {
	req := &apiv1.GetMemoRequest{
		Name: name,
	}

	response, err := c.memoClient.GetMemo(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, wrapSDKError(err, fmt.Sprintf("failed to get memo %s", name))
	}

	info := convertAPIMemoToInfo(response.Msg)
	return &info, nil
}

// ListMemoAttachments retrieves attachments for a specific memo.
func (c *MemoClient) ListMemoAttachments(ctx context.Context, memoName string) ([]FileInfo, error) {
	req := &apiv1.ListMemoAttachmentsRequest{
		Name: memoName,
	}

	response, err := c.memoClient.ListMemoAttachments(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, wrapSDKError(err, fmt.Sprintf("failed to list attachments for memo %s", memoName))
	}

	files := make([]FileInfo, 0, len(response.Msg.Attachments))
	for _, att := range response.Msg.Attachments {
		files = append(files, convertAPIAttachmentToInfo(att))
	}

	return files, nil
}

// ListAttachments retrieves all attachments using the attachment service.
func (c *MemoClient) ListAttachments(ctx context.Context) ([]FileInfo, error) {
	req := &apiv1.ListAttachmentsRequest{}

	response, err := c.attachmentClient.ListAttachments(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, wrapSDKError(err, "failed to list attachments")
	}

	files := make([]FileInfo, 0, len(response.Msg.Attachments))
	for _, att := range response.Msg.Attachments {
		files = append(files, convertAPIAttachmentToInfo(att))
	}

	return files, nil
}

// Authenticate performs sign-in using username/password.
func (c *MemoClient) Authenticate(ctx context.Context, username, password string) (string, error) {
	req := &apiv1.SignInRequest{
		Credentials: &apiv1.SignInRequest_PasswordCredentials_{
			PasswordCredentials: &apiv1.SignInRequest_PasswordCredentials{
				Username: username,
				Password: password,
			},
		},
	}

	response, err := c.authClient.SignIn(ctx, connect.NewRequest(req))
	if err != nil {
		return "", wrapSDKError(err, "authentication failed")
	}

	return response.Msg.AccessToken, nil
}

// GetCurrentUser retrieves information about the authenticated user.
func (c *MemoClient) GetCurrentUser(ctx context.Context) (*apiv1.User, error) {
	req := &apiv1.GetCurrentUserRequest{}

	response, err := c.authClient.GetCurrentUser(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, wrapSDKError(err, "failed to get current user")
	}

	return response.Msg.User, nil
}

// Close performs any cleanup needed for the client.
func (c *MemoClient) Close() error {
	return nil
}

// convertAPIMemoToInfo converts API Memo to MemoInfo.
func convertAPIMemoToInfo(memo *apiv1.Memo) MemoInfo {
	info := MemoInfo{
		Name:    memo.Name,
		ID:      memo.Name,
		Content: memo.Content,
		Snippet: memo.Snippet,
		Tags:    memo.Tags,
		Pinned:  memo.Pinned,
	}

	switch memo.Visibility {
	case apiv1.Visibility_PRIVATE:
		info.Visibility = "private"
	case apiv1.Visibility_PROTECTED:
		info.Visibility = "protected"
	case apiv1.Visibility_PUBLIC:
		info.Visibility = "public"
	default:
		info.Visibility = "unspecified"
	}

	if memo.CreateTime != nil {
		info.CreateTime = memo.CreateTime.AsTime()
	}
	if memo.UpdateTime != nil {
		info.UpdateTime = memo.UpdateTime.AsTime()
	}

	info.HasFiles = len(memo.Attachments) > 0

	return info
}

// convertAPIAttachmentToInfo converts API Attachment to FileInfo.
func convertAPIAttachmentToInfo(att *apiv1.Attachment) FileInfo {
	return FileInfo{
		Name:         att.Name,
		ID:           att.Name,
		Filename:     att.Filename,
		Size:         att.Size,
		ContentType:  att.Type,
		ExternalLink: att.ExternalLink,
	}
}

// wrapSDKError wraps a connect error into SDKError.
func wrapSDKError(err error, message string) error {
	if err == nil {
		return nil
	}

	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		code := SDKErrorCodeUnknown
		switch connectErr.Code() {
		case connect.CodeUnavailable:
			code = SDKErrorCodeUnavailable
		case connect.CodeUnauthenticated:
			code = SDKErrorCodeUnauthorized
		case connect.CodeNotFound:
			code = SDKErrorCodeNotFound
		case connect.CodeInvalidArgument:
			code = SDKErrorCodeInvalidRequest
		}

		return &SDKError{
			Code:    code,
			Message: message,
			Cause:   connectErr,
		}
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return &SDKError{
			Code:    SDKErrorCodeUnavailable,
			Message: message,
			Cause:   err,
		}
	}

	return &SDKError{
		Code:    SDKErrorCodeUnknown,
		Message: message,
		Cause:   err,
	}
}

// IsSDKError checks if an error is an SDKError.
func IsSDKError(err error) bool {
	if err == nil {
		return false
	}
	var sdkErr *SDKError
	return errors.As(err, &sdkErr)
}

// GetSDKErrorCode extracts the error code from an SDKError.
func GetSDKErrorCode(err error) string {
	if err == nil {
		return ""
	}
	var sdkErr *SDKError
	if errors.As(err, &sdkErr) {
		return sdkErr.Code
	}
	return SDKErrorCodeUnknown
}
