package audiobookshelf

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type BrowseSuite struct {
	ToolSuite
}

func TestBrowseSuite(t *testing.T) {
	suite.Run(t, new(BrowseSuite))
}

func (s *BrowseSuite) TestListSeries() {
	s.routes["/api/libraries/lib-1/series"] = map[string]any{
		"results": []map[string]any{
			{
				"id": "s-1", "name": "Sword of Truth",
				"books": []map[string]any{
					bookItem("b1", "Book 1", "Author", 3600.0),
					bookItem("b2", "Book 2", "Author", 3600.0),
				},
			},
			{"id": "s-2", "name": "Mistborn", "books": []map[string]any{}},
		},
		"total": 2,
	}
	s.StartServer()

	tool := findTool(BrowseTools(s.client), "list_series")
	result := s.callTool(tool, map[string]any{"libraryId": "lib-1"})

	var out struct {
		Series []seriesSummary `json:"series"`
		Total  int             `json:"total"`
	}
	s.resultJSON(result, &out)

	s.Equal(2, out.Total)
	s.Require().Len(out.Series, 2)
	s.Equal("Sword of Truth", out.Series[0].Name)
	s.Equal(2, out.Series[0].NumBooks)
	s.Equal("Mistborn", out.Series[1].Name)
	s.Equal(0, out.Series[1].NumBooks)
}

func (s *BrowseSuite) TestGetSeries() {
	s.routes["/api/series/s-1"] = map[string]any{
		"id": "s-1", "name": "Stormlight Archive", "description": "Epic fantasy",
		"books": []map[string]any{
			bookItem("b1", "The Way of Kings", "Brandon Sanderson", 190800.0),
		},
	}
	s.StartServer()

	tool := findTool(BrowseTools(s.client), "get_series")
	result := s.callTool(tool, map[string]any{"seriesId": "s-1"})

	var d seriesDetail
	s.resultJSON(result, &d)

	s.Equal("Stormlight Archive", d.Name)
	s.Equal("Epic fantasy", d.Description)
	s.Require().Len(d.Books, 1)
	s.Equal("The Way of Kings", d.Books[0].Title)
	s.Equal("53h 0m", d.Books[0].Duration)
}

func (s *BrowseSuite) TestGetAuthor() {
	s.routes["/api/authors/a-1"] = map[string]any{
		"id": "a-1", "name": "Terry Pratchett",
		"description": "English author", "numBooks": 41,
		"libraryItems": []map[string]any{
			bookItem("b1", "Guards! Guards!", "Terry Pratchett", 36000.0),
		},
	}
	s.StartServer()

	tool := findTool(BrowseTools(s.client), "get_author")
	result := s.callTool(tool, map[string]any{"authorId": "a-1"})

	var d authorDetail
	s.resultJSON(result, &d)

	s.Equal("Terry Pratchett", d.Name)
	s.Equal(41, d.NumBooks)
	s.Require().Len(d.Books, 1)
	s.Equal("Guards! Guards!", d.Books[0].Title)
}

func (s *BrowseSuite) TestListCollections() {
	s.routes["/api/libraries/lib-1/collections"] = map[string]any{
		"results": []map[string]any{
			{
				"id": "c-1", "name": "Favorites", "description": "My favorites",
				"books": []map[string]any{bookItem("b1", "Book 1", "Author", 3600.0)},
			},
			{"id": "c-2", "name": "To Listen", "description": "", "books": []map[string]any{}},
		},
		"total": 2,
	}
	s.StartServer()

	tool := findTool(BrowseTools(s.client), "list_collections")
	result := s.callTool(tool, map[string]any{"libraryId": "lib-1"})

	var out struct {
		Collections []collectionSummary `json:"collections"`
		Total       int                 `json:"total"`
	}
	s.resultJSON(result, &out)

	s.Equal(2, out.Total)
	s.Require().Len(out.Collections, 2)
	s.Equal("Favorites", out.Collections[0].Name)
	s.Equal(1, out.Collections[0].NumBooks)
	s.Equal("To Listen", out.Collections[1].Name)
	s.Equal(0, out.Collections[1].NumBooks)
}
