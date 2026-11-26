package tools

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/sierikov/audiobookshelf-mcp/internal/service"
)

func TestGetItem_Book(t *testing.T) {
	svc := NewMockItemService(t)
	svc.EXPECT().Get(mock.Anything, "item-1").Return(service.ItemDetail{
		Item:          book("item-1", "Katabasis", "R.F. Kuang", 12*time.Hour),
		Subtitle:      "A Novel",
		Narrator:      "Billie Fulford-Brown",
		Authors:       []string{"R.F. Kuang"},
		Narrators:     []string{"Billie Fulford-Brown"},
		Series:        []service.SeriesEntry{{Name: "Standalone"}},
		Description:   "A descent",
		PublishedYear: "2025",
		Publisher:     "Harper Voyager",
		Genres:        []string{"Fantasy", "Fiction"},
		Tags:          []string{"horror"},
		Language:      "English",
	}, nil)

	result := call(t, ItemTools(svc), "get_item", map[string]any{"itemId": "item-1"})

	assertJSON(t, `{
		"id":"item-1","mediaType":"book","title":"Katabasis","subtitle":"A Novel",
		"author":"R.F. Kuang","narrator":"Billie Fulford-Brown",
		"authors":["R.F. Kuang"],"narrators":["Billie Fulford-Brown"],"series":[{"name":"Standalone"}],
		"description":"A descent","duration":"12h 0m","publishedYear":"2025",
		"publisher":"Harper Voyager","genres":["Fantasy","Fiction"],"tags":["horror"],"language":"English"
	}`, result)
	assert.NotContains(t, resultText(result), "seriesName")
}

func TestGetItem_BookListsAreTheShapeUpdateItemsTakes(t *testing.T) {
	svc := NewMockItemService(t)
	svc.EXPECT().Get(mock.Anything, "item-1").Return(service.ItemDetail{
		Item:      book("item-1", "Joyland", "Stephen King, Richard Bachman", 10*time.Hour),
		Narrator:  "Michael Kelly, Will Patton",
		Authors:   []string{"Stephen King", "Richard Bachman"},
		Narrators: []string{"Michael Kelly", "Will Patton"},
		Series:    []service.SeriesEntry{{Name: "Hard Case Crime", Sequence: "1"}, {Name: "Holly Gibney"}},
	}, nil)

	result := call(t, ItemTools(svc), "get_item", map[string]any{"itemId": "item-1"})

	assertJSON(t, `{
		"id":"item-1","mediaType":"book","title":"Joyland",
		"author":"Stephen King, Richard Bachman","narrator":"Michael Kelly, Will Patton",
		"authors":["Stephen King","Richard Bachman"],"narrators":["Michael Kelly","Will Patton"],
		"series":[{"name":"Hard Case Crime","sequence":"1"},{"name":"Holly Gibney"}],
		"duration":"10h 0m"
	}`, result)

	// Sent straight back, the lists pass update_items' schema as they are.
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(resultText(result)), &got))
	item := map[string]any{"itemId": "item-1"}
	for _, field := range []string{"authors", "narrators", "series"} {
		item[field] = got[field]
	}
	updateItems := findTool(MetadataTools(NewMockMetadataService(t)), "update_items")
	require.NotNil(t, updateItems)
	resolved, err := inputSchema(&updateItems.Tool)
	require.NoError(t, err)
	require.NoError(t, resolved.Validate(map[string]any{"items": []any{item}}))
}

func TestGetItem_BookWithoutListsShowsEmptyLists(t *testing.T) {
	svc := NewMockItemService(t)
	svc.EXPECT().Get(mock.Anything, "item-1").Return(service.ItemDetail{
		Item:      book("item-1", "Anonymous", "", time.Hour),
		Authors:   []string{},
		Narrators: []string{},
		Series:    []service.SeriesEntry{},
	}, nil)

	result := call(t, ItemTools(svc), "get_item", map[string]any{"itemId": "item-1"})

	assertJSON(t, `{
		"id":"item-1","mediaType":"book","title":"Anonymous","duration":"1h 0m",
		"authors":[],"narrators":[],"series":[]
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
	for _, field := range []string{`"authors"`, `"narrators"`, `"series"`} {
		assert.NotContains(t, resultText(result), field, "a podcast has no such lists")
	}
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
