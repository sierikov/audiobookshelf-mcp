package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/sierikov/audiobookshelf-mcp/internal/service"
)

// mockServices returns strict mocks for every service, so a test fails if a
// handler reaches one it did not expect.
func mockServices(t *testing.T) Services {
	t.Helper()
	return Services{
		Libraries: NewMockLibraryService(t),
		Items:     NewMockItemService(t),
		Playback:  NewMockPlaybackService(t),
		Browse:    NewMockBrowseService(t),
		Chapters:  NewMockChapterService(t),
		Metadata:  NewMockMetadataService(t),
	}
}

// callOverProtocol calls a tool the way a client does, through a server with
// every toolset on. The timeout keeps a handler stopped by an unexpected mock
// call from hanging the test.
func callOverProtocol(t *testing.T, svcs Services, name string, args any) *mcp.CallToolResult {
	t.Helper()
	cs := connectSession(t, NewServer(svcs, Options{Toolsets: []string{"all"}}))
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	return result
}

func TestRegister_RejectsArgumentsTheSchemaDoesNotAllow(t *testing.T) {
	tests := []struct {
		name string
		tool string
		args any
		want []string
	}{
		{"wrong type", "get_item", map[string]any{"itemId": 5}, []string{"invalid arguments for get_item:", `want "string"`}},
		{"list given as a string", "add_genres", map[string]any{"itemIds": []any{"li_1"}, "genres": "Classics"},
			[]string{"invalid arguments for add_genres:", `want "array"`}},
		{"missing required", "add_genres", map[string]any{"genres": []any{"Fantasy"}},
			[]string{"invalid arguments for add_genres:", "itemIds"}},
		{"value outside the enum", "find_items", map[string]any{"libraryId": "lib_1", "detail": "brief"},
			[]string{"invalid arguments for find_items:", "brief"}},
		{"no arguments", "get_item", nil, []string{"invalid arguments for get_item:", "itemId"}},
		{"arguments not an object", "get_item", "li_1", []string{"invalid arguments for get_item: arguments must be an object"}},
		{"items not a list", "update_items", map[string]any{"items": "x"}, []string{"invalid arguments for update_items:", `want "array"`}},
		{"item not an object", "update_items", map[string]any{"items": []any{map[string]any{"itemId": "li_1"}, "li_2"}},
			[]string{"invalid arguments for update_items:", `want "object"`}},
		{"item without itemId", "update_items", map[string]any{"items": []any{map[string]any{"language": "x"}}},
			[]string{"invalid arguments for update_items:", `missing properties: ["itemId"]`}},
		{"null where a string goes", "update_items", map[string]any{"items": []any{map[string]any{"itemId": "li_1", "language": nil}}},
			[]string{"invalid arguments for update_items:", `has type "null"`}},
		{"number in a list of strings", "add_tags", map[string]any{"itemIds": []any{"li_1", 3}, "tags": []any{"a"}},
			[]string{"invalid arguments for add_tags:", `want "string"`}},
		{"number in a list of titles", "rename_chapters", map[string]any{"itemId": "item-1", "titles": []any{"A", 3}},
			[]string{"invalid arguments for rename_chapters:", `want "string"`}},
		{"chapter start as clock text", "set_chapters", map[string]any{"itemId": "item-1", "chapters": []any{map[string]any{"start": "10:00", "title": "A"}}},
			[]string{"invalid arguments for set_chapters:", `want "number"`}},
		{"chapter without title", "set_chapters", map[string]any{"itemId": "item-1", "chapters": []any{map[string]any{"start": 0}}},
			[]string{"invalid arguments for set_chapters:", `missing properties: ["title"]`}},
		{"chapter without start", "set_chapters", map[string]any{"itemId": "item-1", "chapters": []any{map[string]any{"title": "A"}}},
			[]string{"invalid arguments for set_chapters:", `missing properties: ["start"]`}},
		{"limit as text", "search_library", map[string]any{"libraryId": "lib-1", "query": "x", "limit": "ten"},
			[]string{"invalid arguments for search_library:", `want "integer"`}},
		{"fractional limit", "find_items", map[string]any{"libraryId": "lib-1", "limit": 1.9},
			[]string{"invalid arguments for find_items:", `want "integer"`}},
		{"limit below 1", "list_library_items", map[string]any{"libraryId": "lib-1", "limit": 0},
			[]string{"invalid arguments for list_library_items:", "/properties/limit", "minimum"}},
		{"limit above 100", "list_series", map[string]any{"libraryId": "lib-1", "limit": 101},
			[]string{"invalid arguments for list_series:", "/properties/limit", "maximum"}},
		{"negative page", "list_collections", map[string]any{"libraryId": "lib-1", "page": -1},
			[]string{"invalid arguments for list_collections:", "/properties/page", "minimum"}},
		{"no item ids", "add_genres", map[string]any{"itemIds": []any{}, "genres": []any{"Fantasy"}},
			[]string{"invalid arguments for add_genres:", "/properties/itemIds", "minItems"}},
		{"too many item ids", "remove_tags", map[string]any{"itemIds": manyIDs(101), "tags": []any{"x"}},
			[]string{"invalid arguments for remove_tags:", "/properties/itemIds", "maxItems"}},
		{"no values", "add_tags", map[string]any{"itemIds": []any{"li_1"}, "tags": []any{}},
			[]string{"invalid arguments for add_tags:", "/properties/tags", "minItems"}},
		{"no items", "update_items", map[string]any{"items": []any{}},
			[]string{"invalid arguments for update_items:", "/properties/items", "minItems"}},
		// null is an error until the strict-mode card decides it means "not given".
		{"null limit", "find_items", map[string]any{"libraryId": "lib-1", "limit": nil},
			[]string{"invalid arguments for find_items:", `has type "null"`}},
		{"unknown key", "find_items", map[string]any{"libraryId": "lib_1", "genres": []any{"Fantasy"}},
			[]string{`invalid arguments for find_items: unexpected additional properties ["genres"]`}},
		{"unknown key inside an item", "update_items",
			map[string]any{"items": []any{map[string]any{"itemId": "li_1", "language": "Russian", "title": "Solaris"}}},
			[]string{"invalid arguments for update_items:", `unexpected additional properties ["title"]`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callOverProtocol(t, mockServices(t), tt.tool, tt.args)

			require.True(t, result.IsError, resultText(result))
			text := resultText(result)
			for _, want := range tt.want {
				assert.Contains(t, text, want)
			}
			assert.NotContains(t, text, "validating root")
		})
	}
}

