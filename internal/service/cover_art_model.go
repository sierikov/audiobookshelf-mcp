package service

import (
	"errors"
	"fmt"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
	"github.com/sierikov/audiobookshelf-mcp/internal/gemini"
)

// MaxNewCovers is the most covers one call of [CoverArt.Generate] makes. A
// larger series takes several calls, each one asked for and seen.
const MaxNewCovers = 4

// SourceGemini is the source of the covers [CoverArt] makes: Google's Gemini
// image model.
const SourceGemini = "gemini"

// CoverModel says which image model makes a cover, by what the caller gets
// from it: the best finish, or the lower price.
type CoverModel string

// The image models a cover can be made with. The empty CoverModel is
// [CoverModelPro].
const (
	// CoverModelPro is Nano Banana Pro: 2048 px, the best finish and
	// lettering, about $0.16 a cover.
	CoverModelPro CoverModel = "pro"
	// CoverModelStandard is Nano Banana 2.1: 1024 px, about $0.07 a cover.
	CoverModelStandard CoverModel = "standard"
)

// imageModel is the image model m names, and the empty one is Pro. Any other
// value is an [ErrUnknownModel] error.
func (m CoverModel) imageModel() (gemini.Model, error) {
	switch m {
	case "", CoverModelPro:
		return gemini.ModelPro, nil
	case CoverModelStandard:
		return gemini.ModelStandard, nil
	}
	return "", ErrUnknownModel
}

// NewCoverRequest asks for one new cover of the book ItemID, drawn in Style,
// such as "moody watercolour, muted palette", which is required. Premise is
// one or two sentences on the setting and the opening situation, and may be
// empty. Title is the title text to show on the cover in place of the book's
// title in Audiobookshelf, such as another translation's title, and may be
// empty; the author stays.
type NewCoverRequest struct {
	ItemID  string
	Style   string
	Premise string
	Title   string
}

// GenerateRequest asks [CoverArt.Generate] for 1 to [MaxNewCovers] new covers,
// for one book in several styles or for several books in one, all made with
// Model. TitleText says whether the title and the author are on the covers as
// text. At most one of the references is given: ReferenceImageID, a cover made
// earlier, or ReferenceItemID, a book whose current cover is used; every cover
// then matches the style of that image, so a series stays consistent.
type GenerateRequest struct {
	Covers           []NewCoverRequest
	Model            CoverModel
	TitleText        bool
	ReferenceImageID string
	ReferenceItemID  string
}

// CurrentCover is a book's cover as it is, to show next to the new ones. Image
// is nil when the book has no cover, or when Audiobookshelf could not read it:
// then Problem says so, in a short phrase of our own, and is "" for a book
// without a cover.
type CurrentCover struct {
	ItemID  string
	Title   string
	Image   *abs.Image
	Problem string
}

// NewCover is a cover made for the book ItemID, in Style, which is "" for a
// cleaned cover. Title is the book's title in Audiobookshelf, whatever title
// text the cover shows. ImageID names the full image, which [CoverArt.Set] uploads,
// and Preview is a small copy of it (see [PreviewWidth] and
// [PreviewMaxHeight]). A cover that was not made, or cannot be shown, has
// neither: Problem says why, in a short phrase of our own.
type NewCover struct {
	ItemID  string
	Title   string
	Style   string
	ImageID string
	Problem string
	Preview *abs.Image
}

// Spend is what a [CoverArt] has made since the server started: Images is the
// number of images made and EstimatedUSD what all its requests cost, refusals
// included, in US dollars, each at the prices of its own model. Google bills by
// tokens, so the cost is an estimate.
type Spend struct {
	Images       int
	EstimatedUSD float64
}

// CoverArtResult is the outcome of making covers: the current cover of each
// book, once per book in the order of the request, then the new covers in the
// order asked. Stopped is set when Google refused more images, such as at a
// spending limit: it says why, and that nothing is retried. Spent is the spend
// so far, this call's included.
type CoverArtResult struct {
	Current []CurrentCover
	Covers  []NewCover
	Stopped string
	Spent   Spend
}

// Errors returned by the CoverArt service. They are wrapped with details, so
// match them with [errors.Is].
var (
	ErrNewCoverCount     = fmt.Errorf("give 1 to %d covers to make", MaxNewCovers)
	ErrUnknownImage      = errors.New("no such made cover: made covers are kept for an hour, and none survive a restart")
	ErrImageForOtherBook = errors.New("that cover was made for another book")
	ErrOneReference      = errors.New("give referenceImageId or referenceItemId, not both")
	ErrNoCoverToClean    = errors.New("the book has no cover to clean")
	ErrNoReferenceCover  = errors.New("the reference book has no cover")
	ErrUnknownModel      = errors.New(`model must be "pro" or "standard"`)
	// ErrNoCoverArt is the error for asking for a made cover from a server
	// that has no Gemini API key, and so no CoverArt service.
	ErrNoCoverArt = errors.New("made covers need a Gemini API key: run audiobookshelf-mcp setup")
)
