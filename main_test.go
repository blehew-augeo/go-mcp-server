package main

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mssql"
)

type JsonRpcRequest struct {
	Jsonrpc string      `json:"jsonrpc"`
	Id      interface{} `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type JsonRpcResponse struct {
	Jsonrpc string        `json:"jsonrpc"`
	Id      interface{}   `json:"id"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *JsonRpcError `json:"error,omitempty"`
}

type JsonRpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func isDockerAvailable() bool {
	cmd := exec.Command("docker", "version")
	return cmd.Run() == nil
}

func TestMCPServerIntegration(t *testing.T) {
	if !isDockerAvailable() {
		t.Skip("Docker not available")
	}

	ctx := context.Background()
	t.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")

	// Start MSSQL container
	mssqlContainer, err := mssql.RunContainer(ctx,
		testcontainers.WithImage("mcr.microsoft.com/mssql/server:2019-latest"),
		mssql.WithAcceptEULA(),
		mssql.WithPassword("Test123!"),
		testcontainers.WithEnv(map[string]string{"MSSQL_PID": "Express"}),
	)
	require.NoError(t, err)
	defer mssqlContainer.Terminate(ctx)

	// Get connection and set environment
	connectionString, err := mssqlContainer.ConnectionString(ctx)
	require.NoError(t, err)
	originalConnString := os.Getenv("MSSQL_CONNECTION_STRING")
	defer os.Setenv("MSSQL_CONNECTION_STRING", originalConnString)
	os.Setenv("MSSQL_CONNECTION_STRING", connectionString)

	// Build and start server
	require.NoError(t, exec.Command("go", "build", "-o", "mcp-server-test", "main.go").Run())
	defer os.Remove("mcp-server-test")

	serverCmd := exec.Command("./mcp-server-test")
	stdin, err := serverCmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := serverCmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, serverCmd.Start())
	defer func() {
		stdin.Close()
		serverCmd.Process.Kill()
		serverCmd.Wait()
	}()

	scanner := bufio.NewScanner(stdout)
	sendRequest := func(req JsonRpcRequest) JsonRpcResponse {
		reqBytes, _ := json.Marshal(req)
		stdin.Write(append(reqBytes, '\n'))
		scanner.Scan()
		var resp JsonRpcResponse
		json.Unmarshal(scanner.Bytes(), &resp)
		return resp
	}

	time.Sleep(100 * time.Millisecond) // Wait for server startup

	// Test 1: Initialize
	resp := sendRequest(JsonRpcRequest{
		Jsonrpc: "2.0", Id: 1, Method: "initialize",
		Params: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo":      map[string]interface{}{"name": "test", "version": "1.0"},
		},
	})
	assert.Nil(t, resp.Error)
	assert.NotNil(t, resp.Result)

	// Test 2: List tools
	resp = sendRequest(JsonRpcRequest{Jsonrpc: "2.0", Id: 2, Method: "tools/list"})
	assert.Nil(t, resp.Error)
	resultData, _ := json.Marshal(resp.Result)
	assert.Contains(t, string(resultData), "execute_sql")

	// Test 3: Execute SQL query
	resp = sendRequest(JsonRpcRequest{
		Jsonrpc: "2.0", Id: 3, Method: "tools/call",
		Params: map[string]interface{}{
			"name":      "execute_sql",
			"arguments": map[string]interface{}{"query": "SELECT 1 as test"},
		},
	})
	assert.Nil(t, resp.Error)
	resultData, _ = json.Marshal(resp.Result)
	assert.Contains(t, string(resultData), "test")
	assert.Contains(t, string(resultData), "1")

	// Test 4: Error handling - Missing query parameter
	resp = sendRequest(JsonRpcRequest{
		Jsonrpc: "2.0", Id: 4, Method: "tools/call",
		Params: map[string]interface{}{
			"name":      "execute_sql",
			"arguments": map[string]interface{}{}, // Missing query
		},
	})
	assert.Nil(t, resp.Error)
	resultData, _ = json.Marshal(resp.Result)
	assert.Contains(t, strings.ToLower(string(resultData)), "missing")

	// === NEGATIVE TESTS ===

	// Test 5: Invalid SQL syntax should return error in content
	resp = sendRequest(JsonRpcRequest{
		Jsonrpc: "2.0", Id: 5, Method: "tools/call",
		Params: map[string]interface{}{
			"name":      "execute_sql",
			"arguments": map[string]interface{}{"query": "SELECT FROM WHERE INVALID SYNTAX"},
		},
	})
	assert.Nil(t, resp.Error, "Should not have JSON-RPC error")
	resultData, _ = json.Marshal(resp.Result)
	assert.Contains(t, strings.ToLower(string(resultData)), "error", "Should contain error in response content")

	// Test 6: Non-existent tool should return JSON-RPC error or proper error response
	resp = sendRequest(JsonRpcRequest{
		Jsonrpc: "2.0", Id: 6, Method: "tools/call",
		Params: map[string]interface{}{
			"name":      "non_existent_tool",
			"arguments": map[string]interface{}{"query": "SELECT 1"},
		},
	})
	// Either JSON-RPC error OR error in result content is acceptable
	if resp.Error == nil {
		resultData, _ = json.Marshal(resp.Result)
		responseStr := strings.ToLower(string(resultData))
		assert.True(t,
			strings.Contains(responseStr, "error") || strings.Contains(responseStr, "not found"),
			"Should contain error message for non-existent tool, got: %s", string(resultData))
	}

	// Test 7: Invalid method should return error
	resp = sendRequest(JsonRpcRequest{
		Jsonrpc: "2.0", Id: 7, Method: "invalid/method",
		Params: map[string]interface{}{},
	})
	assert.NotNil(t, resp.Error, "Invalid method should return JSON-RPC error")
	assert.Contains(t, strings.ToLower(resp.Error.Message), "not found", "Error should mention method not found")

	// Test 8: Malformed parameters should be handled gracefully
	resp = sendRequest(JsonRpcRequest{
		Jsonrpc: "2.0", Id: 8, Method: "tools/call",
		Params: "this_should_be_an_object_not_a_string",
	})
	// Should either have JSON-RPC error or error in result
	hasError := resp.Error != nil
	if !hasError {
		resultData, _ = json.Marshal(resp.Result)
		hasError = strings.Contains(strings.ToLower(string(resultData)), "error")
	}
	assert.True(t, hasError, "Malformed parameters should result in an error")

	// Test 9: Note about connection string limitations
	// Environment variables don't propagate to already-running server processes
	// The TestMCPServerWithBadConnection test covers bad connection handling
	t.Log("Note: Dynamic connection string changes require server restart in separate processes")

	t.Log("✅ All integration tests passed (including negative tests)!")
}