// manyIDs returns n distinct item ids.
func manyIDs(n int) []any {
	ids := make([]any, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("li_%d", i)
	}
	return ids
}

func TestAllTools_LimitAndPageAreBoundedIntegers(t *testing.T) {
	for _, st := range AllTools(Services{}) {
		schema, ok := st.Tool.InputSchema.(*jsonschema.Schema)
		require.True(t, ok, st.Tool.Name)
		if limit, ok := schema.Properties["limit"]; ok {
			assert.Equal(t, "integer", limit.Type, "%s limit", st.Tool.Name)
			assert.Equal(t, new(1.0), limit.Minimum, "%s limit", st.Tool.Name)
			assert.Equal(t, new(float64(service.MaxPageLimit)), limit.Maximum, "%s limit", st.Tool.Name)
		}
		if page, ok := schema.Properties["page"]; ok {
			assert.Equal(t, "integer", page.Type, "%s page", st.Tool.Name)
			assert.Equal(t, new(0.0), page.Minimum, "%s page", st.Tool.Name)
		}
	}
}

// TestRegister_ValidCallsReachTheServices makes one valid call per toolset,
// with its optional arguments set, and checks the exact values the service
// receives once Register has let the call through.
func TestRegister_ValidCallsReachTheServices(t *testing.T) {
	tests := []struct {
		tool   string
		args   map[string]any
		expect func(Services)
		want   string
	}{
		{"list_library_items", map[string]any{"libraryId": "lib-1", "limit": 5, "page": 1, "sort": "media.metadata.title"},
			func(s Services) {
				s.Libraries.(*MockLibraryService).EXPECT().
					Items(mock.Anything, "lib-1", service.Page{Limit: 5, Page: 1}, "media.metadata.title").Return(service.ItemsPage{}, nil)
			}, ""},
		{"get_item", map[string]any{"itemId": "li_1"},
			func(s Services) {
				s.Items.(*MockItemService).EXPECT().
					Get(mock.Anything, "li_1").Return(service.ItemDetail{Item: service.Item{ID: "li_1", Title: "Katabasis"}}, nil)
			}, "Katabasis"},
		{"get_media_progress", map[string]any{"itemId": "li_1", "episodeId": "ep_1"},
			func(s Services) {
				s.Playback.(*MockPlaybackService).EXPECT().Progress(mock.Anything, "li_1", "ep_1").Return(service.Progress{}, nil)
			}, ""},
		{"list_collections", map[string]any{"libraryId": "lib-1", "limit": 2},
			func(s Services) {
				s.Browse.(*MockBrowseService).EXPECT().
					Collections(mock.Anything, "lib-1", service.Page{Limit: 2}).Return(service.CollectionsPage{}, nil)
			}, ""},
		{"set_chapters", map[string]any{"itemId": "li_1", "chapters": []any{
			map[string]any{"start": 0, "title": "Prologue"}, map[string]any{"start": 90.5, "title": "One"},
		}},
			func(s Services) {
				s.Chapters.(*MockChapterService).EXPECT().Set(mock.Anything, "li_1", []service.ChapterInput{
					{Start: 0, Title: "Prologue"}, {Start: 90500 * time.Millisecond, Title: "One"},
				}).Return(service.ChapterChange{}, nil)
			}, ""},
		{"find_items", map[string]any{
			"libraryId": "lib-1", "genre": "Fantasy", "tag": "to-read", "language": "English",
			"missing": "description", "limit": 10, "page": 2, "detail": "full",
		},
			func(s Services) {
				s.Metadata.(*MockMetadataService).EXPECT().Find(mock.Anything, "lib-1",
					service.ItemFilter{Genre: "Fantasy", Tag: "to-read", Language: "English", Missing: service.MissingDescription},
					service.Page{Limit: 10, Page: 2}).Return(service.MetadataPage{}, nil)
			}, ""},
		{"update_items", map[string]any{"items": []any{
			map[string]any{"itemId": "li_1", "language": "Russian"}, map[string]any{"itemId": "li_2", "description": ""},
		}},
			func(s Services) {
				s.Metadata.(*MockMetadataService).EXPECT().Update(mock.Anything, []service.ItemChange{
					{ItemID: "li_1", Language: new("Russian")}, {ItemID: "li_2", Description: new("")},
				}).Return(service.UpdateResult{}, nil)
			}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			svcs := mockServices(t)
			tt.expect(svcs)

			result := callOverProtocol(t, svcs, tt.tool, tt.args)

			require.False(t, result.IsError, resultText(result))
			assert.Contains(t, resultText(result), tt.want)
		})
	}
}

