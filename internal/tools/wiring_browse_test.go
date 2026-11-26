package tools

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs/abstest"
)

type BrowseWiringSuite struct {
	wiringSuite
}

func TestBrowseSuite(t *testing.T) {
	suite.Run(t, new(BrowseWiringSuite))
}

func (s *BrowseWiringSuite) TestListSeries() {
	s.routes["/api/libraries/lib-1/series"] = map[string]any{
		"results": []map[string]any{
			{
				"id": "s-1", "name": "Sword of Truth",
				"books": []map[string]any{
					abstest.BookItem("b1", "Book 1", "Author", 3600.0),
					abstest.BookItem("b2", "Book 2", "Author", 3600.0),
				},
			},
			{"id": "s-2", "name": "Mistborn", "books": []map[string]any{}},
		},
		"total": 2,
	}
	s.StartServer()

	tool := s.tool("list_series")
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

func (s *BrowseWiringSuite) TestGetSeries_FullStack() {
	s.routes["/api/series/s1"] = map[string]any{
		"id": "s1", "name": "Holly Gibney", "description": "Crime",
		"progress": map[string]any{"libraryItemIds": []string{"li_holly", "li_outsider"}},
	}
	// The batch endpoint expands items, so metadata.series is an array.
	s.routes["/api/items/batch/get"] = map[string]any{
		"libraryItems": []map[string]any{
			abstest.BookItemFull("li_holly", map[string]any{
				"title": "Holly", "authorName": "Stephen King",
				"series": []map[string]any{{"id": "s1", "name": "Holly Gibney", "sequence": "3"}},
			}, 36000.0),
			abstest.BookItemFull("li_outsider", map[string]any{
				"title": "The Outsider", "authorName": "Stephen King",
				"series": []map[string]any{{"id": "s1", "name": "Holly Gibney", "sequence": "1"}},
			}, 54000.0),
		},
	}
	s.StartServer()

	result := s.callTool(s.tool("get_series"), map[string]any{"seriesId": "s1"})

	s.JSONEq(`{"id":"s1","name":"Holly Gibney","description":"Crime","books":[
		{"id":"li_outsider","mediaType":"book","title":"The Outsider","author":"Stephen King","duration":"15h 0m","sequence":"1"},
		{"id":"li_holly","mediaType":"book","title":"Holly","author":"Stephen King","duration":"10h 0m","sequence":"3"}
	]}`, resultText(result))
	s.NotContains(resultText(result), `"books":null`)
}

func (s *BrowseWiringSuite) TestGetAuthor() {
	s.routes["/api/authors/a-1"] = map[string]any{
		"id": "a-1", "name": "Terry Pratchett",
		"description": "English author", "numBooks": 41,
		"libraryItems": []map[string]any{
			abstest.BookItem("b1", "Guards! Guards!", "Terry Pratchett", 36000.0),
		},
	}
	s.StartServer()

	tool := s.tool("get_author")
	result := s.callTool(tool, map[string]any{"authorId": "a-1"})

	var d authorDetail
	s.resultJSON(result, &d)

	s.Equal("Terry Pratchett", d.Name)
	s.Equal(41, d.NumBooks)
	s.Require().Len(d.Books, 1)
	s.Equal("Guards! Guards!", d.Books[0].Title)
}

func (s *BrowseWiringSuite) TestListCollections() {
	s.routes["/api/libraries/lib-1/collections"] = map[string]any{
		"results": []map[string]any{
			{
				"id": "c-1", "name": "Favorites", "description": "My favorites",
				"books": []map[string]any{abstest.BookItem("b1", "Book 1", "Author", 3600.0)},
			},
			{"id": "c-2", "name": "To Listen", "description": "", "books": []map[string]any{}},
		},
		"total": 2,
	}
	s.StartServer()

	tool := s.tool("list_collections")
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
