package tools

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs/abstest"
)

type LibrariesWiringSuite struct {
	wiringSuite
}

func TestLibrariesSuite(t *testing.T) {
	suite.Run(t, new(LibrariesWiringSuite))
}

func (s *LibrariesWiringSuite) TestListLibraries() {
	s.routes["/api/libraries"] = map[string]any{
		"libraries": []map[string]any{
			abstest.Library("lib-1", "Audiobooks", "book"),
			abstest.Library("lib-2", "Podcasts", "podcast"),
		},
	}
	s.StartServer()

	tool := s.tool("list_libraries")
	result := s.callTool(tool, nil)

	var libs []librarySummary
	s.resultJSON(result, &libs)

	s.Require().Len(libs, 2)
	s.Equal("lib-1", libs[0].ID)
	s.Equal("Audiobooks", libs[0].Name)
	s.Equal("book", libs[0].MediaType)
	s.Equal("lib-2", libs[1].ID)
	s.Equal("podcast", libs[1].MediaType)
}

func (s *LibrariesWiringSuite) TestSearchLibrary_Books() {
	s.routes["/api/libraries/lib-1/search"] = map[string]any{
		"book":    []map[string]any{abstest.SearchBookHit("item-1", "Wizard's First Rule", "Terry Goodkind", 43200.0)},
		"podcast": []any{},
		"authors": []any{},
		"series":  []any{},
	}
	s.StartServer()

	tool := s.tool("search_library")
	result := s.callTool(tool, map[string]any{"libraryId": "lib-1", "query": "wizard"})

	var results []searchResultItem
	s.resultJSON(result, &results)

	s.Require().Len(results, 1)
	s.Equal("book", results[0].Type)
	s.Equal("Wizard's First Rule", results[0].Title)
	s.Equal("Terry Goodkind", results[0].Author)
	s.Equal("12h 0m", results[0].Duration)
}

func (s *LibrariesWiringSuite) TestSearchLibrary_NoResults() {
	s.routes["/api/libraries/lib-1/search"] = abstest.EmptySearchResults()
	s.StartServer()

	tool := s.tool("search_library")
	result := s.callTool(tool, map[string]any{"libraryId": "lib-1", "query": "nonexistent"})

	s.Equal(`No results found for "nonexistent"`, resultText(result))
}

func (s *LibrariesWiringSuite) TestSearchLibrary_MissingRequiredParam() {
	s.StartServer()

	tool := s.tool("search_library")
	result := s.callTool(tool, map[string]any{"query": "test"})

	s.True(result.IsError)
}

func (s *LibrariesWiringSuite) TestSearchLibrary_AuthorsAndSeries() {
	s.routes["/api/libraries/lib-1/search"] = map[string]any{
		"book":    []any{},
		"podcast": []any{},
		"authors": []map[string]any{{"id": "a-1", "name": "Brandon Sanderson"}},
		"series":  []map[string]any{{"series": map[string]any{"id": "s-1", "name": "Stormlight Archive"}}},
	}
	s.StartServer()

	tool := s.tool("search_library")
	result := s.callTool(tool, map[string]any{"libraryId": "lib-1", "query": "sanderson"})

	var results []searchResultItem
	s.resultJSON(result, &results)

	s.Require().Len(results, 2)
	s.Equal("author", results[0].Type)
	s.Equal("Brandon Sanderson", results[0].Title)
	s.Equal("series", results[1].Type)
	s.Equal("Stormlight Archive", results[1].Title)
}

func (s *LibrariesWiringSuite) TestListLibraryItems() {
	s.routes["/api/libraries/lib-1/items"] = map[string]any{
		"results": []map[string]any{
			abstest.BookItem("item-1", "Dune", "Frank Herbert", 75600.0),
			abstest.BookItem("item-2", "Foundation", "Isaac Asimov", 28800.0),
		},
		"total": 2,
		"page":  0,
	}
	s.StartServer()

	tool := s.tool("list_library_items")
	result := s.callTool(tool, map[string]any{"libraryId": "lib-1"})

	var out struct {
		Items []libraryItemSummary `json:"items"`
		Total int                  `json:"total"`
		Page  int                  `json:"page"`
	}
	s.resultJSON(result, &out)

	s.Equal(2, out.Total)
	s.Require().Len(out.Items, 2)
	s.Equal("Dune", out.Items[0].Title)
	s.Equal("21h 0m", out.Items[0].Duration)
	s.Equal("Foundation", out.Items[1].Title)
}

func (s *LibrariesWiringSuite) TestListLibraryItems_Podcast() {
	s.routes["/api/libraries/lib-2/items"] = map[string]any{
		"results": []map[string]any{abstest.PodcastItem("pod-1", "Science Friday", "SciFri", 100)},
		"total":   1,
		"page":    0,
	}
	s.StartServer()

	tool := s.tool("list_library_items")
	result := s.callTool(tool, map[string]any{"libraryId": "lib-2"})

	s.False(result.IsError)
}
