package tools

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/sierikov/audiobookshelf-mcp/internal/service"
)

func TestGetItem_Book(t *testing.T) {
	svc := NewMockItemService(t)
	svc.EXPECT().Get(mock.Anything, "item-1").Return(service.ItemDetail{
		Item:          book("item-1", "Katabasis", "R.F. Kuang", 12*time.Hour),
		Subtitle:      "A Novel",
		Narrator:      "Billie Fulford-Brown",
		SeriesName:    "Standalone",
		Description:   "A descent",
		PublishedYear: "2025",
		Publisher:     "Harper Voyager",
		Genres:        []string{"Fantasy", "Fiction"},
		Language:      "English",
	}, nil)

	result := call(t, ItemTools(svc), "get_item", map[string]any{"itemId": "item-1"})

	assertJSON(t, `{
		"id":"item-1","mediaType":"book","title":"Katabasis","subtitle":"A Novel",
		"author":"R.F. Kuang","narrator":"Billie Fulford-Brown","seriesName":"Standalone",
		"description":"A descent","duration":"12h 0m","publishedYear":"2025",
		"publisher":"Harper Voyager","genres":["Fantasy","Fiction"],"language":"English"
	}`, result)
}

func TestGetItem_PodcastHasNoDuration(t *testing.T) {
	svc := NewMockItemService(t)
	svc.EXPECT().Get(mock.Anything, "pod-1").Return(service.ItemDetail{
		Item:        podcast("pod-1", "Hardcore History", "Dan Carlin"),
		NumEpisodes: 68,
	}, nil)

	result := call(t, ItemTools(svc), "get_item", map[string]any{"itemId": "pod-1"})

	assertJSON(t, `{"id":"pod-1","mediaType":"podcast","title":"Hardcore History","author":"Dan Carlin","numEpisodes":68}`, result)
}

func TestGetItem_MissingItemID(t *testing.T) {
	result := call(t, ItemTools(NewMockItemService(t)), "get_item", map[string]any{})

	assertToolError(t, "missing required parameter: itemId", result)
}

func TestGetItem_Error(t *testing.T) {
	svc := NewMockItemService(t)
	svc.EXPECT().Get(mock.Anything, "nope").Return(service.ItemDetail{}, errBoom)

	result := call(t, ItemTools(svc), "get_item", map[string]any{"itemId": "nope"})

	assertToolError(t, "failed to get item: ABS API 500: boom", result)
}

func TestGetItem_UndecodableMedia(t *testing.T) {
	svc := NewMockItemService(t)
	svc.EXPECT().Get(mock.Anything, "item-1").Return(service.ItemDetail{},
		&service.MediaError{ItemID: "item-1", MediaType: "book", Err: errors.New("bad json")})

	result := call(t, ItemTools(svc), "get_item", map[string]any{"itemId": "item-1"})

	assertToolError(t, "failed to parse book media: bad json", result)
}

func TestGetItemsInProgress(t *testing.T) {
	svc := NewMockItemService(t)
	svc.EXPECT().InProgress(mock.Anything, 25).Return([]service.InProgressItem{
		{
			Item:     book("item-1", "Katabasis", "R.F. Kuang", 12*time.Hour),
			Progress: &service.Progress{Fraction: 0.25, Remaining: 9 * time.Hour},
		},
		{
			Item:     book("item-2", "Done", "", time.Hour),
			Progress: &service.Progress{Fraction: 1, Remaining: 0},
		},
		{Item: podcast("pod-1", "Hardcore History", "Dan Carlin")},
	}, nil)

	result := call(t, ItemTools(svc), "get_items_in_progress", map[string]any{})

	assertJSON(t, `[
		{"id":"item-1","title":"Katabasis","author":"R.F. Kuang","duration":"12h 0m","progress":"25.0%","timeRemaining":"9h 0m"},
		{"id":"item-2","title":"Done","duration":"1h 0m","progress":"100.0%"},
		{"id":"pod-1","title":"Hardcore History","author":"Dan Carlin","progress":""}
	]`, result)
}

func TestGetItemsInProgress_PassesLimitAndRendersEmptyList(t *testing.T) {
	svc := NewMockItemService(t)
	svc.EXPECT().InProgress(mock.Anything, 5).Return(nil, nil)

	result := call(t, ItemTools(svc), "get_items_in_progress", map[string]any{"limit": float64(5)})

	assertJSON(t, `[]`, result)
}

func TestGetItemsInProgress_Error(t *testing.T) {
	svc := NewMockItemService(t)
	svc.EXPECT().InProgress(mock.Anything, 25).Return(nil, errBoom)

	result := call(t, ItemTools(svc), "get_items_in_progress", map[string]any{})

	assertToolError(t, "failed to get items in progress: ABS API 500: boom", result)
}
