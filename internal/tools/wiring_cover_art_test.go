package tools

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
	"github.com/sierikov/audiobookshelf-mcp/internal/abs/abstest"
	"github.com/sierikov/audiobookshelf-mcp/internal/gemini"
	"github.com/sierikov/audiobookshelf-mcp/internal/service"
)

type CoverArtWiringSuite struct {
	wiringSuite
}

func TestCoverArtSuite(t *testing.T) {
	suite.Run(t, new(CoverArtWiringSuite))
}

// madePNG is the image the fake Gemini makes. It is small, so its preview is
// the image itself, and it is a whole PNG, which the service reads the size of.
var madePNG = func() []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 8, 12))); err != nil {
		panic(err)
	}
	return buf.Bytes()
}()

// generateContentPath is where Google answers a request for an image made by
// model.
func generateContentPath(model gemini.Model) string {
	return "/v1/models/" + string(model) + ":generateContent"
}

// geminiFake is a fake Gemini API: it counts the requests, keeps the key, the
// path and the body of the last one, and answers each as handle says.
type geminiFake struct {
	*httptest.Server
	requests atomic.Int32
	keys     atomic.Value // the x-goog-api-key of the last request
	paths    atomic.Value // the path of the last request
	bodies   atomic.Value // the body of the last request, a string
}

// fakeGemini starts a fake Gemini API for the length of the test. It answers
// only a request to generateContent of one of the two models; anything else is
// a 404.
func (s *CoverArtWiringSuite) fakeGemini(handle http.HandlerFunc) *geminiFake {
	g := &geminiFake{}
	g.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isModel := r.URL.Path == generateContentPath(gemini.ModelPro) || r.URL.Path == generateContentPath(gemini.ModelStandard)
		if r.Method != http.MethodPost || !isModel {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		g.requests.Add(1)
		g.keys.Store(r.Header.Get("x-goog-api-key"))
		g.paths.Store(r.URL.Path)
		g.bodies.Store(string(body))
		handle(w, r)
	}))
	s.T().Cleanup(g.Close)
	return g
}

// makesImage answers a request with madePNG and the usage of one image made.
func makesImage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"candidates": []any{map[string]any{
			"content": map[string]any{"parts": []any{map[string]any{
				"inlineData": map[string]any{"mimeType": "image/png", "data": base64.StdEncoding.EncodeToString(madePNG)},
			}}},
			"finishReason": "STOP",
		}},
		"usageMetadata": map[string]any{"promptTokenCount": 40, "candidatesTokenCount": 1120, "thoughtsTokenCount": 300},
	})
}

// runsOutOfQuota answers a request as Google does at a spending limit.
func runsOutOfQuota(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_, _ = w.Write([]byte(`{"error":{"code":429,"message":"Spending cap exceeded.","status":"RESOURCE_EXHAUSTED"}}`))
}

// startArt starts the fake Audiobookshelf and wires the real services to it,
// with the made-cover service on the fake Gemini at g.
func (s *CoverArtWiringSuite) startArt(g *geminiFake) {
	s.StartServer()
	client := abs.NewClient(s.srv.URL, abstest.Token)
	s.svcs.CoverArt = service.NewCoverArt(client, gemini.NewClient(g.URL, "k"))
}

// hollyRoutes serves the book "Holly" with a current cover, which the
// service reads, and accepts a new cover.
func (s *CoverArtWiringSuite) hollyRoutes() {
	s.routes["/api/items/li_1"] = abstest.BookItem("li_1", "Holly", "Stephen King", 3600)
	s.routes["/api/items/li_1/cover"] = abstest.Raw{ContentType: "image/jpeg", Body: jpegBytes}
}

// madeCover is a cover of the made-cover result, as the text block has it.
type madeCover struct {
	Index   int    `json:"index"`
	ItemID  string `json:"itemId"`
	Title   string `json:"title"`
	Style   string `json:"style"`
	ImageID string `json:"imageId"`
	Problem string `json:"problem"`
}

// madeResult is the text block of a made-cover result.
type madeResult struct {
	Covers  []madeCover `json:"covers"`
	Stopped string      `json:"stopped"`
}

