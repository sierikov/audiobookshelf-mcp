package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
		Covers:    NewMockCoversService(t),
		CoverArt:  NewMockCoverArtService(t),
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
		{"no cover urls", "search_covers", map[string]any{"itemId": "li_1", "urls": []any{}},
			[]string{"invalid arguments for search_covers:", "/properties/urls", "minItems"}},
		{"too many cover urls", "search_covers", map[string]any{"itemId": "li_1", "urls": manyIDs(6)},
			[]string{"invalid arguments for search_covers:", "/properties/urls", "maxItems"}},
		// null is an error until the strict-mode card decides it means "not given".
		{"null limit", "find_items", map[string]any{"libraryId": "lib-1", "limit": nil},
			[]string{"invalid arguments for find_items:", `has type "null"`}},
		{"cover provider outside the enum", "search_covers", map[string]any{"itemId": "li_1", "provider": "bing"},
			[]string{"invalid arguments for search_covers:", "bing"}},
		{"cover image id as a number", "set_cover", map[string]any{"itemId": "li_1", "imageId": 7},
			[]string{"invalid arguments for set_cover:", `want "string"`}},
		{"no covers to make", "generate_covers", map[string]any{"covers": []any{}},
			[]string{"invalid arguments for generate_covers:", "/properties/covers", "minItems"}},
		{"too many covers to make", "generate_covers", map[string]any{"covers": manyCovers(5)},
			[]string{"invalid arguments for generate_covers:", "/properties/covers", "maxItems"}},
		{"cover to make without a style", "generate_covers", map[string]any{"covers": []any{map[string]any{"itemId": "li_1"}}},
			[]string{"invalid arguments for generate_covers:", `missing properties: ["style"]`}},
		{"cover to make without a book", "generate_covers", map[string]any{"covers": []any{map[string]any{"style": "noir"}}},
			[]string{"invalid arguments for generate_covers:", `missing properties: ["itemId"]`}},
		{"style too long", "generate_covers", map[string]any{"covers": []any{map[string]any{"itemId": "li_1", "style": strings.Repeat("a", 301)}}},
			[]string{"invalid arguments for generate_covers:", "maxLength"}},
		{"premise too long", "generate_covers",
			map[string]any{"covers": []any{map[string]any{"itemId": "li_1", "style": "noir", "premise": strings.Repeat("a", 501)}}},
			[]string{"invalid arguments for generate_covers:", "maxLength"}},
		{"unknown key inside a cover to make", "generate_covers",
			map[string]any{"covers": []any{map[string]any{"itemId": "li_1", "style": "noir", "mood": "dark"}}},
			[]string{"invalid arguments for generate_covers:", `unexpected additional properties ["mood"]`}},
		{"title text as a word for the made covers", "generate_covers", map[string]any{"covers": manyCovers(1), "titleText": "no"},
			[]string{"invalid arguments for generate_covers:", `want "boolean"`}},
		{"cleaning without instructions", "clean_cover", map[string]any{"itemId": "li_1"},
			[]string{"invalid arguments for clean_cover:", `missing properties: ["instructions"]`}},
		{"cleaning instructions too long", "clean_cover", map[string]any{"itemId": "li_1", "instructions": strings.Repeat("a", 301)},
			[]string{"invalid arguments for clean_cover:", "maxLength"}},
		{"title text as a word", "get_cover_prompt", map[string]any{"itemId": "li_1", "style": "noir", "titleText": "no"},
			[]string{"invalid arguments for get_cover_prompt:", `want "boolean"`}},
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

