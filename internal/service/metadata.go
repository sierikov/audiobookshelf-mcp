package service

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
)

// Metadata surveys the genres, tags, languages and descriptions of a book
// library, finds books by them, and looks books up on metadata providers.
type Metadata struct {
	api MetadataAPI
	// writeMu serializes writes. Each one reads the book, then saves values
	// computed from what it read, so two at once could lose one's change.
	writeMu sync.Mutex
}

// NewMetadata creates a Metadata service backed by api.
func NewMetadata(api MetadataAPI) *Metadata {
	return &Metadata{api: api}
}

// Overview counts the genres, tags and languages used in a book library and
// how many books lack each field. A library with anything but books is an
// [ErrBooksOnly] error.
func (s *Metadata) Overview(ctx context.Context, libraryID string) (MetadataOverview, error) {
	items, err := s.books(ctx, libraryID)
	if err != nil {
		return MetadataOverview{}, err
	}
	o := MetadataOverview{LibraryID: libraryID, Books: len(items)}
	genres := make([][]string, 0, len(items))
	tags := make([][]string, 0, len(items))
	languages := make([][]string, 0, len(items))
	for _, it := range items {
		genres = append(genres, it.Genres)
		tags = append(tags, it.Tags)
		if strings.TrimSpace(it.Language) != "" {
			languages = append(languages, []string{it.Language})
		}
		o.Missing.add(it)
	}
	o.Genres, o.Tags, o.Languages = countValues(genres), countValues(tags), countValues(languages)
	return o, nil
}

// add counts the fields m lacks.
func (c *MissingCounts) add(m MetadataItem) {
	if isMissing(m, MissingGenres) {
		c.Genres++
	}
	if isMissing(m, MissingTags) {
		c.Tags++
	}
	if isMissing(m, MissingLanguage) {
		c.Language++
	}
	if isMissing(m, MissingDescription) {
		c.Description++
	}
}

// Find returns one page of the books in a library that pass the filter,
// sorted by title (ignoring case) and then by id. The page is checked first:
// the limit must be 1 to [MaxPageLimit] and the page number not negative
// ([ErrBadPage]); so is the missing field ([ErrUnknownMissing]).
func (s *Metadata) Find(ctx context.Context, libraryID string, filter ItemFilter, page Page) (MetadataPage, error) {
	if page.Limit < 1 || page.Limit > MaxPageLimit {
		return MetadataPage{}, fmt.Errorf("%w: limit %d, use 1 to %d", ErrBadPage, page.Limit, MaxPageLimit)
	}
	if page.Page < 0 {
		return MetadataPage{}, fmt.Errorf("%w: page %d, use 0 or more", ErrBadPage, page.Page)
	}
	if filter.Missing != "" && !slices.Contains(MissingFields(), filter.Missing) {
		return MetadataPage{}, fmt.Errorf("%w %q (use one of: %s)", ErrUnknownMissing, filter.Missing, joinMissing(MissingFields()))
	}
	items, err := s.books(ctx, libraryID)
	if err != nil {
		return MetadataPage{}, err
	}
	matches := slices.DeleteFunc(items, func(m MetadataItem) bool { return !matchesFilter(m, filter) })
	slices.SortFunc(matches, func(a, b MetadataItem) int {
		return cmp.Or(cmp.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title)), cmp.Compare(a.ID, b.ID))
	})
	out := MetadataPage{Total: len(matches), Page: page.Page}
	if page.Page <= len(matches)/page.Limit {
		start := page.Page * page.Limit
		out.Items = matches[start:min(start+page.Limit, len(matches))]
	}
	return out, nil
}

func joinMissing(fields []MissingField) string {
	names := make([]string, len(fields))
	for i, f := range fields {
		names[i] = string(f)
	}
	return strings.Join(names, ", ")
}

