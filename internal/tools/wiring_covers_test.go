package tools

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs/abstest"
	"github.com/sierikov/audiobookshelf-mcp/internal/service"
)

type CoversWiringSuite struct {
	wiringSuite
}

func TestCoversSuite(t *testing.T) {
	suite.Run(t, new(CoversWiringSuite))
}

// jpegBytes is a small JPEG: the service reads the size of a candidate's image,
// so it must be a whole one, and as it is small it comes back unchanged.
var jpegBytes = func() []byte {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewGray(image.Rect(0, 0, 4, 6)), nil); err != nil {
		panic(err)
	}
	return buf.Bytes()
}()

// imageServer serves a JPEG at every path, as a cover provider's image host.
func (s *CoversWiringSuite) imageServer() *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(jpegBytes)
	}))
	s.T().Cleanup(srv.Close)
	return srv
}

func (s *CoversWiringSuite) TestSearchCovers_FullStack() {
	images := s.imageServer()
	s.routes["/api/items/li_1"] = abstest.BookItem("li_1", "Holly", "Stephen King", 3600)
	s.routes["/api/items/li_1/cover"] = abstest.Raw{ContentType: "image/jpeg", Body: jpegBytes}
	s.routes["/api/search/covers"] = map[string]any{"results": []any{images.URL + "/holly.jpg"}}
	s.StartServer()

	result := s.callTool(s.tool("search_covers"), map[string]any{"itemId": "li_1"})

	s.Require().False(result.IsError, resultText(result))
	s.Require().Len(result.Content, 3, "one text block and two images")
	s.JSONEq(`{"untrusted":true,"source":"google","book":{"itemId":"li_1","title":"Holly","author":"Stephen King"},
		"current":true,"candidates":[{"index":1,"url":"`+images.URL+`/holly.jpg"}],
		"images":"image 1 is the current cover; then candidate 1"}`, resultText(result))
	for i, c := range result.Content[1:] {
		img, ok := c.(*mcp.ImageContent)
		s.Require().True(ok, "content %d is an image", i+1)
		s.Equal(jpegBytes, img.Data)
		s.Equal("image/jpeg", img.MIMEType)
		s.Equal([]mcp.Role{"user", "assistant"}, img.Annotations.Audience)
	}
	// The search is the last call to Audiobookshelf; the images come from the image host.
	search := s.srv.LastRequest()
	s.Equal("/api/search/covers", search.URL.Path)
	s.Equal("google", search.URL.Query().Get("provider"), "the provider defaults to google")
}

func (s *CoversWiringSuite) TestSearchCovers_BookWithoutCoverFullStack() {
	images := s.imageServer()
	s.routes["/api/items/li_1"] = abstest.BookItem("li_1", "Holly", "Stephen King", 3600)
	s.routes["/api/search/covers"] = map[string]any{"results": []any{images.URL + "/holly.jpg", images.URL + "/holly-2.jpg"}}
	s.StartServer()

	result := s.callTool(s.tool("search_covers"), map[string]any{"itemId": "li_1", "provider": "audible"})

	s.Require().False(result.IsError, resultText(result))
	s.Len(result.Content, 3, "the text block and both candidates; no cover is not an error")
	s.Contains(resultText(result), `"current":false`)
	s.Contains(resultText(result), `"source":"audible"`)
}

func (s *CoversWiringSuite) TestSearchCovers_CurrentCoverThatCannotBeReadFullStack() {
	images := s.imageServer()
	s.routes["/api/items/li_1"] = abstest.BookItem("li_1", "Holly", "Stephen King", 3600)
	s.routes["/api/items/li_1/cover"] = abstest.Raw{
		Status: http.StatusInternalServerError, ContentType: "text/plain", Body: []byte("secret-detail"),
	}
	s.routes["/api/search/covers"] = map[string]any{"results": []any{images.URL + "/holly.jpg"}}
	s.StartServer()

	result := s.callTool(s.tool("search_covers"), map[string]any{"itemId": "li_1"})

	s.Require().False(result.IsError, resultText(result))
	s.Require().Len(result.Content, 2, "the text block and the one candidate")
	s.JSONEq(`{"untrusted":true,"source":"google","book":{"itemId":"li_1","title":"Holly","author":"Stephen King"},
		"current":false,"currentProblem":"the current cover could not be read (HTTP 500)",
		"candidates":[{"index":1,"url":"`+images.URL+`/holly.jpg"}],
		"images":"the current cover could not be read; the image is candidate 1"}`, resultText(result))
	s.NotContains(resultText(result), "secret-detail")
}

func (s *CoversWiringSuite) TestSearchCovers_URLsFullStack() {
	images := s.imageServer()
	s.routes["/api/items/li_1"] = abstest.BookItem("li_1", "Holly", "Stephen King", 3600)
	s.routes["/api/items/li_1/cover"] = abstest.Raw{ContentType: "image/jpeg", Body: jpegBytes}
	s.StartServer()

	found := images.URL + "/found.jpg"
	result := s.callTool(s.tool("search_covers"), map[string]any{"itemId": "li_1", "urls": []any{found}})

	s.Require().False(result.IsError, resultText(result))
	s.Require().Len(result.Content, 3, "one text block, the current cover and the cover found")
	s.JSONEq(`{"untrusted":true,"source":"web","book":{"itemId":"li_1","title":"Holly","author":"Stephen King"},
		"current":true,"candidates":[{"index":1,"url":"`+found+`"}],
		"images":"image 1 is the current cover; then candidate 1"}`, resultText(result))
	// No search: the cover is the last call to Audiobookshelf.
	s.Equal("/api/items/li_1/cover", s.srv.LastRequest().URL.Path)
}

