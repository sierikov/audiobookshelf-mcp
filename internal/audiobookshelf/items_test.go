package audiobookshelf

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ItemsSuite struct {
	ToolSuite
}

func TestItemsSuite(t *testing.T) {
	suite.Run(t, new(ItemsSuite))
}

func (s *ItemsSuite) TestGetItem_Book() {
	s.routes["/api/items/item-1"] = bookItemFull("item-1", map[string]any{
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

	tool := findTool(ItemTools(s.client), "get_item")
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

func (s *ItemsSuite) TestGetItem_Podcast() {
	s.routes["/api/items/pod-1"] = podcastItem("pod-1", "Hardcore History", "Dan Carlin", 68)
	s.StartServer()

	tool := findTool(ItemTools(s.client), "get_item")
	result := s.callTool(tool, map[string]any{"itemId": "pod-1"})

	var d itemDetail
	s.resultJSON(result, &d)

	s.Equal("podcast", d.MediaType)
	s.Equal("Hardcore History", d.Title)
	s.Equal(68, d.NumEpisodes)
}

func (s *ItemsSuite) TestGetItem_MissingParam() {
	s.StartServer()

	tool := findTool(ItemTools(s.client), "get_item")
	result := s.callTool(tool, map[string]any{})

	s.True(result.IsError)
}

func (s *ItemsSuite) TestGetItem_NotFound() {
	s.StartServer()

	tool := findTool(ItemTools(s.client), "get_item")
	result := s.callTool(tool, map[string]any{"itemId": "nonexistent"})

	s.True(result.IsError)
}

func (s *ItemsSuite) TestGetItemsInProgress() {
	s.routes["/api/me/items-in-progress"] = map[string]any{
		"libraryItems": []map[string]any{
			bookItem("item-1", "Katabasis", "R.F. Kuang", 43200.0),
		},
	}
	s.routes["/api/me/progress/item-1"] = progress("item-1", "item-1", 43200.0, 0.25, 10800.0, false)
	s.StartServer()

	tool := findTool(ItemTools(s.client), "get_items_in_progress")
	result := s.callTool(tool, map[string]any{})

	var items []inProgressItem
	s.resultJSON(result, &items)

	s.Require().Len(items, 1)
	s.Equal("Katabasis", items[0].Title)
	s.Equal("25.0%", items[0].Progress)
	s.Equal("9h 0m", items[0].TimeRemaining)
}

func (s *ItemsSuite) TestGetItemsInProgress_Empty() {
	s.routes["/api/me/items-in-progress"] = map[string]any{
		"libraryItems": []map[string]any{},
	}
	s.StartServer()

	tool := findTool(ItemTools(s.client), "get_items_in_progress")
	result := s.callTool(tool, map[string]any{})

	var items []inProgressItem
	s.resultJSON(result, &items)
	s.Empty(items)
}
