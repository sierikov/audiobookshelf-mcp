package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
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
// elsewhere next to a book's own, sets a book's cover from a web URL, and
// writes prompts for making a cover in an image app.
type Covers struct {
	api   CoversAPI
	fetch ImageFetcher
	// serverURL is the address of the Audiobookshelf server, without a
	// trailing slash.
	serverURL string
	// preview makes the preview of an image; tests replace it.
	preview func(abs.Image) (abs.Image, error)
	// slots has a place for each preview being made, [maxConcurrentPreviews]
	// of them, for all searches together: a preview takes one and gives it
	// back when done.
	slots chan struct{}
}

// NewCovers creates a Covers service that talks to Audiobookshelf through
// api and downloads cover images with fetch. serverURL is the address of the
// Audiobookshelf server, which the pages of its books are built from; a
// trailing slash is dropped.
func NewCovers(api CoversAPI, fetch ImageFetcher, serverURL string) *Covers {
	return &Covers{
		api: api, fetch: fetch, serverURL: strings.TrimRight(serverURL, "/"),
		preview: preview, slots: make(chan struct{}, maxConcurrentPreviews),
	}
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
	current, currentProblem, err := currentCover(ctx, s.api, itemID)
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

// currentCover fetches a book's cover through api, at [CurrentCoverWidth]. A
// book without one is not an error: Audiobookshelf answers 404, and the cover
// is nil. Nor is another status of Audiobookshelf: the cover is nil and the
// problem says what the status was, and nothing the server said. An error that
// is not a status is returned.
func currentCover(ctx context.Context, api interface {
	ItemCover(ctx context.Context, itemID string, width int) (abs.Image, error)
}, itemID string) (img *abs.Image, problem string, err error) {
	cover, err := api.ItemCover(ctx, itemID, CurrentCoverWidth)
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
	if err := s.api.SetCoverURL(ctx, itemID, coverURL); err != nil {
		return CoverSet{}, coverSetError(itemID, err)
	}
	return CoverSet{ItemID: itemID, Title: b.Metadata.Title}, nil
}

// coverSetError is the error for err, the failure of a request that set the
// cover of the book itemID. The book was read first, so a 403 can only mean a
// missing permission, and is an [ErrNoCoverPermission] error, without what
// the server said. Any other error is wrapped as it is.
func coverSetError(itemID string, err error) error {
	if status, ok := errors.AsType[*abs.StatusError](err); ok && status.StatusCode == http.StatusForbidden {
		return fmt.Errorf("item %s: %w", itemID, ErrNoCoverPermission)
	}
	return fmt.Errorf("setting the cover of item %s: %w", itemID, err)
}

// Prompt writes a prompt for an image app outside this server, such as the
// Gemini app or ChatGPT, for a book's cover. With r.Style it is the prompt for
// a new cover, as [newCoverPrompt] writes it from the book's title, author and
// genres as stored, r.Premise and r.TitleText, with r.Title in place of the
// book's title when it has one, followed by [squareLine]: audiobook covers are
// square, and no setting of the app carries the shape. With r.Instructions it
// is the prompt for cleaning the current cover as [editCoverPrompt] writes it,
// with Edit set, which asks for a square cover itself. Either way AspectRatio
// is [squareRatio], and the book's library is not read. The inputs are trimmed
// and each one given must be plain text within [MaxStyleLength],
// [MaxPremiseLength], [MaxTitleLength] and [MaxEditLength]; the error names the
// field. Giving both Style and Instructions, or neither, is an
// [ErrStyleOrInstructions] error. All of this is checked before any call. An
// item that is not a book is an [ErrBooksOnly] error.
func (s *Covers) Prompt(ctx context.Context, r PromptRequest) (CoverPrompt, error) {
	style, premise, title, instructions := strings.TrimSpace(r.Style), strings.TrimSpace(r.Premise), strings.TrimSpace(r.Title), strings.TrimSpace(r.Instructions)
	if (style == "") == (instructions == "") {
		return CoverPrompt{}, ErrStyleOrInstructions
	}
	for _, f := range []struct {
		name, value string
		limit       int
	}{{"style", style, MaxStyleLength}, {"premise", premise, MaxPremiseLength}, {"title", title, MaxTitleLength}, {"instructions", instructions, MaxEditLength}} {
		if f.value == "" {
			continue
		}
		if reason := badValue(f.name, f.value, f.limit); reason != "" {
			return CoverPrompt{}, errors.New(reason)
		}
	}

	li, err := s.api.Item(ctx, r.ItemID)
	if err != nil {
		return CoverPrompt{}, err
	}
	b, err := asBook(li)
	if err != nil {
		return CoverPrompt{}, err
	}
	m := b.Metadata
	out := CoverPrompt{
		ItemID: r.ItemID, Title: m.Title, AspectRatio: squareRatio,
		BookPage: s.serverURL + "/item/" + url.PathEscape(r.ItemID),
	}
	if instructions != "" {
		out.Edit = true
		out.Prompt = editCoverPrompt(instructions)
		return out, nil
	}
	book := coverBook{Title: cmp.Or(title, m.Title), Author: m.AuthorName, Genres: m.Genres}
	out.Prompt = newCoverPrompt(book, style, premise, r.TitleText, false) + "\n" + squareLine
	return out, nil
}
