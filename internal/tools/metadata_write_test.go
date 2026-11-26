package tools

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/sierikov/audiobookshelf-mcp/internal/service"
)

// addedChanges is an add result with one changed and one unchanged book; the
// unchanged one has nil lists, which show as [].
func addedChanges() service.ListChangeResult {
	return service.ListChangeResult{
		Changed: 1,
		Items: []service.ListChange{
			{ItemID: "li_1", Title: "Dune", Previous: []string{"Fantasy"}, Now: []string{"Fantasy", "Science Fiction"}, Values: []string{"Science Fiction"}},
			{ItemID: "li_2", Title: "Emma"},
		},
	}
}

const addedChangesJSON = `{
	"changed":1,
	"items":[
		{"itemId":"li_1","title":"Dune","previous":["Fantasy"],"now":["Fantasy","Science Fiction"],"added":["Science Fiction"]},
		{"itemId":"li_2","title":"Emma","previous":[],"now":[],"added":[]}
	]
}`

// removedChanges is a remove result with one changed and one unchanged book;
// the unchanged one has nil lists, which show as [].
func removedChanges() service.ListChangeResult {
	return service.ListChangeResult{
		Changed: 1,
		Items: []service.ListChange{
			{ItemID: "li_1", Title: "Dune", Previous: []string{"Fantasy", "Science Fiction"}, Now: []string{"Science Fiction"}, Values: []string{"Fantasy"}},
			{ItemID: "li_2", Title: "Emma"},
		},
	}
}

const removedChangesJSON = `{
	"changed":1,
	"items":[
		{"itemId":"li_1","title":"Dune","previous":["Fantasy","Science Fiction"],"now":["Science Fiction"],"removed":["Fantasy"]},
		{"itemId":"li_2","title":"Emma","previous":[],"now":[],"removed":[]}
	]
}`

func TestAddGenres(t *testing.T) {
	svc := NewMockMetadataService(t)
	svc.EXPECT().AddGenres(mock.Anything, []string{"li_1", "li_2"}, []string{"Science Fiction"}).Return(addedChanges(), nil)

	result := call(t, MetadataTools(svc), "add_genres", map[string]any{
		"itemIds": []any{"li_1", "li_2"}, "genres": []any{"Science Fiction"},
	})

	assertJSON(t, addedChangesJSON, result)
}

func TestAddGenres_Error(t *testing.T) {
	svc := NewMockMetadataService(t)
	svc.EXPECT().AddGenres(mock.Anything, []string{"li_1"}, []string{"Fiction"}).Return(service.ListChangeResult{}, errBoom)

	result := call(t, MetadataTools(svc), "add_genres", map[string]any{"itemIds": []any{"li_1"}, "genres": []any{"Fiction"}})

	assertToolError(t, "failed to add genres: ABS API 500: boom", result)
}

func TestRemoveGenres(t *testing.T) {
	svc := NewMockMetadataService(t)
	svc.EXPECT().RemoveGenres(mock.Anything, []string{"li_1", "li_2"}, []string{"Fantasy"}).Return(removedChanges(), nil)
	svc.EXPECT().RemoveGenres(mock.Anything, []string{"li_3"}, []string{"x"}).Return(service.ListChangeResult{}, errBoom)

	result := call(t, MetadataTools(svc), "remove_genres", map[string]any{
		"itemIds": []any{"li_1", "li_2"}, "genres": []any{"Fantasy"},
	})
	assertJSON(t, removedChangesJSON, result)

	result = call(t, MetadataTools(svc), "remove_genres", map[string]any{"itemIds": []any{"li_3"}, "genres": []any{"x"}})
	assertToolError(t, "failed to remove genres: ABS API 500: boom", result)
}

func TestAddTags(t *testing.T) {
	svc := NewMockMetadataService(t)
	svc.EXPECT().AddTags(mock.Anything, []string{"li_1", "li_2"}, []string{"to-read"}).Return(addedChanges(), nil)
	svc.EXPECT().AddTags(mock.Anything, []string{"li_3"}, []string{"x"}).Return(service.ListChangeResult{}, errBoom)

	result := call(t, MetadataTools(svc), "add_tags", map[string]any{
		"itemIds": []any{"li_1", "li_2"}, "tags": []any{"to-read"},
	})
	assertJSON(t, addedChangesJSON, result)

	result = call(t, MetadataTools(svc), "add_tags", map[string]any{"itemIds": []any{"li_3"}, "tags": []any{"x"}})
	assertToolError(t, "failed to add tags: ABS API 500: boom", result)
}

