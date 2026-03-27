package main

import (
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
)

func TestSDKError(t *testing.T) {
	err := &SDKError{Code: SDKErrorCodeNotFound, Message: "not found"}
	if err.Error() != "SDK Error [NOT_FOUND]: not found" {
		t.Errorf("unexpected error message: %v", err.Error())
	}
}

func TestIsSDKError(t *testing.T) {
	if IsSDKError(nil) {
		t.Error("IsSDKError(nil) should be false")
	}
	if IsSDKError(errors.New("regular")) {
		t.Error("IsSDKError(regular) should be false")
	}
	if !IsSDKError(&SDKError{Code: "TEST", Message: "test"}) {
		t.Error("IsSDKError(sdkErr) should be true")
	}
}

func TestGetSDKErrorCode(t *testing.T) {
	if GetSDKErrorCode(nil) != "" {
		t.Error("GetSDKErrorCode(nil) should be empty")
	}
	if GetSDKErrorCode(errors.New("regular")) != SDKErrorCodeUnknown {
		t.Error("GetSDKErrorCode(regular) should be UNKNOWN")
	}
	sdkErr := &SDKError{Code: SDKErrorCodeNotFound, Message: "test"}
	if GetSDKErrorCode(sdkErr) != SDKErrorCodeNotFound {
		t.Error("GetSDKErrorCode(sdkErr) should be NOT_FOUND")
	}
}

func TestWrapSDKError(t *testing.T) {
	if wrapSDKError(nil, "test") != nil {
		t.Error("wrapSDKError(nil) should be nil")
	}
	
	connErr := connect.NewError(connect.CodeUnavailable, errors.New("down"))
	wrapped := wrapSDKError(connErr, "list failed")
	if !IsSDKError(wrapped) {
		t.Error("wrapped connect error should be SDKError")
	}
	if GetSDKErrorCode(wrapped) != SDKErrorCodeUnavailable {
		t.Errorf("code = %v, want UNAVAILABLE", GetSDKErrorCode(wrapped))
	}
}

func TestClientConfig(t *testing.T) {
	cfg := ClientConfig{BaseURL: "http://localhost:5230", AccessToken: "tok", HTTPTimeout: 10 * time.Second}
	if cfg.BaseURL != "http://localhost:5230" {
		t.Error("BaseURL mismatch")
	}
}

func TestNewMemoClient_InvalidURL(t *testing.T) {
	_, err := NewMemoClient(ClientConfig{BaseURL: "://invalid"})
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestMemoInfoStruct(t *testing.T) {
	info := MemoInfo{Name: "memos/123", Content: "test", HasFiles: true}
	if info.Name != "memos/123" || !info.HasFiles {
		t.Error("MemoInfo fields incorrect")
	}
}

func TestFileInfoStruct(t *testing.T) {
	info := FileInfo{Name: "att/1", Filename: "test.png", Size: 1024}
	if info.Filename != "test.png" || info.Size != 1024 {
		t.Error("FileInfo fields incorrect")
	}
}

func TestMemoClient_Close(t *testing.T) {
	client, _ := NewMemoClient(ClientConfig{BaseURL: "http://localhost:5230"})
	if err := client.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}
}
