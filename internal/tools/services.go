package tools

import (
	"context"

	"github.com/sierikov/audiobookshelf-mcp/internal/service"
)

// LibraryService is what the libraries toolset needs from the service layer.
type LibraryService interface {
	List(ctx context.Context) ([]service.Library, error)
	Search(ctx context.Context, libraryID, query string, limit int) ([]service.SearchHit, error)
	Items(ctx context.Context, libraryID string, page service.Page, sort string) (service.ItemsPage, error)
}

// ItemService is what the items toolset needs from the service layer.
type ItemService interface {
	Get(ctx context.Context, itemID string) (service.ItemDetail, error)
	InProgress(ctx context.Context, limit int) ([]service.InProgressItem, error)
}

// PlaybackService is what the playback toolset needs from the service layer.
type PlaybackService interface {
	Stats(ctx context.Context) (service.Stats, error)
	Sessions(ctx context.Context, limit int) (service.SessionsPage, error)
	Progress(ctx context.Context, itemID, episodeID string) (service.Progress, error)
}

// BrowseService is what the browse toolset needs from the service layer.
type BrowseService interface {
	ListSeries(ctx context.Context, libraryID string, page service.Page) (service.SeriesPage, error)
	Series(ctx context.Context, seriesID string) (service.SeriesDetail, error)
	Author(ctx context.Context, authorID string) (service.AuthorDetail, error)
	Collections(ctx context.Context, libraryID string, page service.Page) (service.CollectionsPage, error)
}

// ChapterService is what the chapters toolset needs from the service layer.
type ChapterService interface {
	Get(ctx context.Context, itemID string) (service.BookChapters, error)
	SearchAudible(ctx context.Context, itemID, region, title, author string) (service.AudibleSearch, error)
	AudibleChapters(ctx context.Context, asin, region string) (service.AudibleChapters, error)
	Set(ctx context.Context, itemID string, chapters []service.ChapterInput) (service.ChapterChange, error)
	Rename(ctx context.Context, itemID string, titles []string) (service.ChapterChange, error)
}

// MetadataService is what the metadata toolset needs from the service layer.
type MetadataService interface {
	Overview(ctx context.Context, libraryID string) (service.MetadataOverview, error)
	Find(ctx context.Context, libraryID string, filter service.ItemFilter, page service.Page) (service.MetadataPage, error)
	SearchBooks(ctx context.Context, itemID, provider, region, title, author string) (service.MetadataSearch, error)
	AddGenres(ctx context.Context, itemIDs, genres []string) (service.ListChangeResult, error)
	RemoveGenres(ctx context.Context, itemIDs, genres []string) (service.ListChangeResult, error)
	AddTags(ctx context.Context, itemIDs, tags []string) (service.ListChangeResult, error)
	RemoveTags(ctx context.Context, itemIDs, tags []string) (service.ListChangeResult, error)
	Update(ctx context.Context, changes []service.ItemChange) (service.UpdateResult, error)
}

// Services are the services the tools call, one per toolset.
type Services struct {
	Libraries LibraryService
	Items     ItemService
	Playback  PlaybackService
	Browse    BrowseService
	Chapters  ChapterService
	Metadata  MetadataService
}