func TestRemoveTags(t *testing.T) {
	svc := NewMockMetadataService(t)
	svc.EXPECT().RemoveTags(mock.Anything, []string{"li_1", "li_2"}, []string{"to-read"}).Return(removedChanges(), nil)
	svc.EXPECT().RemoveTags(mock.Anything, []string{"li_3"}, []string{"x"}).Return(service.ListChangeResult{}, errBoom)

	result := call(t, MetadataTools(svc), "remove_tags", map[string]any{
		"itemIds": []any{"li_1", "li_2"}, "tags": []any{"to-read"},
	})
	assertJSON(t, removedChangesJSON, result)

	result = call(t, MetadataTools(svc), "remove_tags", map[string]any{"itemIds": []any{"li_3"}, "tags": []any{"x"}})
	assertToolError(t, "failed to remove tags: ABS API 500: boom", result)
}

func TestListTools_BadArguments(t *testing.T) {
	for _, tool := range []struct{ name, param string }{
		{"add_genres", "genres"},
		{"remove_genres", "genres"},
		{"add_tags", "tags"},
		{"remove_tags", "tags"},
	} {
		tools := MetadataTools(NewMockMetadataService(t))
		cases := []struct {
			name string
			args map[string]any
			want string
		}{
			{"missing itemIds", map[string]any{tool.param: []any{"a"}}, "missing required parameter: itemIds"},
			{"missing values", map[string]any{"itemIds": []any{"li_1"}}, "missing required parameter: " + tool.param},
		}
		for _, tc := range cases {
			t.Run(tool.name+"/"+tc.name, func(t *testing.T) {
				assertToolError(t, tc.want, call(t, tools, tool.name, tc.args))
			})
		}
	}
}

func TestListTools_ServiceCheckError(t *testing.T) {
	svc := NewMockMetadataService(t)
	svc.EXPECT().AddGenres(mock.Anything, []string{"li_1"}, []string{"Fiction"}).Return(service.ListChangeResult{}, &service.ItemError{
		Position: 1, ItemID: "li_1", Reason: `already has "fiction"; remove it before adding "Fiction"`,
	})

	result := call(t, MetadataTools(svc), "add_genres", map[string]any{"itemIds": []any{"li_1"}, "genres": []any{"Fiction"}})

	assertToolError(t, `failed to add genres: item 1 (li_1): already has "fiction"; remove it before adding "Fiction"`, result)
}

func TestListTools_WriteIncomplete(t *testing.T) {
	svc := NewMockMetadataService(t)
	svc.EXPECT().RemoveGenres(mock.Anything, []string{"li_1", "li_2"}, []string{"Fantasy"}).Return(service.ListChangeResult{
		Items: []service.ListChange{
			{ItemID: "li_1", Title: "Dune", Previous: []string{"Fantasy", "Epic"}},
			{ItemID: "li_2", Title: "Emma"},
		},
	}, fmt.Errorf("%w: %w", service.ErrWriteIncomplete, errBoom))

	result := call(t, MetadataTools(svc), "remove_genres", map[string]any{
		"itemIds": []any{"li_1", "li_2"}, "genres": []any{"Fantasy"},
	})

	require.True(t, result.IsError)
	text := resultText(result)
	prefix := "failed to remove genres: the write failed; some books may already be saved: ABS API 500: boom; previous values: "
	require.True(t, strings.HasPrefix(text, prefix), text)
	assert.JSONEq(t, `[
		{"itemId":"li_1","title":"Dune","previous":["Fantasy","Epic"]},
		{"itemId":"li_2","title":"Emma","previous":[]}
	]`, strings.TrimPrefix(text, prefix))
}

