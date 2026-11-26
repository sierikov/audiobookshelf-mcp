package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
)

// maxConcurrentPreviews is the most previews the server makes at the same
// time, whatever the number of searches running. A preview decodes a whole
// image, which takes memory and time, so the downloads run together but the
// previews go two at a time.
const maxConcurrentPreviews = 2

// Covers finds covers for a book on a metadata provider, shows covers found
// elsewhere next to a book's own, and sets a book's cover from a web URL.
type Covers struct {
	api   CoversAPI
	fetch ImageFetcher
	// preview makes the preview of an image; tests replace it.
	preview func(abs.Image) (abs.Image, error)
	// slots has a place for each preview being made, [maxConcurrentPreviews]
	// of them, for all searches together: a preview takes one and gives it
	// back when done.
	slots chan struct{}
}

// NewCovers creates a Covers service that talks to Audiobookshelf through
// api and downloads cover images with fetch.
func NewCovers(api CoversAPI, fetch ImageFetcher) *Covers {
	return &Covers{api: api, fetch: fetch, preview: preview, slots: make(chan struct{}, maxConcurrentPreviews)}
}

// Find searches a provider (one of [MetadataProviders], typed in any case) for
// covers of a book. Title and author default to the book's own. An item that
// is not a book is an [ErrBooksOnly] error.
//
// The first [MaxCoverCandidates] results are fetched at the same time, and
// come back in the provider's order, each as a preview that fits inside
// [PreviewWidth] by [PreviewMaxHeight] (see [preview]); at most
// [maxConcurrentPreviews] are made at once, across all searches. A result that cannot be fetched
// or shown is still a candidate, with a Problem and no Image; it does not fail
// the search.
//
// The book's current cover is fetched at [CurrentCoverWidth]. A book without
// one has a nil Current. So has one whose cover Audiobookshelf answers with
// an error status: that is no reason to give up the search, and CurrentProblem
// says it. A failure of the request itself fails the search, as does ctx
// ending before the fetches do.
func (s *Covers) Find(ctx context.Context, itemID, provider, title, author string) (CoverSearch, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if !slices.Contains(metadataProviders, provider) {
		return CoverSearch{}, unknownProvider(provider)
	}
	return s.search(ctx, itemID, provider, func(bookTitle, bookAuthor string) ([]string, error) {
		return s.api.SearchCovers(ctx, provider, cmp.Or(title, bookTitle), cmp.Or(author, bookAuthor))
	})
}

// Show shows the covers at urls, found somewhere other than a provider, such
// as on the web, next to a book's current cover, as [Find] shows a provider's:
// the result is the same, with [WebSource] as its provider and a candidate
// for each of urls, in that order. There must be 1 to [MaxCoverCandidates] of
// them ([ErrCoverURLCount]), checked before any call.
func (s *Covers) Show(ctx context.Context, itemID string, urls []string) (CoverSearch, error) {
	if len(urls) == 0 || len(urls) > MaxCoverCandidates {
		return CoverSearch{}, fmt.Errorf("%w, not %d", ErrCoverURLCount, len(urls))
	}
	return s.search(ctx, itemID, WebSource, func(string, string) ([]string, error) { return urls, nil })
}

// search makes the result of [Find] and [Show] for a book: its current cover,
// and a preview of each of the first [MaxCoverCandidates] covers whose URLs
// find gives, which it is passed the book's title and author for. source is
// where the URLs come from.
func (s *Covers) search(ctx context.Context, itemID, source string, find func(title, author string) ([]string, error)) (CoverSearch, error) {
	li, err := s.api.Item(ctx, itemID)
	if err != nil {
		return CoverSearch{}, err
	}
	b, err := asBook(li)
	if err != nil {
		return CoverSearch{}, err
	}
	m := b.Metadata
	current, currentProblem, err := s.currentCover(ctx, itemID)
	if err != nil {
		return CoverSearch{}, err
	}
	urls, err := find(m.Title, m.AuthorName)
	if err != nil {
		return CoverSearch{}, err
	}
	candidates := s.previews(ctx, urls[:min(len(urls), MaxCoverCandidates)])
	if err := ctx.Err(); err != nil {
		return CoverSearch{}, err // the problems would only say the request was cut short
	}
	return CoverSearch{
		ItemID: itemID, Title: m.Title, Author: m.AuthorName, Provider: source,
		Current: current, CurrentProblem: currentProblem, Candidates: candidates,
	}, nil
}

