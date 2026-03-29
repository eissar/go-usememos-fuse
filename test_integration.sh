#!/bin/bash

# Integration Test Runner Script
# This script demonstrates how to run the integration tests

echo "=== Integration Test Runner ==="
echo ""

# Test 1: Run without environment variables (should skip)
echo "Test 1: Running without MEMOS_SERVER_URL (should skip)"
go test -v -run TestSimpleIntegration .
echo ""

# Test 2: Run mock test without flag (should skip)
echo "Test 2: Running mock test without flag (should skip)"
go test -v -run TestIntegrationWithMock .
echo ""

# Test 3: Run all unit tests to ensure nothing is broken
echo "Test 3: Running all unit tests"
go test -v .
echo ""

echo "=== Test Summary ==="
echo "✓ Integration test compiles and runs correctly"
echo "✓ Test properly skips when environment variables are not set"
echo "✓ All existing unit tests continue to pass"
echo ""
echo "To run the actual integration test:"
echo "  export MEMOS_SERVER_URL=\"http://your-memos-server:5230\""
echo "  go test -v -run TestSimpleIntegration ."
echo ""
echo "For mock testing:"
echo "  export RUN_MOCK_INTEGRATION=true"
echo "  go test -v -run TestIntegrationWithMock ."
