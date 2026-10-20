package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
)

func TestLibraries_List(t *testing.T) {
	api := NewMockLibrariesAPI(t)
	api.EXPECT().Libraries(mock.Anything).Return([]abs.Library{
		{ID: "lib-1", Name: "Audiobooks", MediaType: "book", DisplayOrder: 1},
		{ID: "lib-2", Name: "Podcasts", MediaType: "podcast", DisplayOrder: 2},
	}, nil)

	libs, err := NewLibraries(api).List(context.Background())

	require.NoError(t, err)
	assert.Equal(t, []Library{
		{ID: "lib-1", Name: "Audiobooks", MediaType: "book"},
		{ID: "lib-2", Name: "Podcasts", MediaType: "podcast"},
	}, libs)
}

func TestLibraries_List_ReturnsAPIErrorUnchanged(t *testing.T) {
	api := NewMockLibrariesAPI(t)
	api.EXPECT().Libraries(mock.Anything).Return(nil, errAPI)

	_, err := NewLibraries(api).List(context.Background())

	require.Equal(t, errAPI, err)
}

func TestLibraries_Search_OrdersBooksPodcastsAuthorsSeries(t *testing.T) {
	api := NewMockLibrariesAPI(t)
	api.EXPECT().SearchLibrary(mock.Anything, "lib-1", "dune", 10).Return(abs.SearchResults{
		Series:  []abs.SearchSeriesHit{{Series: abs.Series{ID: "s-1", Name: "Dune Saga"}}},
		Authors: []abs.SearchAuthorHit{{ID: "a-1", Name: "Frank Herbert"}},
		Podcast: []abs.SearchPodcastHit{{LibraryItem: podcastItem("pod-1", "Dune Pod", "Fans", 3)}},
		Book:    []abs.SearchBookHit{{LibraryItem: bookItem("item-1", "Dune", "Frank Herbert", 3600)}},
	}, nil)

	hits, err := NewLibraries(api).Search(context.Background(), "lib-1", "dune", 10)

	require.NoError(t, err)
	assert.Equal(t, []SearchHit{
		{Type: "book", Item: book("item-1", "Dune", "Frank Herbert", time.Hour)},
		{Type: "podcast", Item: podcast("pod-1", "Dune Pod", "Fans")},
		{Type: HitAuthor, Item: Item{ID: "a-1", Title: "Frank Herbert"}},
		{Type: HitSeries, Item: Item{ID: "s-1", Title: "Dune Saga"}},
	}, hits)
}

func TestLibraries_Search_SkipsHitsOfTheWrongOrBrokenMediaType(t *testing.T) {
	api := NewMockLibrariesAPI(t)
	api.EXPECT().SearchLibrary(mock.Anything, "lib-1", "x", 5).Return(abs.SearchResults{
		Book: []abs.SearchBookHit{
			{LibraryItem: podcastItem("pod-1", "Not a book", "", 1)},
			{LibraryItem: brokenItem("item-1", abs.MediaTypeBook)},
		},
		Podcast: []abs.SearchPodcastHit{{LibraryItem: bookItem("item-2", "Not a podcast", "", 1)}},
	}, nil)

	hits, err := NewLibraries(api).Search(context.Background(), "lib-1", "x", 5)

	require.NoError(t, err)
	assert.Empty(t, hits)
}

func TestLibraries_Search_ReturnsAPIErrorUnchanged(t *testing.T) {
	api := NewMockLibrariesAPI(t)
	api.EXPECT().SearchLibrary(mock.Anything, "lib-1", "x", 5).Return(abs.SearchResults{}, errAPI)

	_, err := NewLibraries(api).Search(context.Background(), "lib-1", "x", 5)

	require.Equal(t, errAPI, err)
}

func TestLibraries_Items(t *testing.T) {
	api := NewMockLibrariesAPI(t)
	api.EXPECT().LibraryItems(mock.Anything, "lib-1", abs.Page{Limit: 20, Page: 3}, "media.metadata.title").Return(abs.ItemsPage{
		Results: []abs.LibraryItem{
			bookItem("item-1", "Dune", "Frank Herbert", 0),
			brokenItem("item-2", abs.MediaTypeBook),
			podcastItem("pod-1", "Hardcore History", "Dan Carlin", 68),
			{ID: "odd-1", MediaType: "video"},
		},
		Total: 42,
		Page:  3,
	}, nil)

	page, err := NewLibraries(api).Items(context.Background(), "lib-1", Page{Limit: 20, Page: 3}, "media.metadata.title")

	require.NoError(t, err)
	assert.Equal(t, ItemsPage{
		Items: []Item{
			book("item-1", "Dune", "Frank Herbert", 0),
			podcast("pod-1", "Hardcore History", "Dan Carlin"),
			{ID: "odd-1", MediaType: "video"},
		},
		Total: 42,
		Page:  3,
	}, page)
}

func TestLibraries_Items_ReturnsAPIErrorUnchanged(t *testing.T) {
	api := NewMockLibrariesAPI(t)
	api.EXPECT().LibraryItems(mock.Anything, "lib-1", abs.Page{}, "").Return(abs.ItemsPage{}, errAPI)

	_, err := NewLibraries(api).Items(context.Background(), "lib-1", Page{}, "")

	require.Equal(t, errAPI, err)
}
