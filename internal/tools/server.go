package tools

import (
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AllTools returns every tool the server can offer, backed by svcs.
func AllTools(svcs Services) []ServerTool {
	tools := make([]ServerTool, 0, 25)
	tools = append(tools, LibraryTools(svcs.Libraries)...)
	tools = append(tools, ItemTools(svcs.Items)...)
	tools = append(tools, PlaybackTools(svcs.Playback)...)
	tools = append(tools, BrowseTools(svcs.Browse)...)
	tools = append(tools, ChapterTools(svcs.Chapters)...)
	tools = append(tools, MetadataTools(svcs.Metadata)...)
	return tools
}

// NewServer creates an MCP server with the tools of the enabled toolsets.
// An empty toolsets list means the defaults; "all" enables everything.
func NewServer(svcs Services, toolsets []string) *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "audiobookshelf-mcp-server",
			Version: "0.2.0",
		},
		nil,
	)

	enabled := resolveToolsets(toolsets)
	for _, st := range AllTools(svcs) {
		if enabled[st.Toolset.ID] {
			st.Register(server)
		}
	}
	return server
}

// resolveToolsets converts a list of toolset names into an enabled set.
// Empty input means default toolsets. "all" enables everything.
func resolveToolsets(names []string) map[ToolsetID]bool {
	if len(names) == 0 {
		return DefaultToolsets()
	}

	enabled := make(map[ToolsetID]bool)
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "all" {
			for _, ts := range AllToolsetMetas() {
				enabled[ts.ID] = true
			}
			return enabled
		}
		enabled[ToolsetID(name)] = true
	}
	return enabled
}