func TestRegister_ClientsSeeClosedSchemas(t *testing.T) {
	cs := connectSession(t, NewServer(Services{}, Options{Toolsets: []string{"all"}}))
	list, err := cs.ListTools(t.Context(), nil)
	require.NoError(t, err)
	require.Len(t, list.Tools, len(AllTools(Services{})))

	for _, tool := range list.Tools {
		raw, err := json.Marshal(tool.InputSchema)
		require.NoError(t, err)
		var schema map[string]any
		require.NoError(t, json.Unmarshal(raw, &schema))
		assertObjectsClosed(t, tool.Name, "", schema)
	}
}

// assertObjectsClosed checks that every object schema in node refuses
// properties it doesn't declare.
func assertObjectsClosed(t *testing.T, tool, path string, node any) {
	t.Helper()
	switch n := node.(type) {
	case map[string]any:
		if n["type"] == "object" {
			assert.Equal(t, false, n["additionalProperties"], "%s: object at %q is open", tool, path)
		}
		for key, child := range n {
			assertObjectsClosed(t, tool, path+"/"+key, child)
		}
	case []any:
		for _, child := range n {
			assertObjectsClosed(t, tool, path, child)
		}
	}
}

func TestAllTools_SchemasAcceptOnlyDeclaredKeys(t *testing.T) {
	for _, st := range AllTools(Services{}) {
		name := st.Tool.Name
		resolved, err := inputSchema(&st.Tool)
		require.NoError(t, err, name)
		schema := resolved.Schema()

		require.NoError(t, resolved.Validate(exampleOf(schema)), "%s: an example with every field set", name)

		withBogus, _ := exampleOf(schema).(map[string]any)
		withBogus["bogus"] = 1
		require.ErrorContains(t, resolved.Validate(withBogus), `unexpected additional properties ["bogus"]`, name)

		for key, prop := range schema.Properties {
			if prop.Type != "array" || prop.Items.Type != "object" {
				continue
			}
			nested, _ := exampleOf(schema).(map[string]any)
			nested[key].([]any)[0].(map[string]any)["bogus"] = 1
			require.ErrorContains(t, resolved.Validate(nested), `unexpected additional properties ["bogus"]`, "%s.%s", name, key)
		}
	}
}

// exampleOf builds a value the schema accepts, with every property set.
func exampleOf(s *jsonschema.Schema) any {
	switch {
	case len(s.Enum) > 0:
		return s.Enum[0]
	case s.Type == "object":
		obj := make(map[string]any, len(s.Properties))
		for name, p := range s.Properties {
			obj[name] = exampleOf(p)
		}
		return obj
	case s.Type == "array":
		return []any{exampleOf(s.Items)}
	case s.Type == "number", s.Type == "integer":
		return 1.0
	case s.Type == "boolean":
		return true
	default:
		return "x"
	}
}
