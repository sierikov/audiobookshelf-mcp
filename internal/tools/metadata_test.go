package tools

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/sierikov/audiobookshelf-mcp/internal/service"
)

func TestMetadataTools_Annotations(t *testing.T) {
	tools := MetadataTools(NewMockMetadataService(t))

	for _, tool := range tools {
		assert.Equal(t, toolsetMetadata, tool.Toolset.ID, tool.Tool.Name)
	}

	for _, name := range []string{"get_metadata_overview", "find_items"} {
		tool := findTool(tools, name)
		require.NotNil(t, tool, name)
		a := tool.Tool.Annotations
		assert.True(t, a.ReadOnlyHint, name)
		assert.Nil(t, a.DestructiveHint, name)
		assert.False(t, a.IdempotentHint, name)
		require.NotNil(t, a.OpenWorldHint, name)
		assert.False(t, *a.OpenWorldHint, name)
	}

	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Tool.Name)
	}
	assert.Equal(t, []string{
		"get_metadata_overview", "find_items", "search_book_metadata",
		"add_genres", "remove_genres", "add_tags", "remove_tags", "update_items",
	}, names)

	for name, destructive := range map[string]bool{
		"add_genres": false, "add_tags": false, "remove_genres": true, "remove_tags": true, "update_items": true,
	} {
		tool := findTool(tools, name)
		require.NotNil(t, tool, name)
		a := tool.Tool.Annotations
		assert.False(t, a.ReadOnlyHint, name)
		require.NotNil(t, a.DestructiveHint, name)
		assert.Equal(t, destructive, *a.DestructiveHint, name)
		assert.True(t, a.IdempotentHint, name)
		require.NotNil(t, a.OpenWorldHint, name)
		assert.False(t, *a.OpenWorldHint, name)
	}

	search := findTool(tools, "search_book_metadata")
	require.NotNil(t, search)
	a := search.Tool.Annotations
	assert.True(t, a.ReadOnlyHint)
	require.NotNil(t, a.OpenWorldHint)
	assert.True(t, *a.OpenWorldHint)

	schema, ok := search.Tool.InputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	assert.Equal(t, []any{"fantlab", "google", "audible", "openlibrary", "itunes"}, schema.Properties["provider"].Enum)
	assert.Equal(t, []string{"itemId", "provider"}, schema.Required)
	assert.Equal(t, "Audible store region, for the audible provider only (default us)", schema.Properties["region"].Description)
	assert.Equal(t, []any{"us", "ca", "uk", "au", "fr", "de", "jp", "it", "in", "es"}, schema.Properties["region"].Enum)

	find := findTool(tools, "find_items")
	findSchema, ok := find.Tool.InputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	assert.Equal(t, []any{"genres", "tags", "language", "description"}, findSchema.Properties["missing"].Enum)
	assert.Equal(t, []any{"summary", "full"}, findSchema.Properties["detail"].Enum)
	assert.Equal(t, []string{"libraryId"}, findSchema.Required)
}

func TestMetadataTools_LeaveChapterRegionSchemaAlone(t *testing.T) {
	_ = MetadataTools(NewMockMetadataService(t))

	chapters := ChapterTools(NewMockChapterService(t))
	schema, ok := findTool(chapters, "search_audible").Tool.InputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	assert.Equal(t, "Audible store region (default us)", schema.Properties["region"].Description)
}

func TestGetMetadataOverview(t *testing.T) {
	svc := NewMockMetadataService(t)
	svc.EXPECT().Overview(mock.Anything, "lib-1").Return(service.MetadataOverview{
		LibraryID: "lib-1",
		Books:     120,
		Genres:    []service.ValueCount{{Name: "Fantasy", Count: 40}, {Name: "fantasy", Count: 2}},
		Languages: []service.ValueCount{{Name: "English", Count: 100}},
		Missing:   service.MissingCounts{Genres: 3, Tags: 90, Language: 20, Description: 7},
	}, nil)

	result := call(t, MetadataTools(svc), "get_metadata_overview", map[string]any{"libraryId": "lib-1"})

	assertJSON(t, `{
		"libraryId":"lib-1","books":120,
		"genres":[{"name":"Fantasy","count":40},{"name":"fantasy","count":2}],
		"tags":[],
		"languages":[{"name":"English","count":100}],
		"missing":{"genres":3,"tags":90,"language":20,"description":7}
	}`, result)
}

func TestGetMetadataOverview_MissingLibraryID(t *testing.T) {
	result := call(t, MetadataTools(NewMockMetadataService(t)), "get_metadata_overview", map[string]any{})

	assertToolError(t, "missing required parameter: libraryId", result)
}

func TestGetMetadataOverview_Error(t *testing.T) {
	svc := NewMockMetadataService(t)
	svc.EXPECT().Overview(mock.Anything, "lib-1").Return(service.MetadataOverview{}, errBoom)

	result := call(t, MetadataTools(svc), "get_metadata_overview", map[string]any{"libraryId": "lib-1"})

	assertToolError(t, "failed to get metadata overview: ABS API 500: boom", result)
}