func (s *CoverArtWiringSuite) TestGenerateCovers_FullStack() {
	g := s.fakeGemini(makesImage)
	s.hollyRoutes()
	s.startArt(g)

	result := s.callTool(s.tool("generate_covers"), map[string]any{"covers": []any{
		map[string]any{"itemId": "li_1", "style": "moody watercolour, muted palette", "premise": "A town in winter."},
	}})

	s.Require().False(result.IsError, resultText(result))
	s.Require().Len(result.Content, 3, "the text block, the current cover and the made one")
	var out madeResult
	s.resultJSON(result, &out)
	s.Require().Len(out.Covers, 1)
	s.Regexp(`^gen_[0-9a-f]{16}$`, out.Covers[0].ImageID)
	s.JSONEq(`{"untrusted":true,"source":"gemini",
		"current":[{"itemId":"li_1","title":"Holly","shown":true}],
		"covers":[{"index":1,"itemId":"li_1","title":"Holly","style":"moody watercolour, muted palette","imageId":"`+out.Covers[0].ImageID+`"}],
		"spent":{"images":1,"estimatedUSD":"0.14","note":"made by this server since it started; Google bills by tokens, so this is an estimate"},
		"images":"image 1 is the current cover of Holly; then new cover 1"}`, resultText(result))

	images := result.Content[1:]
	current, ok := images[0].(*mcp.ImageContent)
	s.Require().True(ok, "the current cover is an image")
	s.Equal(jpegBytes, current.Data)
	s.Equal("image/jpeg", current.MIMEType)
	made, ok := images[1].(*mcp.ImageContent)
	s.Require().True(ok, "the made cover is an image")
	s.Equal(madePNG, made.Data, "an image this small is its own preview")
	s.Equal("image/png", made.MIMEType)
	s.Equal([]mcp.Role{"user", "assistant"}, made.Annotations.Audience)
	s.Nil(result.StructuredContent, "Codex hides the images of a result with structured content")

	s.EqualValues(1, g.requests.Load())
	s.Equal("k", g.keys.Load(), "the key goes to Google, in the header")
	s.Equal(generateContentPath(gemini.ModelPro), g.paths.Load(), "no model asked for is pro")
	s.Contains(g.bodies.Load(), `"imageSize":"2K"`)
}

func (s *CoverArtWiringSuite) TestGenerateCovers_ModelAndTitleFullStack() {
	g := s.fakeGemini(makesImage)
	s.hollyRoutes()
	s.startArt(g)

	result := s.callTool(s.tool("generate_covers"), map[string]any{
		"model": "standard",
		"covers": []any{
			map[string]any{"itemId": "li_1", "style": "moody watercolour", "title": "Холли"},
		},
	})

	s.Require().False(result.IsError, resultText(result))
	var out madeResult
	s.resultJSON(result, &out)
	s.Require().Len(out.Covers, 1)
	s.Equal("Holly", out.Covers[0].Title, "the result keeps the library title")
	s.Equal(generateContentPath(gemini.ModelStandard), g.paths.Load())
	s.Contains(g.bodies.Load(), `"imageSize":"1K"`)
	s.Contains(g.bodies.Load(), `Front cover for the audiobook \"Холли\" by Stephen King.`, "the title text is in the prompt in place of the library title")
	s.NotContains(g.bodies.Load(), "Holly")
	s.Contains(resultText(result), `"spent":{"images":1,"estimatedUSD":"0.04"`, "at the standard model's prices")
}

func (s *CoverArtWiringSuite) TestSetCover_MadeCoverFullStack() {
	g := s.fakeGemini(makesImage)
	s.hollyRoutes()
	s.startArt(g)
	generated := s.callTool(s.tool("generate_covers"), map[string]any{"covers": []any{map[string]any{"itemId": "li_1", "style": "noir"}}})
	var out madeResult
	s.resultJSON(generated, &out)
	s.Require().Len(out.Covers, 1)

	result := s.callTool(s.tool("set_cover"), map[string]any{"itemId": "li_1", "imageId": out.Covers[0].ImageID})

	s.Require().False(result.IsError, resultText(result))
	s.JSONEq(`{"itemId":"li_1","title":"Holly"}`, resultText(result))
	// The upload is a multipart POST to the book's cover, holding the made image's bytes.
	upload := s.srv.LastRequest()
	s.Equal(http.MethodPost, upload.Method)
	s.Equal("/api/items/li_1/cover", upload.URL.Path)
	mediaType, params, err := mime.ParseMediaType(upload.Header.Get("Content-Type"))
	s.Require().NoError(err)
	s.Equal("multipart/form-data", mediaType)
	form := multipart.NewReader(bytes.NewReader(s.srv.LastBodyFor("/api/items/li_1/cover")), params["boundary"])
	part, err := form.NextPart()
	s.Require().NoError(err)
	s.Equal("cover", part.FormName())
	s.Equal("cover.png", part.FileName())
	body, err := io.ReadAll(part)
	s.Require().NoError(err)
	s.Equal(madePNG, body, "the made image itself, not its preview")
	_, err = form.NextPart()
	s.Equal(io.EOF, err, "the cover is the only part")
}

