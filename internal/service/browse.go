package service

import (
	"context"

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

// Series returns a series with its decodable books.
func (s *Browse) Series(ctx context.Context, seriesID string) (SeriesDetail, error) {
	sr, err := s.api.Series(ctx, seriesID)
	if err != nil {
		return SeriesDetail{}, err
	}
	return SeriesDetail{
		ID:          sr.ID,
		Name:        sr.Name,
		Description: sr.Description,
		Books:       itemsOfType(sr.Books, abs.MediaTypeBook),
	}, nil
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