func TestFindItems(t *testing.T) {
	svc := NewMockMetadataService(t)
	svc.EXPECT().Find(mock.Anything, "lib-1",
		service.ItemFilter{Genre: "Детектив", Missing: service.MissingDescription},
		service.Page{Limit: 10, Page: 1},
	).Return(service.MetadataPage{
		Items: []service.MetadataItem{
			{
				ID: "li_1", Title: "Дело", Author: "А. Автор", Series: "Цикл", Language: "Russian",
				Genres: []string{"Детектив"}, Tags: []string{"classic"}, Description: "Коротко",
			},
			{ID: "li_2", Title: "Bare", Author: "B"},
		},
		Total: 11,
		Page:  1,
	}, nil)

	result := call(t, MetadataTools(svc), "find_items", map[string]any{
		"libraryId": "lib-1", "genre": "Детектив", "missing": "description", "limit": float64(10), "page": float64(1),
		"detail": "full",
	})

	assertJSON(t, `{
		"items":[
			{"itemId":"li_1","title":"Дело","author":"А. Автор","series":"Цикл","language":"Russian",
			 "genres":["Детектив"],"tags":["classic"],"description":"Коротко","descriptionLength":7},
			{"itemId":"li_2","title":"Bare","author":"B","genres":[],"tags":[],"description":"","descriptionLength":0}
		],
		"total":11,"page":1
	}`, result)
}

func TestFindItems_SummaryByDefault(t *testing.T) {
	svc := NewMockMetadataService(t)
	svc.EXPECT().Find(mock.Anything, "lib-1", service.ItemFilter{Genre: "Детектив"}, service.Page{Limit: 50}).
		Return(service.MetadataPage{
			Items: []service.MetadataItem{
				{
					ID: "li_1", Title: "Дело", Author: "А. Автор", Series: "Цикл", Language: "Russian",
					Genres: []string{"Детектив"}, Tags: []string{"classic"}, Description: "Коротко",
				},
				{ID: "li_2", Title: "Bare", Author: "B"},
			},
			Total: 2,
		}, nil)

	result := call(t, MetadataTools(svc), "find_items", map[string]any{"libraryId": "lib-1", "genre": "Детектив"})

	assertJSON(t, `{
		"items":[
			{"itemId":"li_1","title":"Дело","author":"А. Автор","series":"Цикл","language":"Russian",
			 "genres":["Детектив"],"tags":["classic"]},
			{"itemId":"li_2","title":"Bare","author":"B","genres":[],"tags":[]}
		],
		"total":2,"page":0
	}`, result)
}

func TestFindItems_FullShowsEmptyDescription(t *testing.T) {
	svc := NewMockMetadataService(t)
	svc.EXPECT().Find(mock.Anything, "lib-1", service.ItemFilter{}, service.Page{Limit: 50}).
		Return(service.MetadataPage{Items: []service.MetadataItem{{ID: "li_2", Title: "Bare", Author: "B"}}, Total: 1}, nil)

	result := call(t, MetadataTools(svc), "find_items", map[string]any{"libraryId": "lib-1", "detail": "full"})

	assertJSON(t, `{"items":[{"itemId":"li_2","title":"Bare","author":"B","genres":[],"tags":[],
		"description":"","descriptionLength":0}],"total":1,"page":0}`, result)
}

func TestFindItems_Defaults(t *testing.T) {
	svc := NewMockMetadataService(t)
	svc.EXPECT().Find(mock.Anything, "lib-1", service.ItemFilter{}, service.Page{Limit: 50, Page: 0}).
		Return(service.MetadataPage{}, nil)

	result := call(t, MetadataTools(svc), "find_items", map[string]any{"libraryId": "lib-1"})

	assertJSON(t, `{"items":[],"total":0,"page":0}`, result)
}

