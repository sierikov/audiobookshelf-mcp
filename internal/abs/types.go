package abs

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// Media types a LibraryItem can have.
const (
	MediaTypeBook    = "book"
	MediaTypePodcast = "podcast"
)

// Page selects one page of a paginated list. Page numbers start at 0.
type Page struct {
	Limit int
	Page  int
}

func (p Page) query() url.Values {
	return url.Values{
		"limit": {strconv.Itoa(p.Limit)},
		"page":  {strconv.Itoa(p.Page)},
	}
}

// Library represents an Audiobookshelf library.
type Library struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	MediaType    string `json:"mediaType"`
	DisplayOrder int    `json:"displayOrder"`
}

// LibraryItem is the polymorphic item type. Media changes shape based on MediaType.
type LibraryItem struct {
	ID        string          `json:"id"`
	MediaType string          `json:"mediaType"`
	Media     json.RawMessage `json:"media"`
}

// AsBook unmarshals the Media field as BookMedia.
func (item *LibraryItem) AsBook() (*BookMedia, error) {
	if item.MediaType != MediaTypeBook {
		return nil, fmt.Errorf("item %s is not a book (mediaType=%s)", item.ID, item.MediaType)
	}
	var m BookMedia
	if err := json.Unmarshal(item.Media, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// AsPodcast unmarshals the Media field as PodcastMedia.
func (item *LibraryItem) AsPodcast() (*PodcastMedia, error) {
	if item.MediaType != MediaTypePodcast {
		return nil, fmt.Errorf("item %s is not a podcast (mediaType=%s)", item.ID, item.MediaType)
	}
	var m PodcastMedia
	if err := json.Unmarshal(item.Media, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// BookMedia contains the media fields for a book library item.
type BookMedia struct {
	Metadata BookMetadata `json:"metadata"`
	Duration float64      `json:"duration"`
	Chapters []Chapter    `json:"chapters,omitempty"`
	Tracks   []AudioTrack `json:"tracks,omitempty"`
}

// Chapter is one chapter of a book. Times are in seconds from the start of
// the book, across all of its audio files.
type Chapter struct {
	ID    int     `json:"id"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Title string  `json:"title"`
}

// AudioTrack is one audio file of a book, placed on the book's timeline.
// Times are in seconds; Title is the file name.
type AudioTrack struct {
	Index       int     `json:"index"`
	Title       string  `json:"title"`
	StartOffset float64 `json:"startOffset"`
	Duration    float64 `json:"duration"`
}

// BookMetadata holds metadata for a book.
type BookMetadata struct {
	Title         string   `json:"title"`
	Subtitle      string   `json:"subtitle,omitempty"`
	AuthorName    string   `json:"authorName"`
	NarratorName  string   `json:"narratorName,omitempty"`
	SeriesName    string   `json:"seriesName,omitempty"`
	Genres        []string `json:"genres,omitempty"`
	Description   string   `json:"description,omitempty"`
	PublishedYear string   `json:"publishedYear,omitempty"`
	Publisher     string   `json:"publisher,omitempty"`
	Language      string   `json:"language,omitempty"`
	ISBN          string   `json:"isbn,omitempty"`
	ASIN          string   `json:"asin,omitempty"`
}

// PodcastMedia contains the media fields for a podcast library item.
type PodcastMedia struct {
	Metadata    PodcastMetadata  `json:"metadata"`
	Episodes    []PodcastEpisode `json:"episodes,omitempty"`
	NumEpisodes int              `json:"numEpisodes,omitempty"`
}

// PodcastMetadata holds metadata for a podcast.
type PodcastMetadata struct {
	Title       string   `json:"title"`
	Author      string   `json:"author,omitempty"`
	Description string   `json:"description,omitempty"`
	ReleaseDate string   `json:"releaseDate,omitempty"`
	Genres      []string `json:"genres,omitempty"`
	Language    string   `json:"language,omitempty"`
}

// PodcastEpisode represents a single podcast episode (minimal fields).
type PodcastEpisode struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Season      string  `json:"season,omitempty"`
	Episode     string  `json:"episode,omitempty"`
	Duration    float64 `json:"duration,omitempty"`
	PublishedAt int64   `json:"publishedAt,omitempty"`
}

// MediaProgress tracks a user's progress on a library item.
type MediaProgress struct {
	ID            string  `json:"id"`
	LibraryItemID string  `json:"libraryItemId"`
	EpisodeID     string  `json:"episodeId,omitempty"`
	Duration      float64 `json:"duration"`
	Progress      float64 `json:"progress"`
	CurrentTime   float64 `json:"currentTime"`
	IsFinished    bool    `json:"isFinished"`
	LastUpdate    int64   `json:"lastUpdate"`
	StartedAt     int64   `json:"startedAt"`
	FinishedAt    int64   `json:"finishedAt,omitempty"`
}

// ListeningSession represents a single listening session.
type ListeningSession struct {
	ID            string  `json:"id"`
	LibraryItemID string  `json:"libraryItemId"`
	EpisodeID     string  `json:"episodeId,omitempty"`
	MediaType     string  `json:"mediaType"`
	DisplayTitle  string  `json:"displayTitle"`
	DisplayAuthor string  `json:"displayAuthor"`
	Duration      float64 `json:"duration"`
	PlayMethod    int     `json:"playMethod"`
	StartedAt     int64   `json:"startedAt"`
	UpdatedAt     int64   `json:"updatedAt"`
}

// ListeningStats contains aggregated listening statistics.
type ListeningStats struct {
	TotalTime      float64              `json:"totalTime"`
	Items          map[string]ItemStats `json:"items"`
	Days           map[string]float64   `json:"days"`
	DayOfWeek      map[string]float64   `json:"dayOfWeek"`
	Today          float64              `json:"today"`
	RecentSessions []ListeningSession   `json:"recentSessions"`
}

// ItemStats tracks per-item listening time.
type ItemStats struct {
	ID            string          `json:"id"`
	TimeListening float64         `json:"timeListening"`
	MediaMetadata json.RawMessage `json:"mediaMetadata,omitempty"`
}

// Author represents an Audiobookshelf author.
type Author struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Description  string        `json:"description,omitempty"`
	NumBooks     int           `json:"numBooks,omitempty"`
	LibraryItems []LibraryItem `json:"libraryItems,omitempty"`
}

// Series represents a book series.
type Series struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Books       []LibraryItem `json:"books,omitempty"`
}

// Collection represents a user-created collection.
type Collection struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Books       []LibraryItem `json:"books,omitempty"`
}

// SearchResults mirrors the ABS search endpoint grouped response.
type SearchResults struct {
	Book    []SearchBookHit    `json:"book"`
	Podcast []SearchPodcastHit `json:"podcast"`
	Authors []SearchAuthorHit  `json:"authors"`
	Series  []SearchSeriesHit  `json:"series"`
}

// SearchBookHit contains a book hit from the search response.
type SearchBookHit struct {
	LibraryItem LibraryItem `json:"libraryItem"`
}

// SearchPodcastHit contains a podcast hit from the search response.
type SearchPodcastHit struct {
	LibraryItem LibraryItem `json:"libraryItem"`
}

// SearchAuthorHit contains an author hit from the search response.
type SearchAuthorHit struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SearchSeriesHit contains a series hit from the search response.
type SearchSeriesHit struct {
	Series Series `json:"series"`
}

// ItemsPage is one page of library items.
type ItemsPage struct {
	Results []LibraryItem `json:"results"`
	Total   int           `json:"total"`
	Page    int           `json:"page"`
}

// SessionsPage is one page of listening sessions.
type SessionsPage struct {
	Sessions []ListeningSession `json:"sessions"`
	Total    int                `json:"total"`
}

// SeriesPage is one page of a library's series.
type SeriesPage struct {
	Results []Series `json:"results"`
	Total   int      `json:"total"`
}

// CollectionsPage is one page of a library's collections.
type CollectionsPage struct {
	Results []Collection `json:"results"`
	Total   int          `json:"total"`
}

// BookMatch is a book a metadata provider such as Audible found for a search.
// Duration is in minutes. MatchConfidence is set only when the search named
// a library item to compare against.
type BookMatch struct {
	Title           string   `json:"title"`
	Subtitle        string   `json:"subtitle"`
	Author          string   `json:"author"`
	Narrator        string   `json:"narrator"`
	ASIN            string   `json:"asin"`
	Language        string   `json:"language"`
	Duration        float64  `json:"duration"`
	Abridged        bool     `json:"abridged"`
	MatchConfidence *float64 `json:"matchConfidence,omitempty"`
}

// AudibleChapters is Audible's chapter list for one edition, as Audnexus
// serves it through Audiobookshelf. Times are in milliseconds.
type AudibleChapters struct {
	ASIN                 string           `json:"asin"`
	Region               string           `json:"region"`
	BrandIntroDurationMs int64            `json:"brandIntroDurationMs"`
	BrandOutroDurationMs int64            `json:"brandOutroDurationMs"`
	RuntimeLengthMs      int64            `json:"runtimeLengthMs"`
	IsAccurate           bool             `json:"isAccurate"`
	Chapters             []AudibleChapter `json:"chapters"`
}

// AudibleChapter is one chapter in an [AudibleChapters] list.
type AudibleChapter struct {
	StartOffsetMs int64  `json:"startOffsetMs"`
	LengthMs      int64  `json:"lengthMs"`
	Title         string `json:"title"`
}
