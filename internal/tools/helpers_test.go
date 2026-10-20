package tools

import (
	"context"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errBoom = errors.New("ABS API 500: boom")

// findTool returns the ServerTool with the given name, or nil.
func findTool(tools []ServerTool, name string) *ServerTool {
	for i := range tools {
		if tools[i].Tool.Name == name {
			return &tools[i]
		}
	}
	return nil
}

// call invokes the named tool's handler with args.
func call(t *testing.T, tools []ServerTool, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	tool := findTool(tools, name)
	require.NotNil(t, tool, "tool %q not found", name)
	result, err := tool.Handler(context.Background(), &mcp.CallToolRequest{}, args)
	require.NoError(t, err)
	return result
}

// resultText extracts the text content from a CallToolResult.
func resultText(result *mcp.CallToolResult) string {
	if len(result.Content) == 0 {
		return ""
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		return ""
	}
	return tc.Text
}

// assertJSON checks a successful result's JSON text.
func assertJSON(t *testing.T, want string, result *mcp.CallToolResult) {
	t.Helper()
	require.False(t, result.IsError, "unexpected tool error: %s", resultText(result))
	assert.JSONEq(t, want, resultText(result))
}

// assertToolText checks a successful plain-text result.
func assertToolText(t *testing.T, want string, result *mcp.CallToolResult) {
	t.Helper()
	assert.False(t, result.IsError, "unexpected tool error")
	assert.Equal(t, want, resultText(result))
}

// assertToolError checks that result is a tool error with exactly this text.
func assertToolError(t *testing.T, want string, result *mcp.CallToolResult) {
	t.Helper()
	assert.True(t, result.IsError, "expected a tool error")
	assert.Equal(t, want, resultText(result))
}
