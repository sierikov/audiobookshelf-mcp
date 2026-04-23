package audiobookshelf

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
)

// ToolSuite is a shared test suite that sets up an httptest.Server and ABSClient.
// Embed this in domain-specific suites and set Routes before each test.
type ToolSuite struct {
	suite.Suite
	server *httptest.Server
	client *ABSClient
	routes map[string]any
}

func (s *ToolSuite) SetupTest() {
	s.routes = make(map[string]any)
}

// StartServer creates the httptest server with the current routes.
// Call this after setting s.routes in each test.
func (s *ToolSuite) StartServer() {
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		for path, resp := range s.routes {
			if r.URL.Path == path {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
				return
			}
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	s.client = NewClient(s.server.URL, "test-token")
}

func (s *ToolSuite) TearDownTest() {
	if s.server != nil {
		s.server.Close()
	}
}

// findTool returns the ServerTool with the given name from a slice.
func findTool(tools []ServerTool, name string) *ServerTool {
	for i := range tools {
		if tools[i].Tool.Name == name {
			return &tools[i]
		}
	}
	return nil
}

// callTool invokes a tool handler with the given args.
func (s *ToolSuite) callTool(tool *ServerTool, args map[string]any) *mcp.CallToolResult {
	s.T().Helper()
	s.Require().NotNil(tool, "tool must not be nil")
	result, err := tool.Handler(context.Background(), &mcp.CallToolRequest{}, args)
	s.Require().NoError(err)
	return result
}

// resultText extracts text content from a CallToolResult.
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

// resultJSON unmarshals the text content into v.
func (s *ToolSuite) resultJSON(result *mcp.CallToolResult, v any) {
	s.T().Helper()
	text := resultText(result)
	s.Require().NotEmpty(text, "result has no text content")
	s.Require().NoError(json.Unmarshal([]byte(text), v))
}

// bookItem builds a complete book library item fixture.
func bookItem(id, title, author string, duration float64) map[string]any {
	return map[string]any{
		"id":        id,
		"mediaType": "book",
		"media": map[string]any{
			"metadata": map[string]any{
				"title":      title,
				"authorName": author,
			},
			"duration": duration,
		},
	}
}

// bookItemFull builds a book item with all metadata fields.
func bookItemFull(id string, metadata map[string]any, duration float64) map[string]any {
	return map[string]any{
		"id":        id,
		"mediaType": "book",
		"media": map[string]any{
			"metadata": metadata,
			"duration": duration,
		},
	}
}

// podcastItem builds a complete podcast library item fixture.
func podcastItem(id, title, author string, numEpisodes int) map[string]any {
	return map[string]any{
		"id":        id,
		"mediaType": "podcast",
		"media": map[string]any{
			"metadata": map[string]any{
				"title":  title,
				"author": author,
			},
			"numEpisodes": numEpisodes,
		},
	}
}

// searchBookHit wraps a book item in the search response format.
func searchBookHit(id, title, author string, duration float64) map[string]any {
	return map[string]any{"libraryItem": bookItem(id, title, author, duration)}
}

// library builds a library fixture.
func library(id, name, mediaType string) map[string]any {
	return map[string]any{"id": id, "name": name, "mediaType": mediaType}
}

// session builds a listening session fixture.
func session(id, itemID, title, author string, duration float64, startedAt int64) map[string]any {
	return map[string]any{
		"id": id, "libraryItemId": itemID,
		"displayTitle": title, "displayAuthor": author,
		"duration": duration, "startedAt": startedAt,
	}
}

// progress builds a media progress fixture.
func progress(id, itemID string, duration, prog, currentTime float64, finished bool) map[string]any {
	return map[string]any{
		"id": id, "libraryItemId": itemID,
		"duration": duration, "progress": prog, "currentTime": currentTime,
		"isFinished": finished, "lastUpdate": int64(1668586015691), "startedAt": int64(1668120083771),
	}
}

// emptySearchResults builds an empty search response.
func emptySearchResults() map[string]any {
	return map[string]any{
		"book": []any{}, "podcast": []any{}, "authors": []any{}, "series": []any{},
	}
}