// Test with invalid connection string to ensure failure detection works
func TestMCPServerWithBadConnection(t *testing.T) {
	// Set invalid connection string
	originalConnString := os.Getenv("MSSQL_CONNECTION_STRING")
	defer os.Setenv("MSSQL_CONNECTION_STRING", originalConnString)
	os.Setenv("MSSQL_CONNECTION_STRING", "sqlserver://invalid:badpass@nonexistent:1433?database=fake")

	// Build and start server
	require.NoError(t, exec.Command("go", "build", "-o", "mcp-server-test-bad", "main.go").Run())
	defer os.Remove("mcp-server-test-bad")

	serverCmd := exec.Command("./mcp-server-test-bad")
	stdin, err := serverCmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := serverCmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, serverCmd.Start())
	defer func() {
		stdin.Close()
		serverCmd.Process.Kill()
		serverCmd.Wait()
	}()

	scanner := bufio.NewScanner(stdout)
	sendRequest := func(req JsonRpcRequest) JsonRpcResponse {
		reqBytes, _ := json.Marshal(req)
		stdin.Write(append(reqBytes, '\n'))
		scanner.Scan()
		var resp JsonRpcResponse
		json.Unmarshal(scanner.Bytes(), &resp)
		return resp
	}

	time.Sleep(100 * time.Millisecond)

	// Initialize should still work
	resp := sendRequest(JsonRpcRequest{
		Jsonrpc: "2.0", Id: 1, Method: "initialize",
		Params: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo":      map[string]interface{}{"name": "test", "version": "1.0"},
		},
	})
	assert.Nil(t, resp.Error)

	// But SQL execution should fail with connection error
	resp = sendRequest(JsonRpcRequest{
		Jsonrpc: "2.0", Id: 2, Method: "tools/call",
		Params: map[string]interface{}{
			"name":      "execute_sql",
			"arguments": map[string]interface{}{"query": "SELECT 1"},
		},
	})
	assert.Nil(t, resp.Error, "Should not have JSON-RPC error")
	resultData, _ := json.Marshal(resp.Result)
	resultStr := strings.ToLower(string(resultData))
	assert.True(t,
		strings.Contains(resultStr, "connection") || strings.Contains(resultStr, "error"),
		"Should contain connection error, got: %s", string(resultData))

	t.Log("✅ Bad connection test passed - properly detected connection failure!")
}

