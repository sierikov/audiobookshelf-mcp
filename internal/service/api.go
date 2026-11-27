// Package service holds the application logic of the Audiobookshelf MCP
// server: it composes Audiobookshelf API calls and returns domain types.
//
// Services know nothing about MCP or presentation. Durations are
// [time.Duration], timestamps are [time.Time] and progress is a 0–1 fraction;
// formatting them for people is the caller's job. Services that write check
// their input first and report problems as domain errors, before any call
// reaches Audiobookshelf. Each service declares the small slice of the
// Audiobookshelf API it needs as an interface, satisfied by [abs.Client] and
// by generated mocks in tests.
package service

import (
	"context"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
	"github.com/sierikov/audiobookshelf-mcp/internal/gemini"
)

// LibrariesAPI is the part of the Audiobookshelf API the Libraries service uses.
type LibrariesAPI interface {
	Libraries(ctx context.Context) ([]abs.Library, error)
	SearchLibrary(ctx context.Context, libraryID, query string, limit int) (abs.SearchResults, error)
	LibraryItems(ctx context.Context, libraryID string, page abs.Page, sort string) (abs.ItemsPage, error)
}

// ItemsAPI is the part of the Audiobookshelf API the Items service uses.
type ItemsAPI interface {
	Item(ctx context.Context, itemID string) (abs.LibraryItem, error)
	ItemsInProgress(ctx context.Context, limit int) ([]abs.LibraryItem, error)
	Progress(ctx context.Context, itemID, episodeID string) (abs.MediaProgress, error)
}

// PlaybackAPI is the part of the Audiobookshelf API the Playback service uses.
type PlaybackAPI interface {
	ListeningStats(ctx context.Context) (abs.ListeningStats, error)
	ListeningSessions(ctx context.Context, limit int) (abs.SessionsPage, error)
	Progress(ctx context.Context, itemID, episodeID string) (abs.MediaProgress, error)
}

// BrowseAPI is the part of the Audiobookshelf API the Browse service uses.
type BrowseAPI interface {
	LibrarySeries(ctx context.Context, libraryID string, page abs.Page) (abs.SeriesPage, error)
	Series(ctx context.Context, seriesID string) (abs.Series, error)
	ItemsByID(ctx context.Context, ids []string) ([]abs.LibraryItem, error)
	Author(ctx context.Context, authorID string) (abs.Author, error)
	LibraryCollections(ctx context.Context, libraryID string, page abs.Page) (abs.CollectionsPage, error)
}

var _ BrowseAPI = (*abs.Client)(nil)

// ChaptersAPI is the part of the Audiobookshelf API the Chapters service uses.
type ChaptersAPI interface {
	Item(ctx context.Context, itemID string) (abs.LibraryItem, error)
	SearchBooks(ctx context.Context, provider, title, author, itemID string) ([]abs.BookMatch, error)
	AudibleChapters(ctx context.Context, asin, region string) (abs.AudibleChapters, error)
	SaveChapters(ctx context.Context, itemID string, chapters []abs.Chapter) (bool, error)
}

var _ ChaptersAPI = (*abs.Client)(nil)

// MetadataAPI is the part of the Audiobookshelf API the Metadata service uses.
type MetadataAPI interface {
	LibraryItems(ctx context.Context, libraryID string, page abs.Page, sort string) (abs.ItemsPage, error)
	Item(ctx context.Context, itemID string) (abs.LibraryItem, error)
	ItemsByID(ctx context.Context, ids []string) ([]abs.LibraryItem, error)
	UpdateItems(ctx context.Context, updates []abs.ItemUpdate) (int, error)
	SearchBooks(ctx context.Context, provider, title, author, itemID string) ([]abs.BookMatch, error)
}

var _ MetadataAPI = (*abs.Client)(nil)

// CoversAPI is the part of the Audiobookshelf API the Covers service uses.
type CoversAPI interface {
	Item(ctx context.Context, itemID string) (abs.LibraryItem, error)
	SearchCovers(ctx context.Context, provider, title, author string) ([]string, error)
	ItemCover(ctx context.Context, itemID string, width int) (abs.Image, error)
	SetCoverURL(ctx context.Context, itemID, url string) error
}

var _ CoversAPI = (*abs.Client)(nil)

// CoverArtAPI is the part of the Audiobookshelf API the CoverArt service uses.
type CoverArtAPI interface {
	Item(ctx context.Context, itemID string) (abs.LibraryItem, error)
	ItemCover(ctx context.Context, itemID string, width int) (abs.Image, error)
	SetCoverFile(ctx context.Context, itemID string, img abs.Image) error
}

var _ CoverArtAPI = (*abs.Client)(nil)

// ImageModel is the image model the CoverArt service makes covers with,
// satisfied by [gemini.Client] and by generated mocks in tests.
type ImageModel interface {
	GenerateImage(ctx context.Context, r gemini.Request) (gemini.Result, error)
}

var _ ImageModel = (*gemini.Client)(nil)