func TestUpdateItems(t *testing.T) {
	svc := NewMockMetadataService(t)
	svc.EXPECT().Update(mock.Anything, []service.ItemChange{
		{ItemID: "li_1", Language: new("Russian")},
		{ItemID: "li_2", Description: new("")},
	}).Return(service.UpdateResult{
		Changed: 2,
		Items: []service.ItemUpdateResult{
			{ItemID: "li_1", Title: "Dune", Previous: service.ItemFields{Language: new("")}, Now: service.ItemFields{Language: new("Russian")}},
			{ItemID: "li_2", Title: "Emma", Previous: service.ItemFields{Description: new("<p>Old.</p>")}, Now: service.ItemFields{Description: new("")}},
		},
	}, nil)

	result := call(t, MetadataTools(svc), "update_items", map[string]any{"items": []any{
		map[string]any{"itemId": "li_1", "language": "Russian"},
		map[string]any{"itemId": "li_2", "description": ""},
	}})

	assertJSON(t, `{
		"changed":2,
		"items":[
			{"itemId":"li_1","title":"Dune","previous":{"language":""},"now":{"language":"Russian"}},
			{"itemId":"li_2","title":"Emma","previous":{"description":"<p>Old.</p>"},"now":{"description":""}}
		]
	}`, result)
}

func TestUpdateItems_BadArguments(t *testing.T) {
	tools := MetadataTools(NewMockMetadataService(t))

	assertToolError(t, "missing required parameter: items", call(t, tools, "update_items", map[string]any{}))
}

func TestUpdateItems_Errors(t *testing.T) {
	changes := []service.ItemChange{{ItemID: "li_1", Language: new("Russian")}}
	args := map[string]any{"items": []any{map[string]any{"itemId": "li_1", "language": "Russian"}}}
	partial := service.UpdateResult{Items: []service.ItemUpdateResult{
		{ItemID: "li_1", Title: "Dune", Previous: service.ItemFields{Language: new("English")}},
	}}
	const previous = `[{"itemId":"li_1","title":"Dune","previous":{"language":"English"}}]`

	cases := []struct {
		name       string
		res        service.UpdateResult
		err        error
		wantPrefix string
		wantJSON   string
	}{
		{"plain error", service.UpdateResult{}, errBoom, "failed to update items: ABS API 500: boom", ""},
		{"plain error with empty items", service.UpdateResult{Items: []service.ItemUpdateResult{}}, errBoom, "failed to update items: ABS API 500: boom", ""},
		{"write incomplete", partial, fmt.Errorf("%w: %w", service.ErrWriteIncomplete, errBoom),
			"failed to update items: the write failed; some books may already be saved: ABS API 500: boom; previous values: ", previous},
		{"read back", partial, fmt.Errorf("%w: %w", service.ErrReadBack, errBoom),
			"failed to update items: the books were saved, but reading them back failed: ABS API 500: boom; previous values: ", previous},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewMockMetadataService(t)
			svc.EXPECT().Update(mock.Anything, changes).Return(tc.res, tc.err)

			result := call(t, MetadataTools(svc), "update_items", args)

			if tc.wantJSON == "" {
				assertToolError(t, tc.wantPrefix, result)
				return
			}
			require.True(t, result.IsError)
			text := resultText(result)
			require.True(t, strings.HasPrefix(text, tc.wantPrefix), text)
			assert.JSONEq(t, tc.wantJSON, strings.TrimPrefix(text, tc.wantPrefix))
		})
	}
}

