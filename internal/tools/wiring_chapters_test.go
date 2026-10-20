package tools

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs/abstest"
)

type ChaptersWiringSuite struct {
	wiringSuite
}

func TestChaptersSuite(t *testing.T) {
	suite.Run(t, new(ChaptersWiringSuite))
}

func (s *ChaptersWiringSuite) TestSetChapters_FullStack() {
	s.routes["/api/items/item-1"] = abstest.BookWithChapters("item-1", "Dune", "Frank Herbert", "", 3600,
		[]any{abstest.Chapter(0, 0, 3600, "Chapter 1")},
		[]any{abstest.Track(1, "m001.mp3", 0, 3600)},
	)
	s.routes["/api/items/item-1/chapters"] = map[string]any{"success": true, "updated": true}
	s.StartServer()

	result := s.callTool(s.tool("set_chapters"), map[string]any{
		"itemId": "item-1",
		"chapters": []any{
			map[string]any{"start": 0.0, "title": "Opening"},
			map[string]any{"start": 600.0, "title": "One"},
		},
	})

	s.Require().False(result.IsError, resultText(result))
	s.JSONEq(`{"chapters":[
		{"id":0,"start":0,"end":600,"title":"Opening"},
		{"id":1,"start":600,"end":3600,"title":"One"}
	]}`, string(s.srv.LastBody()))

	var out changeOutput
	s.resultJSON(result, &out)
	s.True(out.Updated)
	s.Require().Len(out.Previous, 1)
	s.Equal("Chapter 1", out.Previous[0].Title)
	s.Len(out.Chapters, 2)
}
