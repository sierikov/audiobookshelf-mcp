package audiobookshelf

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Config holds server configuration parsed from environment variables.
type Config struct {
	URL      string
	Token    string
	Toolsets []string // from AUDIOBOOKSHELF_TOOLSETS, empty means defaults
}

// NewServer creates a configured MCP server with the enabled tools registered.
func NewServer(cfg Config) *mcp.Server {
	client := NewClient(cfg.URL, cfg.Token)

	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "audiobookshelf-mcp-server",
			Version: "0.1.0",
		},
		nil,
	)

	enabled := resolveToolsets(cfg.Toolsets)
	for _, st := range AllTools(client) {
		if enabled[st.Toolset.ID] {
			st.Register(server)
		}
	}

	return server
}

// Run creates the server and runs it on stdio transport.
func Run(ctx context.Context, cfg Config) error {
	server := NewServer(cfg)
	return server.Run(ctx, &mcp.StdioTransport{})
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
