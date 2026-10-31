package service

import (
	"errors"
	"fmt"
)

// ValueCount is a value and how many books carry it.
type ValueCount struct {
	Name  string
	Count int
}

// MissingCounts is how many books lack each field.
type MissingCounts struct {
	Genres      int
	Tags        int
	Language    int
	Description int
}

// MetadataOverview surveys a library's genres, tags and languages, each
// value with its book count, and how many books lack each field.
type MetadataOverview struct {
	LibraryID string
	Books     int
	Genres    []ValueCount
	Tags      []ValueCount
	Languages []ValueCount
	Missing   MissingCounts
}

// MissingField names a field a book may lack.
type MissingField string

// The fields a search can find books missing.
const (
	MissingGenres      MissingField = "genres"
	MissingTags        MissingField = "tags"
	MissingLanguage    MissingField = "language"
	MissingDescription MissingField = "description"
)

// MissingFields returns the fields a book can be found missing, in the order
// they are offered to callers.
func MissingFields() []MissingField {
	return []MissingField{MissingGenres, MissingTags, MissingLanguage, MissingDescription}
}

// ItemFilter selects books by exact genre, tag or language, or by a missing
// field. An empty field means no filter on it; set fields all must match.
type ItemFilter struct {
	Genre    string
	Tag      string
	Language string
	Missing  MissingField
}

// MetadataItem is a book with the metadata the metadata tools work on.
// Description is plain text.
type MetadataItem struct {
	ID          string
	Title       string
	Author      string
	Series      string
	Language    string
	Genres      []string
	Tags        []string
	Description string
}

// MetadataPage is one page of books matching a filter. Total counts all the
// matches, not just this page; Page is 0-based.
type MetadataPage struct {
	Items []MetadataItem
	Total int
	Page  int
}

// MetadataCandidate is a book as a metadata provider describes it.
// Description is the provider's HTML.
type MetadataCandidate struct {
	Title         string
	Subtitle      string
	Author        string
	Narrator      string
	Publisher     string
	PublishedYear string
	Language      string
	Genres        []string
	Tags          []string
	Description   string
}

// MetadataSearch is the result of asking a provider about a book: the book,
// the provider and the candidates found. Region is set only for audible.
type MetadataSearch struct {
	Book       Item
	Provider   string
	Region     string
	Candidates []MetadataCandidate
}

// metadataProviders are the providers Audiobookshelf can search for book
// metadata, in the order they are offered to callers.
var metadataProviders = []string{"fantlab", "google", "audible", "openlibrary", "itunes"}

// MetadataProviders returns the metadata providers a book can be searched on.
func MetadataProviders() []string {
	return append([]string(nil), metadataProviders...)
}

// ListChange is the outcome for one book of adding or removing list values:
// the list before and after.
type ListChange struct {
	ItemID   string
	Title    string
	Previous []string
	Now      []string
}

// ListChangeResult is the outcome of adding or removing list values. Changed
// counts the books that changed; Items has every book asked about.
type ListChangeResult struct {
	Changed int
	Items   []ListChange
}

// ItemChange is a requested change to one book. A nil field is left alone.
type ItemChange struct {
	ItemID      string
	Language    *string
	Description *string
}

// ItemFields is a book's language and description. A nil field was not part
// of the change.
type ItemFields struct {
	Language    *string
	Description *string
}

// ItemUpdateResult is the outcome for one book of an update: the fields
// before and after, for the fields that were asked about.
type ItemUpdateResult struct {
	ItemID   string
	Title    string
	Previous ItemFields
	Now      ItemFields
}

// UpdateResult is the outcome of updating books. Changed counts the books
// that changed; Items has every book asked about.
type UpdateResult struct {
	Changed int
	Items   []ItemUpdateResult
}

// Limits on what the metadata tools accept.
const (
	// MaxItemsPerCall is the most books one write call may name.
	MaxItemsPerCall = 100
	// MaxDescriptionsPerCall is the most descriptions one update may change.
	MaxDescriptionsPerCall = 10
	// MaxPageLimit is the most books one page of results may hold.
	MaxPageLimit = 100
)

// Errors returned by the Metadata service. They are wrapped with details, so
// match them with [errors.Is].
var (
	ErrBooksOnly           = errors.New("the metadata tools work on books only")
	ErrUnknownItems        = errors.New("unknown item ids")
	ErrUnknownProvider     = errors.New("unknown metadata provider")
	ErrRegionNotAudible    = errors.New("region applies to the audible provider only")
	ErrUnknownMissing      = errors.New("unknown missing field")
	ErrNoItems             = errors.New("no items given")
	ErrTooManyItems        = errors.New("too many items")
	ErrTooManyDescriptions = errors.New("too many descriptions")
	ErrNoValues            = errors.New("no values given")
	ErrBadPage             = errors.New("bad page")
	ErrWriteIncomplete     = errors.New("the write failed; some books may already be saved")
	// ErrReadBack is a write that went through, followed by a failed read of
	// the books for their new values; the previous values are still known.
	ErrReadBack = errors.New("the books were saved, but reading them back failed")
)

// ValueError reports a value that fails the checks before saving.
// Position is the 1-based position in the list.
type ValueError struct {
	Position int
	Reason   string
}

func (e *ValueError) Error() string {
	return fmt.Sprintf("value %d %s", e.Position, e.Reason)
}

// ItemError reports a book that fails the checks before saving. Position is
// the 1-based position in the list.
type ItemError struct {
	Position int
	ItemID   string
	Reason   string
}

func (e *ItemError) Error() string {
	return fmt.Sprintf("item %d (%s): %s", e.Position, e.ItemID, e.Reason)
}
