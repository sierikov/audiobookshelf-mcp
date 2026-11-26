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

func TestItems_Get_Book(t *testing.T) {
	api := NewMockItemsAPI(t)
	api.EXPECT().Item(mock.Anything, "item-1").Return(libraryItem("item-1", abs.MediaTypeBook, abs.BookMedia{
		Metadata: abs.BookMetadata{
			Title: "Katabasis", Subtitle: "A Novel", AuthorName: "R.F. Kuang",
			NarratorName: "Billie Fulford-Brown",
			Genres:       []string{"Fantasy"}, Description: "A descent",
			PublishedYear: "2025", Publisher: "Harper Voyager", Language: "English",
		},
		Duration: 43200,
	}), nil)

	detail, err := NewItems(api).Get(context.Background(), "item-1")

	require.NoError(t, err)
	assert.Equal(t, ItemDetail{
		Item:          book("item-1", "Katabasis", "R.F. Kuang", 12*time.Hour),
		Subtitle:      "A Novel",
		Narrator:      "Billie Fulford-Brown",
		Authors:       []string{},
		Narrators:     []string{},
		Series:        []SeriesEntry{},
		Description:   "A descent",
		PublishedYear: "2025",
		Publisher:     "Harper Voyager",
		Genres:        []string{"Fantasy"},
		Language:      "English",
	}, detail, "a book without authors, narrators or series has empty lists, never nil, so they can be sent straight back")
}

func TestItems_Get_BookLists(t *testing.T) {
	api := NewMockItemsAPI(t)
	api.EXPECT().Item(mock.Anything, "item-1").Return(libraryItem("item-1", abs.MediaTypeBook, abs.BookMedia{
		Metadata: abs.BookMetadata{
			Title: "Joyland", AuthorName: "Stephen King, Richard Bachman", NarratorName: "Michael Kelly, Will Patton",
			SeriesName: "Hard Case Crime #1, Holly Gibney",
			Authors:    []abs.AuthorRef{{ID: "au_1", Name: "Stephen King"}, {ID: "au_2", Name: "Richard Bachman"}},
			Narrators:  []string{"Michael Kelly", "Will Patton"},
			Series: abs.FlexSeries{
				{ID: "se_1", Name: "Hard Case Crime", Sequence: "1"},
				{ID: "se_2", Name: "Holly Gibney"},
			},
		},
		Duration: 36000,
	}), nil)

	detail, err := NewItems(api).Get(context.Background(), "item-1")

	require.NoError(t, err)
	assert.Equal(t, []string{"Stephen King", "Richard Bachman"}, detail.Authors, "names only, in the book's order")
	assert.Equal(t, []string{"Michael Kelly", "Will Patton"}, detail.Narrators)
	assert.Equal(t, []SeriesEntry{{Name: "Hard Case Crime", Sequence: "1"}, {Name: "Holly Gibney"}}, detail.Series,
		"the shape update_items takes: no ids, and no sequence for a book with no number")
	assert.Equal(t, "Stephen King, Richard Bachman", detail.Author, "the display strings stay")
	assert.Equal(t, "Michael Kelly, Will Patton", detail.Narrator)
}

func TestItems_Get_Podcast(t *testing.T) {
	api := NewMockItemsAPI(t)
	api.EXPECT().Item(mock.Anything, "pod-1").Return(libraryItem("pod-1", abs.MediaTypePodcast, abs.PodcastMedia{
		Metadata:    abs.PodcastMetadata{Title: "Hardcore History", Author: "Dan Carlin", Description: "History", Genres: []string{"History"}, Language: "en"},
		NumEpisodes: 68,
		Tags:        []string{"history"},
	}), nil)

	detail, err := NewItems(api).Get(context.Background(), "pod-1")

	require.NoError(t, err)
	assert.Equal(t, ItemDetail{
		Item:        podcast("pod-1", "Hardcore History", "Dan Carlin"),
		Description: "History",
		Genres:      []string{"History"},
		Language:    "en",
		Tags:        []string{"history"},
		NumEpisodes: 68,
	}, detail, "a podcast has no authors, narrators or series lists, not even empty ones")
}

