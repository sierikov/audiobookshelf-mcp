package tools

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs/abstest"
)

type MetadataWiringSuite struct {
	wiringSuite
}

func TestMetadataSuite(t *testing.T) {
	suite.Run(t, new(MetadataWiringSuite))
}

// routeSolaris serves one Russian book with one genre and accepts the batch update.
func (s *MetadataWiringSuite) routeSolaris() {
	s.routes["/api/items/batch/get"] = map[string]any{
		"libraryItems": []any{
			abstest.BookWithMetadata("li_1", "Солярис", []string{"Научная фантастика"}, nil, "", ""),
		},
	}
	s.routes["/api/items/batch/update"] = map[string]any{"success": true, "updates": 1}
}

func (s *MetadataWiringSuite) TestAddGenres_FullStack() {
	s.routeSolaris()
	s.StartServer()

	result := s.callTool(s.tool("add_genres"), map[string]any{
		"itemIds": []any{"li_1"},
		"genres":  []any{"Science Fiction"},
	})

	s.Require().False(result.IsError, resultText(result))
	s.JSONEq(`[{"id":"li_1","mediaPayload":{"metadata":{"genres":["Научная фантастика","Science Fiction"]}}}]`,
		string(s.srv.LastBody()))

	var out struct {
		Changed int `json:"changed"`
	}
	s.resultJSON(result, &out)
	s.Equal(1, out.Changed)
}

func (s *MetadataWiringSuite) TestUpdateItems_FullStack() {
	s.routeSolaris()
	s.StartServer()

	result := s.callTool(s.tool("update_items"), map[string]any{
		"items": []any{map[string]any{"itemId": "li_1", "language": "Russian"}},
	})

	s.Require().False(result.IsError, resultText(result))
	s.JSONEq(`[{"id":"li_1","mediaPayload":{"metadata":{"language":"Russian"}}}]`,
		string(s.srv.LastBodyFor("/api/items/batch/update")))

	var out struct {
		Changed int `json:"changed"`
	}
	s.resultJSON(result, &out)
	s.Equal(1, out.Changed)
}
