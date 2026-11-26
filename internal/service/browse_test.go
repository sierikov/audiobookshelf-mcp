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

// numberedBook is a book whose metadata places it in the given series.
func numberedBook(id, title string, series ...abs.SeriesRef) abs.LibraryItem {
	return libraryItem(id, abs.MediaTypeBook, abs.BookMedia{
		Metadata: abs.BookMetadata{Title: title, Series: series},
		Duration: 3600,
	})
}

func seriesProgress(ids ...string) *abs.SeriesProgress {
	return &abs.SeriesProgress{LibraryItemIDs: ids}
}

// seriesTitles lists the titles of a series' books in order.
func seriesTitles(books []SeriesBook) []string {
	titles := make([]string, 0, len(books))
	for _, b := range books {
		titles = append(titles, b.Title)
	}
	return titles
}

func TestBrowse_Series_BooksInOrder(t *testing.T) {
	api := NewMockBrowseAPI(t)
	api.EXPECT().Series(mock.Anything, "s1").Return(abs.Series{
		ID: "s1", Name: "Holly Gibney", Description: "Crime",
		Progress: seriesProgress("li_c", "li_a", "li_b", "li_d"),
	}, nil)
	api.EXPECT().ItemsByID(mock.Anything, []string{"li_c", "li_a", "li_b", "li_d"}).Return([]abs.LibraryItem{
		numberedBook("li_c", "C", abs.SeriesRef{ID: "other", Name: "Other", Sequence: "9"}, abs.SeriesRef{ID: "s1", Name: "Holly Gibney", Sequence: "3"}),
		numberedBook("li_a", "A", abs.SeriesRef{ID: "s1", Name: "Holly Gibney", Sequence: "1"}),
		numberedBook("li_b", "B", abs.SeriesRef{ID: "s1", Name: "Holly Gibney", Sequence: "1.5"}),
		numberedBook("li_d", "D", abs.SeriesRef{ID: "s1", Name: "Holly Gibney"}),
	}, nil)

	detail, err := NewBrowse(api).Series(context.Background(), "s1")

	require.NoError(t, err)
	assert.Equal(t, SeriesDetail{
		ID: "s1", Name: "Holly Gibney", Description: "Crime",
		Books: []SeriesBook{
			{Item: book("li_a", "A", "", time.Hour), Sequence: "1"},
			{Item: book("li_b", "B", "", time.Hour), Sequence: "1.5"},
			{Item: book("li_c", "C", "", time.Hour), Sequence: "3"},
			{Item: book("li_d", "D", "", time.Hour), Sequence: ""},
		},
	}, detail)
}

func TestBrowse_Series_OrdersNumbersFirstThenTitles(t *testing.T) {
	ref := func(seq string) abs.SeriesRef { return abs.SeriesRef{ID: "s1", Name: "S", Sequence: seq} }
	api := NewMockBrowseAPI(t)
	api.EXPECT().Series(mock.Anything, "s1").Return(abs.Series{
		ID: "s1", Progress: seriesProgress("1", "2", "3", "4", "5", "6", "7", "8", "9"),
	}, nil)
	api.EXPECT().ItemsByID(mock.Anything, mock.Anything).Return([]abs.LibraryItem{
		numberedBook("1", "Zero Hour", ref("")),
		numberedBook("2", "Tenth", ref("10")),
		numberedBook("3", "Prequel", ref("Prequel")),
		numberedBook("4", "Second", ref("2")),
		numberedBook("5", "Origin", ref("0")),
		numberedBook("6", "Another Second", ref("2.0")),
		numberedBook("7", "Annex", ref("Bonus")),
		numberedBook("8", "No Entry", abs.SeriesRef{ID: "other", Name: "Other", Sequence: "1"}),
		numberedBook("9", "Half", ref("0.5")),
	}, nil)

	detail, err := NewBrowse(api).Series(context.Background(), "s1")

	require.NoError(t, err)
	assert.Equal(t, []string{
		"Origin", "Half", "Another Second", "Second", "Tenth", // 0, 0.5, 2 (tie by title), 2, 10
		"Annex", "No Entry", "Prequel", "Zero Hour", // no number or not a number, by title
	}, seriesTitles(detail.Books))
}

func TestBrowse_Series_KeepsOnlyDecodableBooks(t *testing.T) {
	api := NewMockBrowseAPI(t)
	api.EXPECT().Series(mock.Anything, "s-1").Return(abs.Series{
		ID: "s-1", Name: "Stormlight Archive", Description: "Epic fantasy",
		Progress: seriesProgress("b1", "b2", "p1"),
	}, nil)
	api.EXPECT().ItemsByID(mock.Anything, []string{"b1", "b2", "p1"}).Return([]abs.LibraryItem{
		bookItem("b1", "The Way of Kings", "Brandon Sanderson", 190800),
		brokenItem("b2", abs.MediaTypeBook),
		podcastItem("p1", "Not a book", "", 1),
	}, nil)

	detail, err := NewBrowse(api).Series(context.Background(), "s-1")

	require.NoError(t, err)
	assert.Equal(t, SeriesDetail{
		ID: "s-1", Name: "Stormlight Archive", Description: "Epic fantasy",
		Books: []SeriesBook{{Item: book("b1", "The Way of Kings", "Brandon Sanderson", 53*time.Hour)}},
	}, detail)
}

func TestBrowse_Series_SkipsPodcasts(t *testing.T) {
	api := NewMockBrowseAPI(t)
	api.EXPECT().Series(mock.Anything, "s1").Return(abs.Series{ID: "s1", Progress: seriesProgress("b1", "p1")}, nil)
	api.EXPECT().ItemsByID(mock.Anything, []string{"b1", "p1"}).Return([]abs.LibraryItem{
		numberedBook("b1", "A", abs.SeriesRef{ID: "s1", Sequence: "1"}),
		podcastItem("p1", "A podcast", "Host", 12),
	}, nil)

	detail, err := NewBrowse(api).Series(context.Background(), "s1")

	require.NoError(t, err)
	assert.Equal(t, []string{"A"}, seriesTitles(detail.Books))
}

func TestBrowse_Series_NoProgressIsEmpty(t *testing.T) {
	for name, progress := range map[string]*abs.SeriesProgress{
		"missing": nil,
		"no ids":  seriesProgress(),
	} {
		t.Run(name, func(t *testing.T) {
			api := NewMockBrowseAPI(t) // an ItemsByID call would fail the test
			api.EXPECT().Series(mock.Anything, "s-1").Return(abs.Series{ID: "s-1", Name: "Empty", Progress: progress}, nil)

			detail, err := NewBrowse(api).Series(context.Background(), "s-1")

			require.NoError(t, err)
			assert.Equal(t, SeriesDetail{ID: "s-1", Name: "Empty", Books: []SeriesBook{}}, detail)
		})
	}
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

func TestBrowse_Series_ReturnsItemsErrorUnchanged(t *testing.T) {
	api := NewMockBrowseAPI(t)
	api.EXPECT().Series(mock.Anything, "s-1").Return(abs.Series{ID: "s-1", Progress: seriesProgress("b1")}, nil)
	api.EXPECT().ItemsByID(mock.Anything, []string{"b1"}).Return(nil, errAPI)

	_, err := NewBrowse(api).Series(context.Background(), "s-1")

	require.Equal(t, errAPI, err)
}
