package tools

import (
	"testing"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/sierikov/audiobookshelf-mcp/internal/service"
)

func TestListSeries(t *testing.T) {
	svc := NewMockBrowseService(t)
	svc.EXPECT().ListSeries(mock.Anything, "lib-1", service.Page{Limit: 20, Page: 0}).Return(service.SeriesPage{
		Series: []service.SeriesSummary{{ID: "s-1", Name: "Sword of Truth", NumBooks: 2}, {ID: "s-2", Name: "Mistborn"}},
		Total:  2,
	}, nil)

	result := call(t, BrowseTools(svc), "list_series", map[string]any{"libraryId": "lib-1"})

	assertJSON(t, `{"series":[
		{"id":"s-1","name":"Sword of Truth","numBooks":2},
		{"id":"s-2","name":"Mistborn","numBooks":0}
	],"total":2}`, result)
}

func TestListSeries_PassesPagingAndRendersEmptyList(t *testing.T) {
	svc := NewMockBrowseService(t)
	svc.EXPECT().ListSeries(mock.Anything, "lib-1", service.Page{Limit: 5, Page: 1}).Return(service.SeriesPage{}, nil)

	result := call(t, BrowseTools(svc), "list_series", map[string]any{"libraryId": "lib-1", "limit": float64(5), "page": float64(1)})

	assertJSON(t, `{"series":[],"total":0}`, result)
}

func TestListSeries_Error(t *testing.T) {
	svc := NewMockBrowseService(t)
	svc.EXPECT().ListSeries(mock.Anything, "lib-1", service.Page{Limit: 20}).Return(service.SeriesPage{}, errBoom)

	result := call(t, BrowseTools(svc), "list_series", map[string]any{"libraryId": "lib-1"})

	assertToolError(t, "failed to list series: ABS API 500: boom", result)
}

func TestGetSeries(t *testing.T) {
	svc := NewMockBrowseService(t)
	svc.EXPECT().Series(mock.Anything, "s-1").Return(service.SeriesDetail{
		ID: "s-1", Name: "Stormlight Archive", Description: "Epic fantasy",
		Books: []service.Item{book("b1", "The Way of Kings", "Brandon Sanderson", 53*time.Hour)},
	}, nil)

	result := call(t, BrowseTools(svc), "get_series", map[string]any{"seriesId": "s-1"})

	assertJSON(t, `{"id":"s-1","name":"Stormlight Archive","description":"Epic fantasy","books":[
		{"id":"b1","mediaType":"book","title":"The Way of Kings","author":"Brandon Sanderson","duration":"53h 0m"}
	]}`, result)
}

func TestGetSeries_WithoutBooksRendersNull(t *testing.T) {
	svc := NewMockBrowseService(t)
	svc.EXPECT().Series(mock.Anything, "s-1").Return(service.SeriesDetail{ID: "s-1", Name: "Empty"}, nil)

	result := call(t, BrowseTools(svc), "get_series", map[string]any{"seriesId": "s-1"})

	assertJSON(t, `{"id":"s-1","name":"Empty","books":null}`, result)
}

func TestGetSeries_Errors(t *testing.T) {
	svc := NewMockBrowseService(t)
	svc.EXPECT().Series(mock.Anything, "s-1").Return(service.SeriesDetail{}, errBoom)
	tools := BrowseTools(svc)

	assertToolError(t, "failed to get series: ABS API 500: boom", call(t, tools, "get_series", map[string]any{"seriesId": "s-1"}))
	assertToolError(t, "missing required parameter: seriesId", call(t, tools, "get_series", map[string]any{}))
}

func TestGetAuthor(t *testing.T) {
	svc := NewMockBrowseService(t)
	svc.EXPECT().Author(mock.Anything, "a-1").Return(service.AuthorDetail{
		ID: "a-1", Name: "Terry Pratchett", Description: "English author", NumBooks: 41,
		Books: []service.Item{book("b1", "Guards! Guards!", "Terry Pratchett", 10*time.Hour)},
	}, nil)

	result := call(t, BrowseTools(svc), "get_author", map[string]any{"authorId": "a-1"})

	assertJSON(t, `{"id":"a-1","name":"Terry Pratchett","description":"English author","numBooks":41,"books":[
		{"id":"b1","mediaType":"book","title":"Guards! Guards!","author":"Terry Pratchett","duration":"10h 0m"}
	]}`, result)
}

func TestGetAuthor_WithoutBooksOmitsBooks(t *testing.T) {
	svc := NewMockBrowseService(t)
	svc.EXPECT().Author(mock.Anything, "a-1").Return(service.AuthorDetail{ID: "a-1", Name: "Nobody"}, nil)

	result := call(t, BrowseTools(svc), "get_author", map[string]any{"authorId": "a-1"})

	assertJSON(t, `{"id":"a-1","name":"Nobody","numBooks":0}`, result)
}

func TestGetAuthor_Error(t *testing.T) {
	svc := NewMockBrowseService(t)
	svc.EXPECT().Author(mock.Anything, "a-1").Return(service.AuthorDetail{}, errBoom)

	result := call(t, BrowseTools(svc), "get_author", map[string]any{"authorId": "a-1"})

	assertToolError(t, "failed to get author: ABS API 500: boom", result)
}

func TestListCollections(t *testing.T) {
	svc := NewMockBrowseService(t)
	svc.EXPECT().Collections(mock.Anything, "lib-1", service.Page{Limit: 20}).Return(service.CollectionsPage{
		Collections: []service.CollectionSummary{
			{ID: "c-1", Name: "Favorites", Description: "My favorites", NumBooks: 1},
			{ID: "c-2", Name: "To Listen"},
		},
		Total: 2,
	}, nil)

	result := call(t, BrowseTools(svc), "list_collections", map[string]any{"libraryId": "lib-1"})

	assertJSON(t, `{"collections":[
		{"id":"c-1","name":"Favorites","description":"My favorites","numBooks":1},
		{"id":"c-2","name":"To Listen","numBooks":0}
	],"total":2}`, result)
}

func TestListCollections_Error(t *testing.T) {
	svc := NewMockBrowseService(t)
	svc.EXPECT().Collections(mock.Anything, "lib-1", service.Page{Limit: 20}).Return(service.CollectionsPage{}, errBoom)

	result := call(t, BrowseTools(svc), "list_collections", map[string]any{"libraryId": "lib-1"})

	assertToolError(t, "failed to list collections: ABS API 500: boom", result)
}