// currentCover fetches a book's cover. A book without one is not an error:
// Audiobookshelf answers 404, and the cover is nil. Nor is another status of
// Audiobookshelf: the cover is nil and the problem says what the status was,
// and nothing the server said. An error that is not a status is returned.
func (s *Covers) currentCover(ctx context.Context, itemID string) (img *abs.Image, problem string, err error) {
	cover, err := s.api.ItemCover(ctx, itemID, CurrentCoverWidth)
	var status *abs.StatusError
	switch {
	case errors.As(err, &status) && status.StatusCode == http.StatusNotFound:
		return nil, "", nil
	case errors.As(err, &status):
		return nil, fmt.Sprintf("the current cover could not be read (HTTP %d)", status.StatusCode), nil
	case err != nil:
		return nil, "", err
	}
	return &cover, "", nil
}

// previews fetches the images at urls at the same time, one request each,
// and returns the candidates, each with a preview of its image, in the order of
// urls. The previews themselves are made [maxConcurrentPreviews] at a time,
// counting those of other searches.
func (s *Covers) previews(ctx context.Context, urls []string) []CoverCandidate {
	out := make([]CoverCandidate, len(urls))
	var wg sync.WaitGroup
	for i, u := range urls {
		wg.Go(func() { out[i] = s.candidate(ctx, u) })
	}
	wg.Wait()
	return out
}

// candidate fetches one candidate and makes its image a [preview], once a slot
// is free. A URL that is not a web URL is not fetched, and is shown
// through [quoted]: nothing can use it, so it need not come back whole, and
// text that may hide invisible characters or run on must not.
func (s *Covers) candidate(ctx context.Context, imageURL string) CoverCandidate {
	if !isWebURL(imageURL) {
		return CoverCandidate{URL: quoted(imageURL), Problem: problemOf(ErrNotWebURL)}
	}
	c := CoverCandidate{URL: imageURL}
	img, err := s.fetch.Fetch(ctx, imageURL)
	if err == nil {
		img, err = s.previewWhenFree(ctx, img)
	}
	if err != nil {
		c.Problem = problemOf(err)
		return c
	}
	c.Image = &img
	return c
}

// previewWhenFree makes the [preview] of img once it has a slot, which it gives
// back when done. It gives up with the error of ctx if that ends first.
func (s *Covers) previewWhenFree(ctx context.Context, img abs.Image) (abs.Image, error) {
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-ctx.Done():
		return abs.Image{}, ctx.Err()
	}
	return s.preview(img)
}

// problemOf is the phrase that says why a candidate could not be fetched or
// shown. The phrases are ours: an error from a request carries the URL and,
// often, text the server chose, and a problem goes on to the model.
func problemOf(err error) string {
	var status *statusError
	switch {
	case errors.Is(err, ErrNotWebURL):
		return ErrNotWebURL.Error()
	case errors.Is(err, errNotImage):
		return errNotImage.Error()
	case errors.Is(err, errImageTooLarge):
		return fmt.Sprintf("over %d MiB", abs.MaxImageBytes>>20)
	case errors.Is(err, errPreviewTooLarge):
		return errPreviewTooLarge.Error()
	case errors.Is(err, errCannotPreview):
		return errCannotPreview.Error()
	case errors.Is(err, errTooManyRedirects):
		return "too many redirects"
	case isTimeout(err):
		return "timed out"
	case errors.As(err, &status):
		return status.Error()
	}
	return "could not be fetched"
}

// isTimeout reports whether err is a deadline that passed, whichever way the
// network stack says so.
func isTimeout(err error) bool {
	var netErr net.Error
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) ||
		(errors.As(err, &netErr) && netErr.Timeout())
}

// Set makes the image at a web URL a book's cover, in place of the old one.
// This is final: Audiobookshelf deletes the old cover file. The URL must be
// http or https ([ErrNotWebURL]), checked before any call; the item must be a
// book ([ErrBooksOnly]). Audiobookshelf refuses a user without both the Update
// and the Upload permission, with the same bare 403 for either. The item was
// read first, so that 403 can only mean a missing permission, and is an
// [ErrNoCoverPermission] error.
func (s *Covers) Set(ctx context.Context, itemID, coverURL string) (CoverSet, error) {
	if !isWebURL(coverURL) {
		return CoverSet{}, fmt.Errorf("cover URL %s: %w", quoted(coverURL), ErrNotWebURL)
	}
	li, err := s.api.Item(ctx, itemID)
	if err != nil {
		return CoverSet{}, err
	}
	b, err := asBook(li)
	if err != nil {
		return CoverSet{}, err
	}
	err = s.api.SetCoverURL(ctx, itemID, coverURL)
	var status *abs.StatusError
	switch {
	case errors.As(err, &status) && status.StatusCode == http.StatusForbidden:
		return CoverSet{}, fmt.Errorf("item %s: %w", itemID, ErrNoCoverPermission)
	case err != nil:
		return CoverSet{}, fmt.Errorf("setting the cover of item %s: %w", itemID, err)
	}
	return CoverSet{ItemID: itemID, Title: b.Metadata.Title}, nil
}
