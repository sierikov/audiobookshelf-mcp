package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
)

// ImageFetcher downloads an image from a web URL.
//
// A failure is [ErrNotWebURL], one of this package's own errors for a
// response that is not a usable image, or the error of the request itself,
// which names the URL and may carry text the server chose. So callers show a
// phrase of their own for it (see [problemOf]), never the error.
type ImageFetcher interface {
	Fetch(ctx context.Context, imageURL string) (abs.Image, error)
}

const (
	// fetchTimeout bounds one download, redirects and body included.
	fetchTimeout = 15 * time.Second
	// maxRedirects is the most redirects a download follows.
	maxRedirects = 3
)

// imageTypes are the media types a download may have, and the ones
// Audiobookshelf's own covers come in.
var imageTypes = []string{"image/jpeg", "image/png", "image/webp", "image/gif"}

var (
	errNotImage         = errors.New("not an image")
	errImageTooLarge    = fmt.Errorf("image is over %d MiB", abs.MaxImageBytes>>20)
	errTooManyRedirects = fmt.Errorf("more than %d redirects", maxRedirects)
)

// statusError is a response with a status other than 200. It keeps the
// status and nothing the server said.
type statusError struct{ Code int }

func (e *statusError) Error() string { return fmt.Sprintf("HTTP %d", e.Code) }

// webImageFetcher downloads images with a client of its own: it carries no
// Audiobookshelf token, no cookies and no credentials of any kind.
type webImageFetcher struct {
	client *http.Client
}

// NewImageFetcher creates an [ImageFetcher] for the web. A download takes at
// most 15 seconds and follows at most 3 redirects, each to an http or https
// URL. The response must be a JPEG, PNG, WebP or GIF of at most
// [abs.MaxImageBytes]: its Content-Type must say so, and so must its first
// bytes. Nothing is written to disk.
func NewImageFetcher() ImageFetcher {
	return newImageFetcher(fetchTimeout)
}

func newImageFetcher(timeout time.Duration) *webImageFetcher {
	return &webImageFetcher{client: &http.Client{Timeout: timeout, CheckRedirect: checkRedirect}}
}

// checkRedirect stops a download at too many redirects and at one to
// anything but a web URL, one with a user name or password included.
func checkRedirect(req *http.Request, via []*http.Request) error {
	switch {
	case len(via) > maxRedirects:
		return errTooManyRedirects
	case !isWebTarget(req.URL):
		return ErrNotWebURL
	}
	return nil
}

// Fetch implements [ImageFetcher]. The image's type is what its bytes are,
// which a server's Content-Type may only claim.
func (f *webImageFetcher) Fetch(ctx context.Context, imageURL string) (abs.Image, error) {
	if !isWebURL(imageURL) {
		return abs.Image{}, ErrNotWebURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, http.NoBody)
	if err != nil {
		return abs.Image{}, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return abs.Image{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return abs.Image{}, &statusError{Code: resp.StatusCode}
	}
	return readImage(resp)
}

// readImage reads the body of a response that must be an image.
func readImage(resp *http.Response) (abs.Image, error) {
	declared, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || !slices.Contains(imageTypes, declared) {
		return abs.Image{}, errNotImage
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, abs.MaxImageBytes+1))
	if err != nil {
		return abs.Image{}, err
	}
	if len(data) > abs.MaxImageBytes {
		return abs.Image{}, errImageTooLarge
	}
	sniffed := http.DetectContentType(data)
	if !slices.Contains(imageTypes, sniffed) {
		return abs.Image{}, errNotImage
	}
	return abs.Image{Data: data, MIMEType: sniffed}, nil
}

// isWebTarget reports whether u is an http or https URL with a host and
// without a user name or password: Go sends those as an Authorization header,
// and a provider can use them to dress one host up as another
// (https://trusted.example@evil.example/).
func isWebTarget(u *url.URL) bool {
	return (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil
}

// maxWebURLBytes is the longest a web URL may be, in bytes: the length the
// common browsers and servers accept, and more than any cover's address needs.
const maxWebURLBytes = 2048

// isWebURL reports whether s is a URL [isWebTarget] accepts, is at most
// [maxWebURLBytes] long and has no whitespace, control or invisible character,
// which no real URL needs and text meant for a reader could hide in.
func isWebURL(s string) bool {
	if len(s) > maxWebURLBytes {
		return false
	}
	u, err := url.Parse(s)
	if err != nil || !isWebTarget(u) {
		return false
	}
	return !strings.ContainsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || isUnseen(r) })
}

// isUnseen reports whether r is a control character or an invisible one: what
// [isHidden] names, and the line and paragraph separators, which [checkPlain]
// refuses too.
func isUnseen(r rune) bool {
	return unicode.IsControl(r) || isHidden(r) || r == '\u2028' || r == '\u2029'
}