// Test proper connection string handling from server startup
func TestMCPServerConnectionStringHandling(t *testing.T) {
	ctx := context.Background()
	t.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")

	// Start MSSQL container for the good connection test
	mssqlContainer, err := mssql.RunContainer(ctx,
		testcontainers.WithImage("mcr.microsoft.com/mssql/server:2019-latest"),
		mssql.WithAcceptEULA(),
		mssql.WithPassword("Test123!"),
		testcontainers.WithEnv(map[string]string{"MSSQL_PID": "Express"}),
	)
	require.NoError(t, err)
	defer mssqlContainer.Terminate(ctx)

	goodConnectionString, err := mssqlContainer.ConnectionString(ctx)
	require.NoError(t, err)

	// Test 1: Server with missing connection string
	originalConnString := os.Getenv("MSSQL_CONNECTION_STRING")
	defer os.Setenv("MSSQL_CONNECTION_STRING", originalConnString)

	// Clear connection string and test
	os.Setenv("MSSQL_CONNECTION_STRING", "")

	require.NoError(t, exec.Command("go", "build", "-o", "mcp-server-empty", "main.go").Run())
	defer os.Remove("mcp-server-empty")

	emptyServerCmd := exec.Command("./mcp-server-empty")
	emptyStdin, err := emptyServerCmd.StdinPipe()
	require.NoError(t, err)
	emptyStdout, err := emptyServerCmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, emptyServerCmd.Start())
	defer func() {
		emptyStdin.Close()
		emptyServerCmd.Process.Kill()
		emptyServerCmd.Wait()
	}()

	emptyScanner := bufio.NewScanner(emptyStdout)
	sendEmptyRequest := func(req JsonRpcRequest) JsonRpcResponse {
		reqBytes, _ := json.Marshal(req)
		emptyStdin.Write(append(reqBytes, '\n'))
		emptyScanner.Scan()
		var resp JsonRpcResponse
		json.Unmarshal(emptyScanner.Bytes(), &resp)
		return resp
	}

	time.Sleep(100 * time.Millisecond)

	// Initialize should work even without connection string
	resp := sendEmptyRequest(JsonRpcRequest{
		Jsonrpc: "2.0", Id: 1, Method: "initialize",
		Params: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo":      map[string]interface{}{"name": "test", "version": "1.0"},
		},
	})
	assert.Nil(t, resp.Error, "Server should initialize without connection string")

	// SQL execution should fail gracefully
	resp = sendEmptyRequest(JsonRpcRequest{
		Jsonrpc: "2.0", Id: 2, Method: "tools/call",
		Params: map[string]interface{}{
			"name":      "execute_sql",
			"arguments": map[string]interface{}{"query": "SELECT 1"},
		},
	})
	assert.Nil(t, resp.Error, "Should not crash with missing connection string")
	resultData, _ := json.Marshal(resp.Result)
	resultStr := strings.ToLower(string(resultData))
	assert.True(t,
		strings.Contains(resultStr, "not set") || strings.Contains(resultStr, "connection") || strings.Contains(resultStr, "error"),
		"Should contain connection error with missing connection string, got: %s", string(resultData))

	// Test 2: Server with good connection string from start
	os.Setenv("MSSQL_CONNECTION_STRING", goodConnectionString)

	require.NoError(t, exec.Command("go", "build", "-o", "mcp-server-good", "main.go").Run())
	defer os.Remove("mcp-server-good")

	goodServerCmd := exec.Command("./mcp-server-good")
	goodStdin, err := goodServerCmd.StdinPipe()
	require.NoError(t, err)
	goodStdout, err := goodServerCmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, goodServerCmd.Start())
	defer func() {
		goodStdin.Close()
		goodServerCmd.Process.Kill()
		goodServerCmd.Wait()
	}()

	goodScanner := bufio.NewScanner(goodStdout)
	sendGoodRequest := func(req JsonRpcRequest) JsonRpcResponse {
		reqBytes, _ := json.Marshal(req)
		goodStdin.Write(append(reqBytes, '\n'))
		goodScanner.Scan()
		var resp JsonRpcResponse
		json.Unmarshal(goodScanner.Bytes(), &resp)
		return resp
	}

	time.Sleep(100 * time.Millisecond)

	// Initialize should work
	resp = sendGoodRequest(JsonRpcRequest{
		Jsonrpc: "2.0", Id: 1, Method: "initialize",
		Params: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo":      map[string]interface{}{"name": "test", "version": "1.0"},
		},
	})
	assert.Nil(t, resp.Error, "Server should initialize with good connection string")

	// SQL execution should succeed
	resp = sendGoodRequest(JsonRpcRequest{
		Jsonrpc: "2.0", Id: 2, Method: "tools/call",
		Params: map[string]interface{}{
			"name":      "execute_sql",
			"arguments": map[string]interface{}{"query": "SELECT 1 as test_connection"},
		},
	})
	assert.Nil(t, resp.Error, "Should not have error with good connection")
	resultData, _ = json.Marshal(resp.Result)
	assert.Contains(t, string(resultData), "test_connection", "Should successfully execute query")
	assert.Contains(t, string(resultData), "1", "Should return expected result")

	t.Log("✅ Connection string handling test passed!")
}
