package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolsetID identifies a group of related tools.
type ToolsetID string

const (
	toolsetLibraries ToolsetID = "libraries"
	toolsetItems     ToolsetID = "items"
	toolsetPlayback  ToolsetID = "playback"
	toolsetBrowse    ToolsetID = "browse"
	toolsetChapters  ToolsetID = "chapters"
)

// ToolsetMeta describes a toolset.
type ToolsetMeta struct {
	ID          ToolsetID
	Description string
	Default     bool
}

var (
	metaLibraries = ToolsetMeta{toolsetLibraries, "Library listing and search", true}
	metaItems     = ToolsetMeta{toolsetItems, "Item details and progress tracking", true}
	metaPlayback  = ToolsetMeta{toolsetPlayback, "Listening stats and sessions", true}
	metaBrowse    = ToolsetMeta{toolsetBrowse, "Browse series, authors, collections", false}
	metaChapters  = ToolsetMeta{toolsetChapters, "Find, fix and save audiobook chapters (includes writes)", false}
)

// AllToolsetMetas returns all defined toolset metadata.
func AllToolsetMetas() []ToolsetMeta {
	return []ToolsetMeta{metaLibraries, metaItems, metaPlayback, metaBrowse, metaChapters}
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