func (s *CoversWiringSuite) TestSearchCovers_TooManyURLsFullStack() {
	s.StartServer()
	urls := make([]any, 6)
	for i := range urls {
		urls[i] = fmt.Sprintf("https://img.example/%d.jpg", i)
	}

	result := s.callTool(s.tool("search_covers"), map[string]any{"itemId": "li_1", "urls": urls})

	// The schema allows at most 5 before the handler runs; the service checks too.
	s.True(result.IsError)
	s.Equal("cover search failed: give 1 to 5 cover URLs, not 6", resultText(result))
}

func (s *CoversWiringSuite) TestSetCover_FullStack() {
	s.routes["/api/items/li_1"] = abstest.BookItem("li_1", "Holly", "Stephen King", 3600)
	s.routes["/api/items/li_1/cover"] = map[string]any{"success": true, "cover": "/metadata/items/li_1/cover.jpg"}
	s.StartServer()

	result := s.callTool(s.tool("set_cover"), map[string]any{"itemId": "li_1", "url": "https://img.example/holly.jpg"})

	s.Require().False(result.IsError, resultText(result))
	s.JSONEq(`{"url":"https://img.example/holly.jpg"}`, string(s.srv.LastBodyFor("/api/items/li_1/cover")))
	s.JSONEq(`{"itemId":"li_1","title":"Holly"}`, resultText(result))
}

func (s *CoversWiringSuite) TestSetCover_ForbiddenFullStack() {
	s.routes["/api/items/li_1"] = abstest.BookItem("li_1", "Holly", "Stephen King", 3600)
	s.routes["/api/items/li_1/cover"] = abstest.Raw{
		Status: http.StatusForbidden, ContentType: "text/html", Body: []byte("<html><body>Forbidden</body></html>"),
	}
	s.StartServer()

	result := s.callTool(s.tool("set_cover"), map[string]any{"itemId": "li_1", "url": "https://img.example/holly.jpg"})

	assertToolError(s.T(), "failed to set cover: the Audiobookshelf user needs the Update and Upload permissions", result)
}

func (s *CoversWiringSuite) TestGetCoverPrompt_FullStack() {
	item := abstest.BookItemFull("li_1", map[string]any{
		"title": "Holly", "authorName": "Stephen King", "genres": []any{"Horror", "Thriller"},
	}, 3600)
	item["libraryId"] = "lib_1"
	s.routes["/api/items/li_1"] = item
	// A book-shaped library, which this server ignores: audiobook covers are square.
	s.routes["/api/libraries/lib_1"] = map[string]any{"id": "lib_1", "name": "Audiobooks", "settings": map[string]any{"coverAspectRatio": 0}}
	s.StartServer()

	result := s.callTool(s.tool("get_cover_prompt"), map[string]any{"itemId": "li_1", "style": "Moody watercolour", "premise": "A town in winter."})

	s.Require().False(result.IsError, resultText(result))
	var out struct {
		Prompt, AspectRatio, BookPage string
		Edit                          bool
	}
	s.resultJSON(result, &out)
	s.Equal("1:1", out.AspectRatio, "always square")
	s.Equal(s.srv.URL+"/item/li_1", out.BookPage, "the page of the book on the server the client talks to")
	s.False(out.Edit)
	s.Contains(out.Prompt, "Genre: Horror, Thriller.\nPremise: A town in winter.\nStyle: Moody watercolour\n")
	s.True(strings.HasSuffix(out.Prompt, "\nThe cover is square."))
	// The item only: no library is read, and no image is fetched.
	s.Equal("/api/items/li_1", s.srv.LastRequest().URL.Path)
	s.Nil(s.srv.LastBodyFor("/api/libraries/lib_1"))
}

func (s *CoversWiringSuite) TestGetCoverPrompt_CleanFullStack() {
	item := abstest.BookItem("li_1", "Holly", "Stephen King", 3600)
	item["libraryId"] = "lib_1"
	s.routes["/api/items/li_1"] = item
	s.routes["/api/libraries/lib_1"] = map[string]any{"id": "lib_1", "name": "Audiobooks", "settings": map[string]any{"coverAspectRatio": 0}}
	s.StartServer()

	result := s.callTool(s.tool("get_cover_prompt"), map[string]any{"itemId": "li_1", "instructions": "remove the store label at the bottom"})

	s.Require().False(result.IsError, resultText(result))
	var out struct {
		Prompt, AspectRatio string
		Edit                bool
	}
	s.resultJSON(result, &out)
	s.True(out.Edit)
	s.Equal("1:1", out.AspectRatio, "always square")
	s.True(strings.HasPrefix(out.Prompt, "Edit this audiobook cover: remove the store label at the bottom\n"))
	s.True(strings.HasSuffix(out.Prompt, "\nMake the cover square, extending the background so that nothing is cut off or stretched."))
	s.NotContains(out.Prompt, "The cover is", "the edit line takes the place of the shape line")
	// The item only: the library is not read.
	s.Equal("/api/items/li_1", s.srv.LastRequest().URL.Path)
	s.Nil(s.srv.LastBodyFor("/api/libraries/lib_1"))
}

func (s *CoversWiringSuite) TestGetCoverPrompt_BothStyleAndInstructionsFullStack() {
	s.StartServer()

	result := s.callTool(s.tool("get_cover_prompt"), map[string]any{"itemId": "li_1", "style": "noir", "instructions": "remove the label"})

	assertToolError(s.T(), "failed to write the cover prompt: "+service.ErrStyleOrInstructions.Error(), result)
}
