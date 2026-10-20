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

func TestBrowse_ListSeries_CountsBooks(t *testing.T) {
	api := NewMockBrowseAPI(t)
	api.EXPECT().LibrarySeries(mock.Anything, "lib-1", abs.Page{Limit: 20, Page: 1}).Return(abs.SeriesPage{
		Results: []abs.Series{
			{ID: "s-1", Name: "Sword of Truth", Books: []abs.LibraryItem{bookItem("b1", "", "", 0), bookItem("b2", "", "", 0)}},
			{ID: "s-2", Name: "Mistborn"},
		},
		Total: 2,
	}, nil)

	page, err := NewBrowse(api).ListSeries(context.Background(), "lib-1", Page{Limit: 20, Page: 1})

	require.NoError(t, err)
	assert.Equal(t, SeriesPage{
		Series: []SeriesSummary{{ID: "s-1", Name: "Sword of Truth", NumBooks: 2}, {ID: "s-2", Name: "Mistborn"}},
		Total:  2,
	}, page)
}

func TestBrowse_Series_KeepsOnlyDecodableBooks(t *testing.T) {
	api := NewMockBrowseAPI(t)
	api.EXPECT().Series(mock.Anything, "s-1").Return(abs.Series{
		ID: "s-1", Name: "Stormlight Archive", Description: "Epic fantasy",
		Books: []abs.LibraryItem{
			bookItem("b1", "The Way of Kings", "Brandon Sanderson", 190800),
			brokenItem("b2", abs.MediaTypeBook),
			podcastItem("p1", "Not a book", "", 1),
		},
	}, nil)

	detail, err := NewBrowse(api).Series(context.Background(), "s-1")

	require.NoError(t, err)
	assert.Equal(t, SeriesDetail{
		ID: "s-1", Name: "Stormlight Archive", Description: "Epic fantasy",
		Books: []Item{book("b1", "The Way of Kings", "Brandon Sanderson", 53*time.Hour)},
	}, detail)
}

func TestBrowse_Series_WithoutBooksHasNilBooks(t *testing.T) {
	api := NewMockBrowseAPI(t)
	api.EXPECT().Series(mock.Anything, "s-1").Return(abs.Series{ID: "s-1"}, nil)

	detail, err := NewBrowse(api).Series(context.Background(), "s-1")

	require.NoError(t, err)
	assert.Nil(t, detail.Books)
}

func TestBrowse_Author(t *testing.T) {
	api := NewMockBrowseAPI(t)
	api.EXPECT().Author(mock.Anything, "a-1").Return(abs.Author{
		ID: "a-1", Name: "Terry Pratchett", Description: "English author", NumBooks: 41,
		LibraryItems: []abs.LibraryItem{bookItem("b1", "Guards! Guards!", "Terry Pratchett", 36000)},
	}, nil)

	detail, err := NewBrowse(api).Author(context.Background(), "a-1")

	require.NoError(t, err)
	assert.Equal(t, AuthorDetail{
		ID: "a-1", Name: "Terry Pratchett", Description: "English author", NumBooks: 41,
		Books: []Item{book("b1", "Guards! Guards!", "Terry Pratchett", 10*time.Hour)},
	}, detail)
}

func TestBrowse_Collections_CountsBooks(t *testing.T) {
	api := NewMockBrowseAPI(t)
	api.EXPECT().LibraryCollections(mock.Anything, "lib-1", abs.Page{Limit: 20}).Return(abs.CollectionsPage{
		Results: []abs.Collection{
			{ID: "c-1", Name: "Favorites", Description: "My favorites", Books: []abs.LibraryItem{bookItem("b1", "", "", 0)}},
			{ID: "c-2", Name: "To Listen"},
		},
		Total: 2,
	}, nil)

	page, err := NewBrowse(api).Collections(context.Background(), "lib-1", Page{Limit: 20})

	require.NoError(t, err)
	assert.Equal(t, CollectionsPage{
		Collections: []CollectionSummary{
			{ID: "c-1", Name: "Favorites", Description: "My favorites", NumBooks: 1},
			{ID: "c-2", Name: "To Listen"},
		},
		Total: 2,
	}, page)
}

func TestBrowse_ReturnsAPIErrorsUnchanged(t *testing.T) {
	ctx := context.Background()
	api := NewMockBrowseAPI(t)
	api.EXPECT().LibrarySeries(mock.Anything, "lib-1", abs.Page{}).Return(abs.SeriesPage{}, errAPI)
	api.EXPECT().Series(mock.Anything, "s-1").Return(abs.Series{}, errAPI)
	api.EXPECT().Author(mock.Anything, "a-1").Return(abs.Author{}, errAPI)
	api.EXPECT().LibraryCollections(mock.Anything, "lib-1", abs.Page{}).Return(abs.CollectionsPage{}, errAPI)
	svc := NewBrowse(api)

	_, err := svc.ListSeries(ctx, "lib-1", Page{})
	require.Equal(t, errAPI, err)
	_, err = svc.Series(ctx, "s-1")
	require.Equal(t, errAPI, err)
	_, err = svc.Author(ctx, "a-1")
	require.Equal(t, errAPI, err)
	_, err = svc.Collections(ctx, "lib-1", Page{})
	require.Equal(t, errAPI, err)
}
