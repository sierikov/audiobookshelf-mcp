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
	for _, tool := range []struct{ name, param, noun string }{
		{"add_genres", "genres", "genre"},
		{"remove_genres", "genres", "genre"},
		{"add_tags", "tags", "tag"},
		{"remove_tags", "tags", "tag"},
	} {
		tools := MetadataTools(NewMockMetadataService(t))
		cases := []struct {
			name string
			args map[string]any
			want string
		}{
			{"missing itemIds", map[string]any{tool.param: []any{"a"}}, "missing required parameter: itemIds"},
			{"itemIds not a list", map[string]any{"itemIds": "li_1", tool.param: []any{"a"}},
				"parameter itemIds has wrong type: expected []interface {}, got string"},
			{"itemId not a string", map[string]any{"itemIds": []any{"li_1", 3}, tool.param: []any{"a"}}, "itemId 2 must be a string"},
			{"missing values", map[string]any{"itemIds": []any{"li_1"}}, "missing required parameter: " + tool.param},
			{"value null", map[string]any{"itemIds": []any{"li_1"}, tool.param: []any{nil}}, tool.noun + " 1 must be a string"},
			{"value number", map[string]any{"itemIds": []any{"li_1"}, tool.param: []any{"a", 1}}, tool.noun + " 2 must be a string"},
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
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"missing items", map[string]any{}, "missing required parameter: items"},
		{"items not a list", map[string]any{"items": "x"}, "parameter items has wrong type: expected []interface {}, got string"},
		{"item not an object", map[string]any{"items": []any{map[string]any{"itemId": "li_1"}, "li_2"}}, "item 2 must be an object with itemId"},
		{"item without itemId", map[string]any{"items": []any{map[string]any{"language": "x"}}}, "item 1 must be an object with itemId"},
		{"itemId not a string", map[string]any{"items": []any{map[string]any{"itemId": 5}}}, "item 1: itemId must be a string"},
		{"language null", map[string]any{"items": []any{map[string]any{"itemId": "li_1", "language": nil}}}, "item 1: language must be a string"},
		{"description number", map[string]any{"items": []any{map[string]any{"itemId": "li_1"}, map[string]any{"itemId": "li_2", "description": 3}}},
			"item 2: description must be a string"},
	}
	tools := MetadataTools(NewMockMetadataService(t))
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertToolError(t, tc.want, call(t, tools, "update_items", tc.args))
		})
	}
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