func TestFindItems_Preview(t *testing.T) {
	long := strings.Repeat("ж", 250)
	svc := NewMockMetadataService(t)
	svc.EXPECT().Find(mock.Anything, "lib-1", service.ItemFilter{}, service.Page{Limit: 50}).
		Return(service.MetadataPage{
			Items: []service.MetadataItem{
				{ID: "li_1", Title: "Long", Description: long},
				{ID: "li_2", Title: "Edge", Description: strings.Repeat("я", 200)},
				{ID: "li_3", Title: "Short", Description: "short"},
			},
			Total: 3,
		}, nil)

	result := call(t, MetadataTools(svc), "find_items", map[string]any{"libraryId": "lib-1", "detail": "full"})

	require.False(t, result.IsError, resultText(result))
	var out struct {
		Items []struct {
			Description       string `json:"description"`
			DescriptionLength int    `json:"descriptionLength"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(resultText(result)), &out))
	require.Len(t, out.Items, 3)

	assert.True(t, utf8.ValidString(out.Items[0].Description))
	assert.Equal(t, strings.Repeat("ж", 200), out.Items[0].Description)
	assert.Equal(t, 250, out.Items[0].DescriptionLength)

	assert.Equal(t, strings.Repeat("я", 200), out.Items[1].Description)
	assert.Equal(t, 200, out.Items[1].DescriptionLength)

	assert.Equal(t, "short", out.Items[2].Description)
	assert.Equal(t, 5, out.Items[2].DescriptionLength)
}

func TestFindItems_Error(t *testing.T) {
	svc := NewMockMetadataService(t)
	svc.EXPECT().Find(mock.Anything, "lib-1", service.ItemFilter{}, service.Page{Limit: 500}).
		Return(service.MetadataPage{}, fmt.Errorf("%w: limit 500, use 1 to 100", service.ErrBadPage))

	result := call(t, MetadataTools(svc), "find_items", map[string]any{"libraryId": "lib-1", "limit": float64(500)})

	assertToolError(t, "failed to find items: bad page: limit 500, use 1 to 100", result)
}

func TestFindItems_BadArguments(t *testing.T) {
	tools := MetadataTools(NewMockMetadataService(t))

	assertToolError(t, "missing required parameter: libraryId", call(t, tools, "find_items", map[string]any{}))
	assertToolError(t, "parameter limit: expected number, got string",
		call(t, tools, "find_items", map[string]any{"libraryId": "lib-1", "limit": "ten"}))
	assertToolError(t, "parameter genre has wrong type: expected string, got float64",
		call(t, tools, "find_items", map[string]any{"libraryId": "lib-1", "genre": 1.0}))
	assertToolError(t, `detail must be "summary" or "full", got "everything"`,
		call(t, tools, "find_items", map[string]any{"libraryId": "lib-1", "detail": "everything"}))
	assertToolError(t, "parameter detail has wrong type: expected string, got float64",
		call(t, tools, "find_items", map[string]any{"libraryId": "lib-1", "detail": 1.0}))
}

func TestSearchBookMetadata(t *testing.T) {
	svc := NewMockMetadataService(t)
	svc.EXPECT().SearchBooks(mock.Anything, "li_1", "audible", "de", "", "").Return(service.MetadataSearch{
		Book:     service.Item{ID: "li_1", Title: "Dune", Author: "Frank Herbert"},
		Provider: "audible",
		Region:   "de",
		Candidates: []service.MetadataCandidate{
			{
				Title: "Dune", Author: "Frank Herbert", Language: "German",
				Genres: []string{"Science Fiction"}, Tags: []string{"Genre Fiction"},
				Description: "<p>Spice.</p>",
			},
			{Title: "Dune Messiah"},
		},
	}, nil)

	result := call(t, MetadataTools(svc), "search_book_metadata", map[string]any{
		"itemId": "li_1", "provider": "audible", "region": "de",
	})

	assertJSON(t, `{
		"book":{"id":"li_1","title":"Dune","author":"Frank Herbert"},
		"provider":"audible","region":"de",
		"candidates":[
			{"title":"Dune","author":"Frank Herbert","language":"German",
			 "genres":["Science Fiction"],"tags":["Genre Fiction"],"description":"<p>Spice.</p>"},
			{"title":"Dune Messiah","genres":[],"tags":[]}
		]
	}`, result)
}

func TestSearchBookMetadata_NoRegionNoCandidates(t *testing.T) {
	svc := NewMockMetadataService(t)
	svc.EXPECT().SearchBooks(mock.Anything, "li_1", "fantlab", "", "Title", "Author").Return(service.MetadataSearch{
		Book:     service.Item{ID: "li_1", Title: "Title", Author: "Author"},
		Provider: "fantlab",
	}, nil)

	result := call(t, MetadataTools(svc), "search_book_metadata", map[string]any{
		"itemId": "li_1", "provider": "fantlab", "title": "Title", "author": "Author",
	})

	assertJSON(t, `{
		"book":{"id":"li_1","title":"Title","author":"Author"},
		"provider":"fantlab","candidates":[]
	}`, result)
}

func TestSearchBookMetadata_BadArguments(t *testing.T) {
	tools := MetadataTools(NewMockMetadataService(t))

	assertToolError(t, "missing required parameter: itemId", call(t, tools, "search_book_metadata", map[string]any{"provider": "google"}))
	assertToolError(t, "missing required parameter: provider", call(t, tools, "search_book_metadata", map[string]any{"itemId": "li_1"}))
}

func TestSearchBookMetadata_Error(t *testing.T) {
	svc := NewMockMetadataService(t)
	svc.EXPECT().SearchBooks(mock.Anything, "li_1", "google", "", "", "").Return(service.MetadataSearch{}, errBoom)

	result := call(t, MetadataTools(svc), "search_book_metadata", map[string]any{"itemId": "li_1", "provider": "google"})

	assertToolError(t, "metadata search failed: ABS API 500: boom", result)
}
