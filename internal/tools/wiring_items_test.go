package tools

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs/abstest"
)

type ItemsWiringSuite struct {
	wiringSuite
}

func TestItemsSuite(t *testing.T) {
	suite.Run(t, new(ItemsWiringSuite))
}

func (s *ItemsWiringSuite) TestGetItem_Book() {
	s.routes["/api/items/item-1"] = abstest.BookItemFull("item-1", map[string]any{
		"title":         "Katabasis",
		"subtitle":      "A Novel",
		"authorName":    "R.F. Kuang",
		"narratorName":  "Billie Fulford-Brown",
		"genres":        []string{"Fantasy", "Fiction"},
		"description":   "A descent into the underworld",
		"publishedYear": "2025",
		"publisher":     "Harper Voyager",
		"language":      "English",
	}, 43200.0)
	s.StartServer()

	tool := s.tool("get_item")
	result := s.callTool(tool, map[string]any{"itemId": "item-1"})

	var d itemDetail
	s.resultJSON(result, &d)

	s.Equal("item-1", d.ID)
	s.Equal("Katabasis", d.Title)
	s.Equal("R.F. Kuang", d.Author)
	s.Equal("Billie Fulford-Brown", d.Narrator)
	s.Equal("12h 0m", d.Duration)
	s.Len(d.Genres, 2)
	s.Equal("2025", d.PublishedYear)
}

func (s *ItemsWiringSuite) TestGetItem_BookListsInUpdateItemsShape() {
	s.routes["/api/items/item-1"] = abstest.BookItemFull("item-1", map[string]any{
		"title":        "Joyland",
		"authorName":   "Stephen King, Richard Bachman",
		"narratorName": "Michael Kelly, Will Patton",
		"seriesName":   "Hard Case Crime #1, Holly Gibney",
		"authors": []map[string]any{
			{"id": "au_1", "name": "Stephen King"},
			{"id": "au_2", "name": "Richard Bachman"},
		},
		"narrators": []string{"Michael Kelly", "Will Patton"},
		"series": []map[string]any{
			{"id": "se_1", "name": "Hard Case Crime", "sequence": "1"},
			{"id": "se_2", "name": "Holly Gibney", "sequence": ""},
		},
	}, 36000.0)
	s.StartServer()

	result := s.callTool(s.tool("get_item"), map[string]any{"itemId": "item-1"})

	s.JSONEq(`{
		"id":"item-1","mediaType":"book","title":"Joyland",
		"author":"Stephen King, Richard Bachman","narrator":"Michael Kelly, Will Patton",
		"authors":["Stephen King","Richard Bachman"],"narrators":["Michael Kelly","Will Patton"],
		"series":[{"name":"Hard Case Crime","sequence":"1"},{"name":"Holly Gibney"}],
		"duration":"10h 0m"
	}`, resultText(result))
}

func (s *ItemsWiringSuite) TestGetItem_Podcast() {
	s.routes["/api/items/pod-1"] = abstest.PodcastItem("pod-1", "Hardcore History", "Dan Carlin", 68)
	s.StartServer()

	tool := s.tool("get_item")
	result := s.callTool(tool, map[string]any{"itemId": "pod-1"})

	var d itemDetail
	s.resultJSON(result, &d)

	s.Equal("podcast", d.MediaType)
	s.Equal("Hardcore History", d.Title)
	s.Equal(68, d.NumEpisodes)
}

func (s *ItemsWiringSuite) TestGetItem_MissingParam() {
	s.StartServer()

	tool := s.tool("get_item")
	result := s.callTool(tool, map[string]any{})

	s.True(result.IsError)
}

func (s *ItemsWiringSuite) TestGetItem_NotFound() {
	s.StartServer()

	tool := s.tool("get_item")
	result := s.callTool(tool, map[string]any{"itemId": "nonexistent"})

	s.True(result.IsError)
}

func (s *ItemsWiringSuite) TestGetItemsInProgress() {
	s.routes["/api/me/items-in-progress"] = map[string]any{
		"libraryItems": []map[string]any{
			abstest.BookItem("item-1", "Katabasis", "R.F. Kuang", 43200.0),
		},
	}
	s.routes["/api/me/progress/item-1"] = abstest.Progress("item-1", "item-1", 43200.0, 0.25, 10800.0, false)
	s.StartServer()

	tool := s.tool("get_items_in_progress")
	result := s.callTool(tool, map[string]any{})

	var items []inProgressItem
	s.resultJSON(result, &items)

	s.Require().Len(items, 1)
	s.Equal("Katabasis", items[0].Title)
	s.Equal("25.0%", items[0].Progress)
	s.Equal("9h 0m", items[0].TimeRemaining)
}

func (s *ItemsWiringSuite) TestGetItemsInProgress_Empty() {
	s.routes["/api/me/items-in-progress"] = map[string]any{
		"libraryItems": []map[string]any{},
	}
	s.StartServer()

	tool := s.tool("get_items_in_progress")
	result := s.callTool(tool, map[string]any{})

	var items []inProgressItem
	s.resultJSON(result, &items)
	s.Empty(items)
}
