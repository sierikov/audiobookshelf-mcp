package service

import (
	"context"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
)

// Libraries lists, searches and pages through libraries.
type Libraries struct {
	api LibrariesAPI
}

// NewLibraries creates a Libraries service backed by api.
func NewLibraries(api LibrariesAPI) *Libraries {
	return &Libraries{api: api}
}

// List returns all libraries.
func (s *Libraries) List(ctx context.Context) ([]Library, error) {
	libs, err := s.api.Libraries(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Library, len(libs))
	for i, l := range libs {
		out[i] = Library{ID: l.ID, Name: l.Name, MediaType: l.MediaType}
	}
	return out, nil
}

// Search finds books, podcasts, authors and series in a library, in that
// order. Book and podcast hits that cannot be decoded are skipped.
func (s *Libraries) Search(ctx context.Context, libraryID, query string, limit int) ([]SearchHit, error) {
	res, err := s.api.SearchLibrary(ctx, libraryID, query, limit)
	if err != nil {
		return nil, err
	}
	var hits []SearchHit
	for _, h := range res.Book {
		if it, ok := itemOfType(h.LibraryItem, abs.MediaTypeBook); ok {
			hits = append(hits, SearchHit{Type: abs.MediaTypeBook, Item: it})
		}
	}
	for _, h := range res.Podcast {
		if it, ok := itemOfType(h.LibraryItem, abs.MediaTypePodcast); ok {
			hits = append(hits, SearchHit{Type: abs.MediaTypePodcast, Item: it})
		}
	}
	for _, a := range res.Authors {
		hits = append(hits, SearchHit{Type: HitAuthor, Item: Item{ID: a.ID, Title: a.Name}})
	}
	for _, h := range res.Series {
		hits = append(hits, SearchHit{Type: HitSeries, Item: Item{ID: h.Series.ID, Title: h.Series.Name}})
	}
	return hits, nil
}

// Items returns one page of a library's items, skipping undecodable ones.
func (s *Libraries) Items(ctx context.Context, libraryID string, page Page, sort string) (ItemsPage, error) {
	res, err := s.api.LibraryItems(ctx, libraryID, abs.Page(page), sort)
	if err != nil {
		return ItemsPage{}, err
	}
	return ItemsPage{Items: toItems(res.Results), Total: res.Total, Page: res.Page}, nil
}
