package service

import (
	"cmp"
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
)

// Browse explores series, authors and collections.
type Browse struct {
	api BrowseAPI
}

// NewBrowse creates a Browse service backed by api.
func NewBrowse(api BrowseAPI) *Browse {
	return &Browse{api: api}
}

// ListSeries returns one page of a library's series with their book counts.
func (s *Browse) ListSeries(ctx context.Context, libraryID string, page Page) (SeriesPage, error) {
	res, err := s.api.LibrarySeries(ctx, libraryID, abs.Page(page))
	if err != nil {
		return SeriesPage{}, err
	}
	series := make([]SeriesSummary, 0, len(res.Results))
	for _, sr := range res.Results {
		series = append(series, SeriesSummary{ID: sr.ID, Name: sr.Name, NumBooks: len(sr.Books)})
	}
	return SeriesPage{Series: series, Total: res.Total}, nil
}

// Series returns a series with its books in reading order. Audiobookshelf
// sends no books with the series itself, only the IDs of its items in the
// progress block, so the books come from a second call. Items that are not
// decodable books are left out.
func (s *Browse) Series(ctx context.Context, seriesID string) (SeriesDetail, error) {
	sr, err := s.api.Series(ctx, seriesID)
	if err != nil {
		return SeriesDetail{}, err
	}
	detail := SeriesDetail{ID: sr.ID, Name: sr.Name, Description: sr.Description, Books: []SeriesBook{}}
	if sr.Progress == nil || len(sr.Progress.LibraryItemIDs) == 0 {
		return detail, nil
	}
	lis, err := s.api.ItemsByID(ctx, sr.Progress.LibraryItemIDs)
	if err != nil {
		return SeriesDetail{}, err
	}
	for _, li := range lis {
		if b, ok := seriesBook(li, seriesID); ok {
			detail.Books = append(detail.Books, b)
		}
	}
	slices.SortStableFunc(detail.Books, compareSeriesBooks)
	return detail, nil
}

// seriesBook maps li if it is a decodable book, taking its sequence from its
// entry for the series; a book without such an entry has an empty one.
func seriesBook(li abs.LibraryItem, seriesID string) (SeriesBook, bool) {
	it, ok := itemOfType(li, abs.MediaTypeBook)
	if !ok {
		return SeriesBook{}, false
	}
	media, err := li.AsBook()
	if err != nil {
		return SeriesBook{}, false
	}
	b := SeriesBook{Item: it}
	if i := slices.IndexFunc(media.Metadata.Series, func(r abs.SeriesRef) bool { return r.ID == seriesID }); i >= 0 {
		b.Sequence = media.Metadata.Series[i].Sequence
	}
	return b, true
}

// compareSeriesBooks orders books with a numeric sequence first, by number,
// and the rest by title. Equal numbers are ordered by title too.
func compareSeriesBooks(a, b SeriesBook) int {
	an, aErr := strconv.ParseFloat(a.Sequence, 64)
	bn, bErr := strconv.ParseFloat(b.Sequence, 64)
	switch {
	case aErr == nil && bErr == nil:
		if c := cmp.Compare(an, bn); c != 0 {
			return c
		}
	case aErr == nil:
		return -1
	case bErr == nil:
		return 1
	}
	return strings.Compare(a.Title, b.Title)
}

// Author returns an author with their decodable books.
func (s *Browse) Author(ctx context.Context, authorID string) (AuthorDetail, error) {
	a, err := s.api.Author(ctx, authorID)
	if err != nil {
		return AuthorDetail{}, err
	}
	return AuthorDetail{
		ID:          a.ID,
		Name:        a.Name,
		Description: a.Description,
		NumBooks:    a.NumBooks,
		Books:       itemsOfType(a.LibraryItems, abs.MediaTypeBook),
	}, nil
}

// Collections returns one page of a library's collections with their book counts.
func (s *Browse) Collections(ctx context.Context, libraryID string, page Page) (CollectionsPage, error) {
	res, err := s.api.LibraryCollections(ctx, libraryID, abs.Page(page))
	if err != nil {
		return CollectionsPage{}, err
	}
	collections := make([]CollectionSummary, 0, len(res.Results))
	for _, c := range res.Results {
		collections = append(collections, CollectionSummary{
			ID: c.ID, Name: c.Name, Description: c.Description, NumBooks: len(c.Books),
		})
	}
	return CollectionsPage{Collections: collections, Total: res.Total}, nil
}