// SearchBooks asks a metadata provider about a book. Title and author
// default to the book's own. The provider is one of [MetadataProviders],
// typed in any case; region applies to audible only ("" means
// [DefaultAudibleRegion]). Candidate descriptions are the provider's HTML.
func (s *Metadata) SearchBooks(ctx context.Context, itemID, provider, region, title, author string) (MetadataSearch, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	apiProvider, region, err := searchProvider(provider, region)
	if err != nil {
		return MetadataSearch{}, err
	}
	li, err := s.api.Item(ctx, itemID)
	if err != nil {
		return MetadataSearch{}, err
	}
	b, err := asBook(li)
	if err != nil {
		return MetadataSearch{}, err
	}
	m := b.Metadata
	matches, err := s.api.SearchBooks(ctx, apiProvider, cmp.Or(title, m.Title), cmp.Or(author, m.AuthorName), itemID)
	if err != nil {
		return MetadataSearch{}, err
	}
	return MetadataSearch{
		Book:       Item{ID: itemID, MediaType: abs.MediaTypeBook, Title: m.Title, Author: m.AuthorName},
		Provider:   provider,
		Region:     region,
		Candidates: toCandidates(matches),
	}, nil
}

// searchProvider checks a normalised provider name and region and returns
// the provider name Audiobookshelf expects with the normalised region, which
// is set for audible only.
func searchProvider(provider, region string) (apiProvider, normalised string, err error) {
	switch {
	case !slices.Contains(metadataProviders, provider):
		return "", "", fmt.Errorf("%w %q (use one of: %s)", ErrUnknownProvider, provider, strings.Join(metadataProviders, ", "))
	case provider == "audible":
		return audibleProvider(region)
	case strings.TrimSpace(region) != "":
		return "", "", fmt.Errorf("%w: got region %q for %s", ErrRegionNotAudible, region, provider)
	}
	return provider, "", nil
}

func toCandidates(ms []abs.BookMatch) []MetadataCandidate {
	out := make([]MetadataCandidate, 0, len(ms))
	for _, m := range ms {
		out = append(out, MetadataCandidate{
			Title: m.Title, Subtitle: m.Subtitle, Author: m.Author, Narrator: m.Narrator,
			Publisher: m.Publisher, PublishedYear: string(m.PublishedYear), Language: m.Language,
			Genres: m.Genres, Tags: m.Tags, Description: m.Description,
		})
	}
	return out
}

// books fetches every item of a library as a [MetadataItem]. An item that is
// not a book is an [ErrBooksOnly] error; one whose media cannot be decoded
// is a [*MediaError].
func (s *Metadata) books(ctx context.Context, libraryID string) ([]MetadataItem, error) {
	page, err := s.api.LibraryItems(ctx, libraryID, abs.Page{Limit: 0}, "")
	if err != nil {
		return nil, err
	}
	items := make([]MetadataItem, 0, len(page.Results))
	for _, li := range page.Results {
		b, err := asBook(li)
		if err != nil {
			return nil, err
		}
		m := b.Metadata
		items = append(items, MetadataItem{
			ID: li.ID, Title: m.Title, Author: m.AuthorName, Series: m.SeriesName,
			Language: m.Language, Genres: m.Genres, Tags: b.Tags,
			Description: plainText(m.Description),
		})
	}
	return items, nil
}

// booksByID fetches books by id in one call and returns them by id. An id
// Audiobookshelf does not know is an [ErrUnknownItems] error listing all
// such ids in the order given; an item that is not a book is an
// [ErrBooksOnly] error.
func (s *Metadata) booksByID(ctx context.Context, ids []string) (map[string]abs.LibraryItem, error) {
	found, err := s.api.ItemsByID(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]abs.LibraryItem, len(found))
	for _, li := range found {
		if li.MediaType != abs.MediaTypeBook {
			return nil, notABook(li)
		}
		byID[li.ID] = li
	}
	var unknown []string
	for _, id := range ids {
		if _, ok := byID[id]; !ok {
			unknown = append(unknown, id)
		}
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrUnknownItems, strings.Join(unknown, ", "))
	}
	return byID, nil
}

// asBook decodes a library item as a book.
func asBook(li abs.LibraryItem) (*abs.BookMedia, error) {
	if li.MediaType != abs.MediaTypeBook {
		return nil, notABook(li)
	}
	b, err := li.AsBook()
	if err != nil {
		return nil, &MediaError{ItemID: li.ID, MediaType: li.MediaType, Err: err}
	}
	return b, nil
}

func notABook(li abs.LibraryItem) error {
	return fmt.Errorf("item %s is a %s: %w", li.ID, li.MediaType, ErrBooksOnly)
}