// manyCovers returns n covers to make, each of a different book.
func manyCovers(n int) []any {
	covers := make([]any, n)
	for i := range covers {
		covers[i] = map[string]any{"itemId": fmt.Sprintf("li_%d", i), "style": "noir"}
	}
	return covers
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
			map[string]any{"itemId": "li_1", "language": "Russian", "subtitle": "Роман"},
			map[string]any{"itemId": "li_2", "description": "", "subtitle": ""},
		}},
			func(s Services) {
				s.Metadata.(*MockMetadataService).EXPECT().Update(mock.Anything, []service.ItemChange{
					{ItemID: "li_1", Language: new("Russian"), Subtitle: new("Роман")},
					{ItemID: "li_2", Description: new(""), Subtitle: new("")},
				}).Return(service.UpdateResult{}, nil)
			}, ""},
		{"search_covers", map[string]any{"itemId": "li_1", "provider": "audible", "title": "Holly", "author": "Stephen King"},
			func(s Services) {
				s.Covers.(*MockCoversService).EXPECT().Find(mock.Anything, "li_1", "audible", "Holly", "Stephen King").
					Return(service.CoverSearch{ItemID: "li_1", Title: "Holly", Provider: "audible"}, nil)
			}, `"source":"audible"`},
		{"search_covers", map[string]any{"itemId": "li_1"},
			func(s Services) {
				s.Covers.(*MockCoversService).EXPECT().Find(mock.Anything, "li_1", "google", "", "").
					Return(service.CoverSearch{ItemID: "li_1", Title: "Holly", Provider: "google"}, nil)
			}, `"source":"google"`},
		{"get_cover_prompt", map[string]any{"itemId": "li_1", "style": "noir", "premise": "A town in winter.", "titleText": false},
			func(s Services) {
				s.Covers.(*MockCoversService).EXPECT().
					Prompt(mock.Anything, service.PromptRequest{ItemID: "li_1", Style: "noir", Premise: "A town in winter."}).
					Return(service.CoverPrompt{ItemID: "li_1", Title: "Holly", Prompt: "p"}, nil)
			}, `"title":"Holly"`},
		{"get_cover_prompt", map[string]any{"itemId": "li_1", "instructions": "remove the label"},
			func(s Services) {
				s.Covers.(*MockCoversService).EXPECT().
					Prompt(mock.Anything, service.PromptRequest{ItemID: "li_1", Instructions: "remove the label", TitleText: true}).
					Return(service.CoverPrompt{ItemID: "li_1", Title: "Holly", Prompt: "p", Edit: true}, nil)
			}, `"edit":true`},
		{"set_cover", map[string]any{"itemId": "li_1", "url": "https://img.example/1.jpg"},
			func(s Services) {
				s.Covers.(*MockCoversService).EXPECT().Set(mock.Anything, "li_1", "https://img.example/1.jpg").
					Return(service.CoverSet{ItemID: "li_1", Title: "Holly"}, nil)
			}, `"title":"Holly"`},
		{"set_cover", map[string]any{"itemId": "li_1", "imageId": "gen_0123456789abcdef"},
			func(s Services) {
				s.CoverArt.(*MockCoverArtService).EXPECT().Set(mock.Anything, "li_1", "gen_0123456789abcdef").
					Return(service.CoverSet{ItemID: "li_1", Title: "Holly"}, nil)
			}, `"title":"Holly"`},
		{"generate_covers", map[string]any{
			"covers": []any{
				map[string]any{"itemId": "li_1", "style": "noir", "premise": "A town in winter.", "title": "Холли"},
				map[string]any{"itemId": "li_2", "style": "pop art"},
			},
			"titleText": false, "model": "standard", "referenceImageId": "gen_0123456789abcdef",
		},
			func(s Services) {
				s.CoverArt.(*MockCoverArtService).EXPECT().Generate(mock.Anything, service.GenerateRequest{
					Covers: []service.NewCoverRequest{
						{ItemID: "li_1", Style: "noir", Premise: "A town in winter.", Title: "Холли"}, {ItemID: "li_2", Style: "pop art"},
					},
					Model:            service.CoverModelStandard,
					ReferenceImageID: "gen_0123456789abcdef",
				}).Return(service.CoverArtResult{}, nil)
			}, `"source":"gemini"`},
		{"generate_covers", map[string]any{"covers": manyCovers(1), "referenceItemId": "li_9"},
			func(s Services) {
				s.CoverArt.(*MockCoverArtService).EXPECT().Generate(mock.Anything, service.GenerateRequest{
					Covers:          []service.NewCoverRequest{{ItemID: "li_0", Style: "noir"}},
					TitleText:       true,
					ReferenceItemID: "li_9",
				}).Return(service.CoverArtResult{}, nil)
			}, `"source":"gemini"`},
		{"clean_cover", map[string]any{"itemId": "li_1", "instructions": "remove the store label", "model": "standard"},
			func(s Services) {
				s.CoverArt.(*MockCoverArtService).EXPECT().Clean(mock.Anything, "li_1", "remove the store label", service.CoverModelStandard).
					Return(service.CoverArtResult{}, nil)
			}, `"source":"gemini"`},
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
	svcs := mockServices(t) // with the made-cover tools, which need a service
	cs := connectSession(t, NewServer(svcs, Options{Toolsets: []string{"all"}}))
	list, err := cs.ListTools(t.Context(), nil)
	require.NoError(t, err)
	require.Len(t, list.Tools, len(AllTools(svcs)))
	require.Greater(t, len(AllTools(svcs)), len(AllTools(Services{})))

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
	for _, st := range AllTools(Services{CoverArt: NewMockCoverArtService(t)}) {
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

// schemaOf returns the input schema of the named tool.
func schemaOf(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	st := findTool(AllTools(Services{}), name)
	require.NotNil(t, st, name)
	schema, ok := st.Tool.InputSchema.(*jsonschema.Schema)
	require.True(t, ok, name)
	return schema
}

func TestAllTools_SchemaLengthLimits(t *testing.T) {
	// Added genres and tags are always checked, so their schema says the limit.
	assert.Equal(t, new(service.MaxValueLength), schemaOf(t, "add_genres").Properties["genres"].Items.MaxLength)
	assert.Equal(t, new(service.MaxValueLength), schemaOf(t, "add_tags").Properties["tags"].Items.MaxLength)
	assert.Nil(t, schemaOf(t, "remove_genres").Properties["genres"].Items.MaxLength, "removes accept any stored value")
	assert.Nil(t, schemaOf(t, "remove_tags").Properties["tags"].Items.MaxLength, "removes accept any stored value")

	// A language, description or title equal to the stored one passes at any
	// length, so the limit is in the service and the text, not the schema.
	item := schemaOf(t, "update_items").Properties["items"].Items
	assert.Nil(t, item.Properties["language"].MaxLength)
	assert.Contains(t, item.Properties["language"].Description, "64 characters")
	assert.Nil(t, item.Properties["subtitle"].MaxLength)
	assert.Contains(t, item.Properties["subtitle"].Description, "200 characters")
	assert.Nil(t, item.Properties["description"].MaxLength)
	assert.Contains(t, item.Properties["description"].Description, "10,000 characters")
	setTitle := schemaOf(t, "set_chapters").Properties["chapters"].Items.Properties["title"]
	assert.Nil(t, setTitle.MaxLength)
	assert.Contains(t, setTitle.Description, "200 characters")
	renameTitles := schemaOf(t, "rename_chapters").Properties["titles"]
	assert.Nil(t, renameTitles.Items.MaxLength)
	assert.Contains(t, renameTitles.Description, "200 characters")
}
