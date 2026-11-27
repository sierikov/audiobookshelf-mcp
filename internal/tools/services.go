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

// CoversService is what the covers toolset needs from the service layer.
type CoversService interface {
	Find(ctx context.Context, itemID, provider, title, author string) (service.CoverSearch, error)
	Show(ctx context.Context, itemID string, urls []string) (service.CoverSearch, error)
	Set(ctx context.Context, itemID, url string) (service.CoverSet, error)
	Prompt(ctx context.Context, r service.PromptRequest) (service.CoverPrompt, error)
}

// CoverArtService is what the generate_covers and clean_cover tools, and the
// imageId of set_cover, need from the service layer. It makes images with
// Google's image model, which costs money, so a server has one only when it
// has a Gemini API key; [Services].CoverArt is nil otherwise.
type CoverArtService interface {
	Generate(ctx context.Context, r service.GenerateRequest) (service.CoverArtResult, error)
	Clean(ctx context.Context, itemID, instructions string, model service.CoverModel) (service.CoverArtResult, error)
	Set(ctx context.Context, itemID, imageID string) (service.CoverSet, error)
}

// Services are the services the tools call, one per toolset. CoverArt is nil
// without a Gemini API key, and then the tools that make covers are not offered.
// Leave it unset in that case: a nil *service.CoverArt assigned to it is not a
// nil interface, and would offer the paid tools.
type Services struct {
	Libraries LibraryService
	Items     ItemService
	Playback  PlaybackService
	Browse    BrowseService
	Chapters  ChapterService
	Metadata  MetadataService
	Covers    CoversService
	CoverArt  CoverArtService
}
