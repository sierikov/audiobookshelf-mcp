package tools

import (
	"testing"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/sierikov/audiobookshelf-mcp/internal/service"
)

func book(id, title, author string, d time.Duration) service.Item {
	return service.Item{ID: id, MediaType: "book", Title: title, Author: author, Duration: d, HasDuration: true}
}

func podcast(id, title, author string) service.Item {
	return service.Item{ID: id, MediaType: "podcast", Title: title, Author: author}
}

func TestListLibraries(t *testing.T) {
	svc := NewMockLibraryService(t)
	svc.EXPECT().List(mock.Anything).Return([]service.Library{{ID: "lib-1", Name: "Audiobooks", MediaType: "book"}}, nil)

	result := call(t, LibraryTools(svc), "list_libraries", nil)

	assertJSON(t, `[{"id":"lib-1","name":"Audiobooks","mediaType":"book"}]`, result)
}

func TestListLibraries_Empty(t *testing.T) {
	svc := NewMockLibraryService(t)
	svc.EXPECT().List(mock.Anything).Return(nil, nil)

	assertJSON(t, `[]`, call(t, LibraryTools(svc), "list_libraries", nil))
}

func TestListLibraries_Error(t *testing.T) {
	svc := NewMockLibraryService(t)
	svc.EXPECT().List(mock.Anything).Return(nil, errBoom)

	assertToolError(t, "failed to list libraries: ABS API 500: boom", call(t, LibraryTools(svc), "list_libraries", nil))
}

func TestSearchLibrary(t *testing.T) {
	svc := NewMockLibraryService(t)
	svc.EXPECT().Search(mock.Anything, "lib-1", "dune", 10).Return([]service.SearchHit{
		{Type: "book", Item: book("item-1", "Dune", "Frank Herbert", 12*time.Hour)},
		{Type: "podcast", Item: podcast("pod-1", "Dune Pod", "Fans")},
		{Type: service.HitAuthor, Item: service.Item{ID: "a-1", Title: "Frank Herbert"}},
	}, nil)

	result := call(t, LibraryTools(svc), "search_library", map[string]any{"libraryId": "lib-1", "query": "dune"})

	assertJSON(t, `[
		{"type":"book","id":"item-1","title":"Dune","author":"Frank Herbert","duration":"12h 0m"},
		{"type":"podcast","id":"pod-1","title":"Dune Pod","author":"Fans"},
		{"type":"author","id":"a-1","title":"Frank Herbert"}
	]`, result)
}

func TestSearchLibrary_PassesLimit(t *testing.T) {
	svc := NewMockLibraryService(t)
	svc.EXPECT().Search(mock.Anything, "lib-1", "dune", 3).Return([]service.SearchHit{{Type: service.HitSeries, Item: service.Item{ID: "s-1", Title: "Dune"}}}, nil)

	result := call(t, LibraryTools(svc), "search_library", map[string]any{"libraryId": "lib-1", "query": "dune", "limit": float64(3)})

	assertJSON(t, `[{"type":"series","id":"s-1","title":"Dune"}]`, result)
}

func TestSearchLibrary_NoResultsIsText(t *testing.T) {
	svc := NewMockLibraryService(t)
	svc.EXPECT().Search(mock.Anything, "lib-1", "nothing", 10).Return(nil, nil)

	result := call(t, LibraryTools(svc), "search_library", map[string]any{"libraryId": "lib-1", "query": "nothing"})

	assertToolText(t, `No results found for "nothing"`, result)
}

func TestSearchLibrary_InvalidArguments(t *testing.T) {
	tools := LibraryTools(NewMockLibraryService(t))

	assertToolError(t, "missing required parameter: libraryId", call(t, tools, "search_library", map[string]any{"query": "x"}))
	assertToolError(t, "missing required parameter: query", call(t, tools, "search_library", map[string]any{"libraryId": "lib-1"}))
	assertToolError(t, "parameter limit: expected number, got string",
		call(t, tools, "search_library", map[string]any{"libraryId": "lib-1", "query": "x", "limit": "ten"}))
}

func TestSearchLibrary_Error(t *testing.T) {
	svc := NewMockLibraryService(t)
	svc.EXPECT().Search(mock.Anything, "lib-1", "x", 10).Return(nil, errBoom)

	result := call(t, LibraryTools(svc), "search_library", map[string]any{"libraryId": "lib-1", "query": "x"})

	assertToolError(t, "search failed: ABS API 500: boom", result)
}

func TestListLibraryItems_Defaults(t *testing.T) {
	svc := NewMockLibraryService(t)
	svc.EXPECT().Items(mock.Anything, "lib-1", service.Page{Limit: 20, Page: 0}, "").Return(service.ItemsPage{}, nil)

	result := call(t, LibraryTools(svc), "list_library_items", map[string]any{"libraryId": "lib-1"})

	assertJSON(t, `{"items":[],"total":0,"page":0}`, result)
}

func TestListLibraryItems(t *testing.T) {
	svc := NewMockLibraryService(t)
	svc.EXPECT().Items(mock.Anything, "lib-1", service.Page{Limit: 5, Page: 2}, "media.metadata.title").Return(service.ItemsPage{
		Items: []service.Item{
			book("item-1", "Dune", "Frank Herbert", 21*time.Hour),
			book("item-2", "Short", "", 0),
			podcast("pod-1", "Science Friday", "SciFri"),
		},
		Total: 12,
		Page:  2,
	}, nil)

	result := call(t, LibraryTools(svc), "list_library_items", map[string]any{
		"libraryId": "lib-1", "limit": float64(5), "page": float64(2), "sort": "media.metadata.title",
	})

	assertJSON(t, `{"items":[
		{"id":"item-1","mediaType":"book","title":"Dune","author":"Frank Herbert","duration":"21h 0m"},
		{"id":"item-2","mediaType":"book","title":"Short","duration":"< 1m"},
		{"id":"pod-1","mediaType":"podcast","title":"Science Friday","author":"SciFri"}
	],"total":12,"page":2}`, result)
}

func TestListLibraryItems_Error(t *testing.T) {
	svc := NewMockLibraryService(t)
	svc.EXPECT().Items(mock.Anything, "lib-1", service.Page{Limit: 20}, "").Return(service.ItemsPage{}, errBoom)

	result := call(t, LibraryTools(svc), "list_library_items", map[string]any{"libraryId": "lib-1"})

	assertToolError(t, "failed to list items: ABS API 500: boom", result)
}
