package tools

import (
	"bytes"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// marshalJSON encodes v as JSON without escaping <, > and &, so descriptions
// stay readable and don't grow by six characters per angle bracket. Clients
// read the result as JSON, never as HTML.
func marshalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// textResult returns a successful tool result with text content.
func textResult(text string) (*mcp.CallToolResult, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}, nil
}

// marshalledResult marshals v as JSON and returns it as a text result.
func marshalledResult(v any) (*mcp.CallToolResult, error) {
	data, err := marshalJSON(v)
	if err != nil {
		return errResult("failed to marshal result: " + err.Error())
	}
	return textResult(string(data))
}

// errResult returns an error tool result (API-level error, not protocol error).
func errResult(msg string) (*mcp.CallToolResult, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
		IsError: true,
	}, nil
}