func (s *CoverArtWiringSuite) TestSetCover_ImageForAnotherBookFullStack() {
	g := s.fakeGemini(makesImage)
	s.hollyRoutes()
	s.routes["/api/items/li_2"] = abstest.BookItem("li_2", "If It Bleeds", "Stephen King", 3600)
	s.startArt(g)
	generated := s.callTool(s.tool("generate_covers"), map[string]any{"covers": []any{map[string]any{"itemId": "li_1", "style": "noir"}}})
	var out madeResult
	s.resultJSON(generated, &out)

	result := s.callTool(s.tool("set_cover"), map[string]any{"itemId": "li_2", "imageId": out.Covers[0].ImageID})

	assertToolError(s.T(), `failed to set cover: imageId "`+out.Covers[0].ImageID+`": `+service.ErrImageForOtherBook.Error(), result)
	s.NotEqual("/api/items/li_2/cover", s.srv.LastRequest().URL.Path, "nothing is uploaded")
}

func (s *CoverArtWiringSuite) TestSetCover_UnknownImageFullStack() {
	g := s.fakeGemini(makesImage)
	s.hollyRoutes()
	s.startArt(g)

	result := s.callTool(s.tool("set_cover"), map[string]any{"itemId": "li_1", "imageId": "gen_0123456789abcdef"})

	s.True(result.IsError)
	s.Contains(resultText(result), "failed to set cover: imageId")
	s.Contains(resultText(result), service.ErrUnknownImage.Error())
	s.Nil(s.srv.LastRequest(), "nothing is asked of Audiobookshelf, and nothing is uploaded")
}

func (s *CoverArtWiringSuite) TestSetCover_ImageIDWithoutAKeyFullStack() {
	s.hollyRoutes()
	s.StartServer() // no CoverArt, as without a Gemini key

	result := s.callTool(s.tool("set_cover"), map[string]any{"itemId": "li_1", "imageId": "gen_0123456789abcdef"})

	assertToolError(s.T(), "failed to set cover: made covers need a Gemini API key: run audiobookshelf-mcp setup", result)
	s.Nil(s.srv.LastRequest())
	s.Nil(s.tool("generate_covers"), "the paid tools are not there without a key")
	s.Nil(s.tool("clean_cover"))
}

func (s *CoverArtWiringSuite) TestGenerateCovers_QuotaStopsTheCallFullStack() {
	g := s.fakeGemini(runsOutOfQuota)
	s.hollyRoutes()
	s.startArt(g)

	result := s.callTool(s.tool("generate_covers"), map[string]any{"covers": []any{map[string]any{"itemId": "li_1", "style": "noir"}}})

	s.False(result.IsError, "a stopped call is a result: what was made before is in it")
	var out madeResult
	s.resultJSON(result, &out)
	s.NotEmpty(out.Stopped)
	s.Contains(out.Stopped, "Google refused more images (HTTP 429 RESOURCE_EXHAUSTED)")
	s.Contains(out.Stopped, "nothing is retried")
	s.Require().Len(out.Covers, 1)
	s.Empty(out.Covers[0].ImageID)
	s.Equal("not made: the call stopped", out.Covers[0].Problem)
	s.Contains(resultText(result), `"spent":{"images":0,"estimatedUSD":"0.00"`)
	s.Len(imageBlocks(s.T(), result), 1, "the current cover, which is still worth showing")
	s.EqualValues(1, g.requests.Load())
}

