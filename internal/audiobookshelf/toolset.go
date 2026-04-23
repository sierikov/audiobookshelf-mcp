package audiobookshelf

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolsetID identifies a group of related tools.
type ToolsetID string

const (
	ToolsetLibraries ToolsetID = "libraries"
	ToolsetItems     ToolsetID = "items"
	ToolsetPlayback  ToolsetID = "playback"
	ToolsetBrowse    ToolsetID = "browse"
)

// ToolsetMeta describes a toolset.
type ToolsetMeta struct {
	ID          ToolsetID
	Description string
	Default     bool
}

var (
	MetaLibraries = ToolsetMeta{ToolsetLibraries, "Library listing and search", true}
	MetaItems     = ToolsetMeta{ToolsetItems, "Item details and progress tracking", true}
	MetaPlayback  = ToolsetMeta{ToolsetPlayback, "Listening stats and sessions", true}
	MetaBrowse    = ToolsetMeta{ToolsetBrowse, "Browse series, authors, collections", false}
)

// AllToolsetMetas returns all defined toolset metadata.
func AllToolsetMetas() []ToolsetMeta {
	return []ToolsetMeta{MetaLibraries, MetaItems, MetaPlayback, MetaBrowse}
}

// DefaultToolsets returns the set of toolsets enabled by default.
func DefaultToolsets() map[ToolsetID]bool {
	m := make(map[ToolsetID]bool)
	for _, ts := range AllToolsetMetas() {
		if ts.Default {
			m[ts.ID] = true
		}
	}
	return m
}

// ToolHandlerFunc is the handler signature for all tools.
// Arguments are unmarshalled as map[string]any for explicit schema control.
type ToolHandlerFunc func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, error)

// ServerTool pairs a tool definition with its toolset and a registration function.
type ServerTool struct {
	Tool    mcp.Tool
	Toolset ToolsetMeta
	Handler ToolHandlerFunc
}

// Register adds this tool to the MCP server using the non-generic s.AddTool method.
func (st *ServerTool) Register(s *mcp.Server) {
	tool := st.Tool // shallow copy to avoid mutation
	handler := st.Handler
	s.AddTool(&tool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args map[string]any
		if req.Params.Arguments != nil {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
		}
		if args == nil {
			args = make(map[string]any)
		}
		return handler(ctx, req, args)
	})
}

// RequiredParam extracts a required parameter from args.
func RequiredParam[T any](args map[string]any, name string) (T, error) {
	var zero T
	v, ok := args[name]
	if !ok {
		return zero, fmt.Errorf("missing required parameter: %s", name)
	}
	tv, ok := v.(T)
	if !ok {
		return zero, fmt.Errorf("parameter %s has wrong type: expected %T, got %T", name, zero, v)
	}
	return tv, nil
}

// OptionalParam extracts an optional parameter from args.
func OptionalParam[T any](args map[string]any, name string) (T, bool, error) {
	var zero T
	v, ok := args[name]
	if !ok {
		return zero, false, nil
	}
	tv, ok := v.(T)
	if !ok {
		return zero, false, fmt.Errorf("parameter %s has wrong type: expected %T, got %T", name, zero, v)
	}
	return tv, true, nil
}

// OptionalIntParam extracts an optional integer parameter, handling JSON number → int conversion.
func OptionalIntParam(args map[string]any, name string, defaultVal int) (int, error) {
	v, ok := args[name]
	if !ok {
		return defaultVal, nil
	}
	switch n := v.(type) {
	case float64:
		return int(n), nil
	case int:
		return n, nil
	default:
		return 0, fmt.Errorf("parameter %s: expected number, got %T", name, v)
	}
}

// textResult returns a successful tool result with text content.
func textResult(text string) (*mcp.CallToolResult, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}, nil
}

// marshalledResult marshals v as JSON and returns it as a text result.
func marshalledResult(v any) (*mcp.CallToolResult, error) {
	data, err := json.Marshal(v)
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
