package service

import (
	"errors"
	"fmt"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
)

// CoverSearch is the result of looking for covers for a book: the book, the
// provider searched ([WebSource] for covers found elsewhere), the book's
// current cover and the covers found. Current
// is nil when the book has no cover, or when Audiobookshelf could not read it:
// then CurrentProblem says so, in a short phrase of our own, and is "" for a
// book without a cover.
type CoverSearch struct {
	ItemID         string
	Title          string
	Author         string
	Provider       string
	Current        *abs.Image
	CurrentProblem string
	Candidates     []CoverCandidate
}

// CoverCandidate is a cover a provider found, in the provider's order, or one
// given to [Covers.Show], in the order given. URL is what the provider or the
// caller gave, untrusted text, unless it is not a web URL: then it
// is quoted and cut as [quoted] does, and cannot be used. Image is a small
// preview of the cover (see [PreviewWidth] and [PreviewMaxHeight]), nil when
// the cover could not be fetched or shown, and then Problem says why in a
// short phrase of our own, never the error text of the failed request.
type CoverCandidate struct {
	URL     string
	Image   *abs.Image
	Problem string
}

// CoverSet is the outcome of setting a book's cover.
type CoverSet struct {
	ItemID string
	Title  string
}

// WebSource is the provider of a [CoverSearch] made by [Covers.Show], whose
// covers were found on the web rather than by a provider.
const WebSource = "web"

// Limits on the cover tools.
const (
	// MaxCoverCandidates is the most covers one search fetches and returns.
	MaxCoverCandidates = 5
	// CurrentCoverWidth is the width, in pixels, a book's current cover is
	// fetched at.
	CurrentCoverWidth = 400
	// PreviewWidth is the widest a candidate's preview is, in pixels: the
	// width of the current cover, so that old and new compare at one size.
	PreviewWidth = CurrentCoverWidth
	// PreviewMaxHeight is the tallest a candidate's preview is, in pixels. A
	// candidate over either limit is scaled down to fit, by the larger of the
	// two ratios.
	PreviewMaxHeight = 800
	// MaxPreviewBytes is the size up to which a JPEG, PNG or GIF candidate that
	// fits inside [PreviewWidth] by [PreviewMaxHeight] is shown as it is, 256
	// KiB. A WebP is never shown as it is: it is converted to a JPEG.
	MaxPreviewBytes = 256 << 10
	// MaxPreviewPixels is the most pixels (width times height) a candidate may
	// claim to have. A larger one is refused before it is decoded, as a small
	// file can claim a size no memory holds.
	MaxPreviewPixels = 16_000_000
)

// Errors returned by the Covers service. They are wrapped with details, so
// match them with [errors.Is].
var (
	ErrNoCoverPermission = errors.New("the Audiobookshelf user needs the Update and Upload permissions")
	ErrNotWebURL         = errors.New("not an http or https URL")
	ErrCoverURLCount     = fmt.Errorf("give 1 to %d cover URLs", MaxCoverCandidates)
)