func TestUpdateItems_Lists(t *testing.T) {
	svc := NewMockMetadataService(t)
	svc.EXPECT().Update(mock.Anything, []service.ItemChange{
		{
			ItemID:    "li_1",
			Series:    new([]service.SeriesEntry{{Name: "Holly Gibney", Sequence: "3"}, {Name: "Bill Hodges"}}),
			Authors:   new([]string{"Stephen King"}),
			Narrators: new([]string{"Will Patton"}),
		},
		{ItemID: "li_2", Series: new([]service.SeriesEntry{}), Authors: new([]string{}), Narrators: new([]string{})},
		{ItemID: "li_3", Language: new("English")},
	}).Return(service.UpdateResult{
		Changed: 3,
		Items: []service.ItemUpdateResult{
			{
				ItemID: "li_1", Title: "Holly",
				Previous: service.ItemFields{
					Series:    new([]service.SeriesEntry{{Name: "Holly Gibney", Sequence: "3"}}),
					Authors:   new([]string{"Stephen King"}),
					Narrators: nil,
				},
				Now: service.ItemFields{
					Series:    new([]service.SeriesEntry{{Name: "Holly Gibney", Sequence: "3"}, {Name: "Bill Hodges"}}),
					Authors:   new([]string{"Stephen King"}),
					Narrators: new([]string{"Will Patton"}),
				},
			},
			{
				ItemID: "li_2", Title: "Emma",
				Previous: service.ItemFields{
					Series:    new([]service.SeriesEntry{{Name: "Cycle", Sequence: "1.5"}}),
					Authors:   new([]string{"Jane Austen"}),
					Narrators: new([]string{"Juliet Stevenson"}),
				},
				Now: service.ItemFields{Series: new([]service.SeriesEntry{}), Authors: new([]string{}), Narrators: new([]string{})},
			},
			{ItemID: "li_3", Title: "Dune", Previous: service.ItemFields{Language: new("")}, Now: service.ItemFields{Language: new("English")}},
		},
	}, nil)

	result := call(t, MetadataTools(svc), "update_items", map[string]any{"items": []any{
		map[string]any{
			"itemId":    "li_1",
			"series":    []any{map[string]any{"name": "Holly Gibney", "sequence": "3"}, map[string]any{"name": "Bill Hodges"}},
			"authors":   []any{"Stephen King"},
			"narrators": []any{"Will Patton"},
		},
		map[string]any{"itemId": "li_2", "series": []any{}, "authors": []any{}, "narrators": []any{}},
		map[string]any{"itemId": "li_3", "language": "English"},
	}})

	// A list that was asked about always shows, as [] when empty; one that
	// was not asked about is left out. Previous and now fit straight back in.
	assertJSON(t, `{
		"changed":3,
		"items":[
			{
				"itemId":"li_1","title":"Holly",
				"previous":{"series":[{"name":"Holly Gibney","sequence":"3"}],"authors":["Stephen King"]},
				"now":{"series":[{"name":"Holly Gibney","sequence":"3"},{"name":"Bill Hodges"}],"authors":["Stephen King"],"narrators":["Will Patton"]}
			},
			{
				"itemId":"li_2","title":"Emma",
				"previous":{"series":[{"name":"Cycle","sequence":"1.5"}],"authors":["Jane Austen"],"narrators":["Juliet Stevenson"]},
				"now":{"series":[],"authors":[],"narrators":[]}
			},
			{"itemId":"li_3","title":"Dune","previous":{"language":""},"now":{"language":"English"}}
		]
	}`, result)
}

func TestUpdateItems_ListsWriteIncomplete(t *testing.T) {
	svc := NewMockMetadataService(t)
	svc.EXPECT().Update(mock.Anything, []service.ItemChange{{ItemID: "li_1", Series: new([]service.SeriesEntry{})}}).
		Return(service.UpdateResult{Items: []service.ItemUpdateResult{{
			ItemID: "li_1", Title: "Holly",
			Previous: service.ItemFields{Series: new([]service.SeriesEntry{{Name: "Holly Gibney", Sequence: "3"}})},
		}}}, fmt.Errorf("%w: %w", service.ErrWriteIncomplete, errBoom))

	result := call(t, MetadataTools(svc), "update_items", map[string]any{"items": []any{
		map[string]any{"itemId": "li_1", "series": []any{}},
	}})

	require.True(t, result.IsError)
	text := resultText(result)
	prefix := "failed to update items: the write failed; some books may already be saved: ABS API 500: boom; previous values: "
	require.True(t, strings.HasPrefix(text, prefix), text)
	assert.JSONEq(t, `[{"itemId":"li_1","title":"Holly","previous":{"series":[{"name":"Holly Gibney","sequence":"3"}]}}]`,
		strings.TrimPrefix(text, prefix))
}