func (s *CoverArtWiringSuite) TestCleanCover_FullStack() {
	g := s.fakeGemini(makesImage)
	s.hollyRoutes()
	s.startArt(g)

	result := s.callTool(s.tool("clean_cover"), map[string]any{"itemId": "li_1", "instructions": "remove the store label at the bottom"})

	s.Require().False(result.IsError, resultText(result))
	s.Require().Len(result.Content, 3, "the text block, the current cover and the cleaned one")
	var out madeResult
	s.resultJSON(result, &out)
	s.Require().Len(out.Covers, 1)
	s.JSONEq(`{"untrusted":true,"source":"gemini",
		"current":[{"itemId":"li_1","title":"Holly","shown":true}],
		"covers":[{"index":1,"itemId":"li_1","title":"Holly","imageId":"`+out.Covers[0].ImageID+`"}],
		"spent":{"images":1,"estimatedUSD":"0.14","note":"made by this server since it started; Google bills by tokens, so this is an estimate"},
		"images":"image 1 is the current cover of Holly; then new cover 1"}`, resultText(result))
	s.NotContains(resultText(result), `"style"`, "a cleaned cover has no style")
	s.Equal(generateContentPath(gemini.ModelPro), g.paths.Load(), "no model asked for is pro")
	// Every cleaned cover is square.
	s.Contains(g.bodies.Load(), `"aspectRatio":"1:1"`)
	s.Contains(g.bodies.Load(), "Make the cover square, extending the background so that nothing is cut off or stretched.")
}

// bookInABookShapedLibrary serves the book Holly in a library whose cover shape
// setting is 0, book-shaped, which Audiobookshelf offers and this server
// ignores: audiobook covers are square.
func (s *CoverArtWiringSuite) bookInABookShapedLibrary() {
	s.hollyRoutes()
	item := abstest.BookItem("li_1", "Holly", "Stephen King", 3600)
	item["libraryId"] = "lib_1"
	s.routes["/api/items/li_1"] = item
	s.routes["/api/libraries/lib_1"] = map[string]any{"id": "lib_1", "name": "Audiobooks", "settings": map[string]any{"coverAspectRatio": 0}}
}

func (s *CoverArtWiringSuite) TestCleanCover_IsSquareWhateverTheLibraryFullStack() {
	g := s.fakeGemini(makesImage)
	s.bookInABookShapedLibrary()
	s.startArt(g)

	result := s.callTool(s.tool("clean_cover"), map[string]any{"itemId": "li_1", "instructions": "remove the store label at the bottom"})

	s.Require().False(result.IsError, resultText(result))
	s.Contains(g.bodies.Load(), `"aspectRatio":"1:1"`, "square, though the library says book-shaped")
	s.Contains(g.bodies.Load(), "Make the cover square, extending the background so that nothing is cut off or stretched.")
	s.NotContains(g.bodies.Load(), "portrait")
	s.Nil(s.srv.LastBodyFor("/api/libraries/lib_1"), "the library is not read")
}

func (s *CoverArtWiringSuite) TestGenerateCovers_IsSquareWhateverTheLibraryFullStack() {
	g := s.fakeGemini(makesImage)
	s.bookInABookShapedLibrary()
	s.startArt(g)

	result := s.callTool(s.tool("generate_covers"), map[string]any{"covers": []any{map[string]any{"itemId": "li_1", "style": "noir"}}})

	s.Require().False(result.IsError, resultText(result))
	s.Contains(g.bodies.Load(), `"aspectRatio":"1:1"`, "square, though the library says book-shaped")
	s.Nil(s.srv.LastBodyFor("/api/libraries/lib_1"), "the library is not read")
}

func (s *CoverArtWiringSuite) TestCleanCover_StandardModelFullStack() {
	g := s.fakeGemini(makesImage)
	s.hollyRoutes()
	s.startArt(g)

	result := s.callTool(s.tool("clean_cover"), map[string]any{
		"itemId": "li_1", "instructions": "remove the store label at the bottom", "model": "standard",
	})

	s.Require().False(result.IsError, resultText(result))
	s.Equal(generateContentPath(gemini.ModelStandard), g.paths.Load())
	s.Contains(g.bodies.Load(), `"imageSize":"1K"`)
	s.Contains(resultText(result), `"spent":{"images":1,"estimatedUSD":"0.04"`, "at the standard model's prices")
}

func (s *CoverArtWiringSuite) TestSpendCountsAcrossCalls() {
	g := s.fakeGemini(makesImage)
	s.hollyRoutes()
	s.startArt(g)
	args := map[string]any{"covers": []any{map[string]any{"itemId": "li_1", "style": "noir"}}}

	s.callTool(s.tool("generate_covers"), args)
	result := s.callTool(s.tool("generate_covers"), args)

	s.Contains(resultText(result), `"spent":{"images":2,"estimatedUSD":"0.28"`, "everything this server has made since it started")
}
