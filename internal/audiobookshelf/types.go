package audiobookshelf

import (
	"encoding/json"
	"fmt"
)

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
	if item.MediaType != "book" {
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
	if item.MediaType != "podcast" {
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
}

// PodcastMedia contains the media fields for a podcast library item.
type PodcastMedia struct {
	Metadata      PodcastMetadata `json:"metadata"`
	Episodes      []PodcastEpisode `json:"episodes,omitempty"`
	NumEpisodes   int              `json:"numEpisodes,omitempty"`
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
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	Season    string  `json:"season,omitempty"`
	Episode   string  `json:"episode,omitempty"`
	Duration  float64 `json:"duration,omitempty"`
	PublishedAt int64 `json:"publishedAt,omitempty"`
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
	TotalTime      float64                `json:"totalTime"`
	Items          map[string]ItemStats   `json:"items"`
	Days           map[string]float64     `json:"days"`
	DayOfWeek      map[string]float64     `json:"dayOfWeek"`
	Today          float64                `json:"today"`
	RecentSessions []ListeningSession     `json:"recentSessions"`
}

// ItemStats tracks per-item listening time.
type ItemStats struct {
	ID            string  `json:"id"`
	TimeListening float64 `json:"timeListening"`
	MediaMetadata json.RawMessage `json:"mediaMetadata,omitempty"`
}

// Author represents an Audiobookshelf author.
type Author struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	NumBooks    int           `json:"numBooks,omitempty"`
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

type SearchBookHit struct {
	LibraryItem LibraryItem `json:"libraryItem"`
}

type SearchPodcastHit struct {
	LibraryItem LibraryItem `json:"libraryItem"`
}

type SearchAuthorHit struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type SearchSeriesHit struct {
	Series Series `json:"series"`
}
