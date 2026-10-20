package tools

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
	"github.com/sierikov/audiobookshelf-mcp/internal/abs/abstest"
	"github.com/sierikov/audiobookshelf-mcp/internal/service"
)

// wiringSuite runs tools on the real services and HTTP client against a fake
// Audiobookshelf server. These are the original end-to-end handler tests,
// kept to prove the layers fit together. Set routes, then call StartServer.
type wiringSuite struct {
	suite.Suite
	routes map[string]any
	svcs   Services
	srv    *abstest.Server
}

func (s *wiringSuite) SetupTest() {
	s.routes = make(map[string]any)
}

// StartServer starts the fake API with the current routes and wires the real
// client and services to it.
func (s *wiringSuite) StartServer() {
	s.srv = abstest.NewServer(s.T(), s.routes)
	client := abs.NewClient(s.srv.URL, abstest.Token)
	s.svcs = Services{
		Libraries: service.NewLibraries(client),
		Items:     service.NewItems(client),
		Playback:  service.NewPlayback(client),
		Browse:    service.NewBrowse(client),
		Chapters:  service.NewChapters(client),
	}
}

// tool returns the named tool wired to the services.
func (s *wiringSuite) tool(name string) *ServerTool {
	return findTool(AllTools(s.svcs), name)
}

// callTool invokes a tool handler with the given args.
func (s *wiringSuite) callTool(tool *ServerTool, args map[string]any) *mcp.CallToolResult {
	s.T().Helper()
	s.Require().NotNil(tool, "tool must not be nil")
	result, err := tool.Handler(context.Background(), &mcp.CallToolRequest{}, args)
	s.Require().NoError(err)
	return result
}

// resultJSON unmarshals the text content into v.
func (s *wiringSuite) resultJSON(result *mcp.CallToolResult, v any) {
	s.T().Helper()
	text := resultText(result)
	s.Require().NotEmpty(text, "result has no text content")
	s.Require().NoError(json.Unmarshal([]byte(text), v))
}