func TestItems_Get_ReturnsAPIErrorUnchanged(t *testing.T) {
	api := NewMockItemsAPI(t)
	api.EXPECT().Item(mock.Anything, "nope").Return(abs.LibraryItem{}, errAPI)

	_, err := NewItems(api).Get(context.Background(), "nope")

	require.Equal(t, errAPI, err)
}

func TestItems_Get_ReturnsMediaErrorForUndecodableMedia(t *testing.T) {
	api := NewMockItemsAPI(t)
	api.EXPECT().Item(mock.Anything, "item-1").Return(brokenItem("item-1", abs.MediaTypeBook), nil)

	_, err := NewItems(api).Get(context.Background(), "item-1")

	var mediaErr *MediaError
	require.ErrorAs(t, err, &mediaErr)
	assert.Equal(t, "item-1", mediaErr.ItemID)
	assert.Equal(t, abs.MediaTypeBook, mediaErr.MediaType)
}

func TestItems_InProgress_AttachesProgressPerItem(t *testing.T) {
	api := NewMockItemsAPI(t)
	api.EXPECT().ItemsInProgress(mock.Anything, 25).Return([]abs.LibraryItem{
		bookItem("item-1", "Katabasis", "R.F. Kuang", 43200),
		podcastItem("pod-1", "Hardcore History", "Dan Carlin", 68),
	}, nil)
	api.EXPECT().Progress(mock.Anything, "item-1", "").Return(abs.MediaProgress{
		Duration: 43200, Progress: 0.25, CurrentTime: 10800, LastUpdate: 1668586015691,
	}, nil)
	api.EXPECT().Progress(mock.Anything, "pod-1", "").Return(abs.MediaProgress{}, errAPI)

	items, err := NewItems(api).InProgress(context.Background(), 25)

	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, book("item-1", "Katabasis", "R.F. Kuang", 12*time.Hour), items[0].Item)
	require.NotNil(t, items[0].Progress)
	assert.InDelta(t, 0.25, items[0].Progress.Fraction, 0)
	assert.Equal(t, 9*time.Hour, items[0].Progress.Remaining)
	assert.Equal(t, time.UnixMilli(1668586015691), items[0].Progress.LastUpdate)

	assert.Equal(t, podcast("pod-1", "Hardcore History", "Dan Carlin"), items[1].Item)
	assert.Nil(t, items[1].Progress, "a failed progress fetch leaves progress unknown, not an error")
}

func TestItems_InProgress_SkipsUndecodableItems(t *testing.T) {
	api := NewMockItemsAPI(t)
	api.EXPECT().ItemsInProgress(mock.Anything, 5).Return([]abs.LibraryItem{brokenItem("item-1", abs.MediaTypeBook)}, nil)
	api.EXPECT().Progress(mock.Anything, mock.Anything, mock.Anything).Return(abs.MediaProgress{}, nil).Maybe()

	items, err := NewItems(api).InProgress(context.Background(), 5)

	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestItems_InProgress_ReturnsAPIErrorUnchanged(t *testing.T) {
	api := NewMockItemsAPI(t)
	api.EXPECT().ItemsInProgress(mock.Anything, 5).Return(nil, errAPI)

	_, err := NewItems(api).InProgress(context.Background(), 5)

	require.Equal(t, errAPI, err)
}

func TestItemsGet_Tags(t *testing.T) {
	api := NewMockItemsAPI(t)
	api.EXPECT().Item(mock.Anything, "item-1").Return(libraryItem("item-1", abs.MediaTypeBook, abs.BookMedia{
		Metadata: abs.BookMetadata{Title: "Katabasis"},
		Tags:     []string{"horror"},
	}), nil)

	detail, err := NewItems(api).Get(context.Background(), "item-1")

	require.NoError(t, err)
	assert.Equal(t, []string{"horror"}, detail.Tags)
}