func TestUpdateItems_SeriesSchema(t *testing.T) {
	item := schemaOf(t, "update_items").Properties["items"].Items

	series := item.Properties["series"]
	require.NotNil(t, series)
	assert.Equal(t, "array", series.Type)
	assert.Equal(t, new(10), series.MaxItems)
	assert.Equal(t, "object", series.Items.Type)
	assert.Equal(t, []string{"name"}, series.Items.Required)
	assert.Equal(t, "string", series.Items.Properties["name"].Type)
	assert.Equal(t, "string", series.Items.Properties["sequence"].Type)
	for _, name := range []string{"authors", "narrators"} {
		list := item.Properties[name]
		require.NotNil(t, list, name)
		assert.Equal(t, "array", list.Type, name)
		assert.Equal(t, "string", list.Items.Type, name)
		assert.Equal(t, new(20), list.MaxItems, name)
	}

	// A value equal to the stored one passes at any length, so the limits are
	// in the service and the text, not the schema.
	assert.Nil(t, series.Items.Properties["name"].MaxLength)
	assert.Contains(t, series.Items.Properties["name"].Description, "64 characters")
	assert.Nil(t, series.Items.Properties["sequence"].MaxLength)
	assert.Contains(t, series.Items.Properties["sequence"].Description, "16 characters")
	assert.Nil(t, item.Properties["authors"].Items.MaxLength)
	assert.Contains(t, item.Properties["authors"].Description, "64 characters")
	assert.Nil(t, item.Properties["narrators"].Items.MaxLength)
	assert.Contains(t, item.Properties["narrators"].Description, "64 characters")
}

func TestUpdateItems_SchemaChecksLists(t *testing.T) {
	st := findTool(AllTools(Services{}), "update_items")
	require.NotNil(t, st)
	resolved, err := inputSchema(&st.Tool)
	require.NoError(t, err)
	item := func(field string, value any) map[string]any {
		return map[string]any{"items": []any{map[string]any{"itemId": "li_1", field: value}}}
	}
	repeat := func(n int, v any) []any {
		out := make([]any, n)
		for i := range out {
			out[i] = v
		}
		return out
	}
	long := strings.Repeat("a", 100)

	require.NoError(t, resolved.Validate(item("series", []any{map[string]any{"name": "Holly Gibney", "sequence": "3"}})))
	require.NoError(t, resolved.Validate(item("series", []any{map[string]any{"name": "Holly Gibney"}})))
	require.NoError(t, resolved.Validate(item("series", []any{})), "an empty list removes the series")
	require.NoError(t, resolved.Validate(item("authors", []any{})))
	require.NoError(t, resolved.Validate(item("narrators", []any{})))
	require.NoError(t, resolved.Validate(item("series", repeat(10, map[string]any{"name": "S"}))))
	require.NoError(t, resolved.Validate(item("authors", repeat(20, "A"))))
	require.NoError(t, resolved.Validate(item("narrators", repeat(20, "N"))))
	require.NoError(t, resolved.Validate(item("authors", []any{long})), "a stored value sent back unchanged is for the service to judge")
	require.NoError(t, resolved.Validate(item("series", []any{map[string]any{"name": long, "sequence": long}})))

	require.Error(t, resolved.Validate(item("series", []any{map[string]any{"sequence": "3"}})), "a series needs a name")
	require.Error(t, resolved.Validate(item("series", []any{map[string]any{"name": "S", "bogus": 1}})))
	require.Error(t, resolved.Validate(item("series", []any{"Holly Gibney"})))
	require.Error(t, resolved.Validate(item("series", repeat(11, map[string]any{"name": "S"}))))
	require.Error(t, resolved.Validate(item("authors", repeat(21, "A"))))
	require.Error(t, resolved.Validate(item("narrators", repeat(21, "N"))))
	require.Error(t, resolved.Validate(item("authors", []any{1})))
}

func TestUpdateItems_DescriptionWarnsAboutLists(t *testing.T) {
	st := findTool(MetadataTools(NewMockMetadataService(t)), "update_items")
	require.NotNil(t, st)

	for _, want := range []string{
		"series, authors and narrators replace the book's whole list",
		"an empty list removes them",
		"Audiobookshelf creates a series or author that doesn't exist yet and deletes a series left with no books",
		"Names match regardless of case",
		"Use the exact name of an existing series or author (search_library finds them); a misspelt name creates a new one",
		"Audiobookshelf also deletes an author left with no books unless it has a photo, ASIN or description",
		"Read the book's current lists with get_item and send them back with your change",
		"to undo, call again with them",
	} {
		assert.Contains(t, st.Tool.Description, want)
	}
}

func TestUpdateItems_ItemIDSaysWhereToFindIt(t *testing.T) {
	itemID := schemaOf(t, "update_items").Properties["items"].Items.Properties["itemId"]
	require.NotNil(t, itemID)

	assert.Contains(t, itemID.Description, "(from find_items, search_library or get_series)")
}
