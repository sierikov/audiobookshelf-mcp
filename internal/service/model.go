package service

import (
	"fmt"
	"time"
)

// Search hit types besides the media types "book" and "podcast".
const (
	HitAuthor = "author"
	HitSeries = "series"
)

// Page selects one page of a paginated list. Page numbers start at 0.
type Page struct {
	Limit int
	Page  int
}

// Library is an audiobook or podcast library.
type Library struct {
	ID        string
	Name      string
	MediaType string
}

// Item is a library item reduced to what lists show.
type Item struct {
	ID        string
	MediaType string
	Title     string
	Author    string
	// Duration is known only for books. HasDuration tells a known zero
	// duration apart from no duration at all.
	Duration    time.Duration
	HasDuration bool
}

// ItemDetail is a library item with all of its metadata.
type ItemDetail struct {
	Item
	Subtitle      string
	Narrator      string
	SeriesName    string
	Description   string
	PublishedYear string
	Publisher     string
	Genres        []string
	Language      string
	NumEpisodes   int
}

// ItemsPage is one page of a library's items.
type ItemsPage struct {
	Items []Item
	Total int
	Page  int
}

// SearchHit is one search result. Type is "book", "podcast", [HitAuthor] or
// [HitSeries]; authors and series carry only an ID and a Title.
type SearchHit struct {
	Type string
	Item
}

// Progress is the user's listening progress on an item or podcast episode.
type Progress struct {
	Fraction    float64 // 0.0–1.0
	CurrentTime time.Duration
	Duration    time.Duration
	Remaining   time.Duration // Duration minus CurrentTime; may be zero or negative
	IsFinished  bool
	LastUpdate  time.Time // zero when unknown
	StartedAt   time.Time // zero when unknown
}

// InProgressItem is an item the user is listening to. Progress is nil when
// it could not be fetched.
type InProgressItem struct {
	Item
	Progress *Progress
}

// Session is one listening session.
type Session struct {
	Title     string
	Author    string
	Duration  time.Duration
	StartedAt time.Time // zero when unknown
}

// SessionsPage is the most recent listening sessions and the total count.
type SessionsPage struct {
	Sessions []Session
	Total    int
}

// Stats are the user's aggregated listening statistics.
type Stats struct {
	Total          time.Duration
	Today          time.Duration
	DayOfWeek      map[string]time.Duration
	RecentSessions []Session
}

// SeriesSummary is a series as it appears in a library listing.
type SeriesSummary struct {
	ID       string
	Name     string
	NumBooks int
}

// SeriesPage is one page of a library's series.
type SeriesPage struct {
	Series []SeriesSummary
	Total  int
}

// SeriesDetail is a series with its books.
type SeriesDetail struct {
	ID          string
	Name        string
	Description string
	Books       []Item
}

// AuthorDetail is an author with their books.
type AuthorDetail struct {
	ID          string
	Name        string
	Description string
	NumBooks    int
	Books       []Item
}

// CollectionSummary is a collection as it appears in a library listing.
type CollectionSummary struct {
	ID          string
	Name        string
	Description string
	NumBooks    int
}

// CollectionsPage is one page of a library's collections.
type CollectionsPage struct {
	Collections []CollectionSummary
	Total       int
}

// MediaError reports a library item whose media could not be decoded.
type MediaError struct {
	ItemID    string
	MediaType string
	Err       error
}

func (e *MediaError) Error() string {
	return fmt.Sprintf("item %s: decoding %s media: %v", e.ItemID, e.MediaType, e.Err)
}

// Unwrap returns the underlying decode error.
func (e *MediaError) Unwrap() error { return e.Err }
