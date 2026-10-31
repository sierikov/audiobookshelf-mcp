package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sierikov/audiobookshelf-mcp/internal/service"
)

// previousOutput is a book's values from before a write that may have failed,
// where P is the shape of those values.
type previousOutput[P any] struct {
	ItemID   string `json:"itemId"`
	Title    string `json:"title"`
	Previous P      `json:"previous"`
}

// writeFailure is the error result of a write tool. If the service saved some
// books before failing it also returned them, and the error then ends with
// their previous values, so the caller can undo; with no books it is just
// the prefix and the error.
func writeFailure[P any](prefix string, err error, previous []previousOutput[P]) (*mcp.CallToolResult, error) {
	msg := prefix + err.Error()
	if len(previous) > 0 {
		if data, mErr := json.Marshal(previous); mErr == nil {
			msg += "; previous values: " + string(data)
		}
	}
	return errResult(msg)
}

type listChangeOutput struct {
	ItemID   string   `json:"itemId"`
	Title    string   `json:"title"`
	Previous []string `json:"previous"`
	Now      []string `json:"now"`
}

type listChangeResultOutput struct {
	Changed int                `json:"changed"`
	Items   []listChangeOutput `json:"items"`
}

func toListChanges(items []service.ListChange) []listChangeOutput {
	out := make([]listChangeOutput, 0, len(items))
	for _, it := range items {
		out = append(out, listChangeOutput{
			ItemID:   it.ItemID,
			Title:    it.Title,
			Previous: nonNil(it.Previous),
			Now:      nonNil(it.Now),
		})
	}
	return out
}

func toListPrevious(items []service.ListChange) []previousOutput[[]string] {
	out := make([]previousOutput[[]string], 0, len(items))
	for _, it := range items {
		out = append(out, previousOutput[[]string]{ItemID: it.ItemID, Title: it.Title, Previous: nonNil(it.Previous)})
	}
	return out
}

// listWrite describes one of the four tools that add or remove genres or tags.
type listWrite struct {
	name        string
	description string
	// plural names the values parameter, as in "genres"; noun is how one value
	// is named in errors, as in "genre".
	plural      string
	noun        string
	errorPrefix string
	annotations *mcp.ToolAnnotations
	// change is the service method the tool calls, as a method expression, so
	// the service is not touched until the tool is called.
	change func(svc MetadataService, ctx context.Context, itemIDs, values []string) (service.ListChangeResult, error)
}

func (w listWrite) tool(svc MetadataService) ServerTool {
	return ServerTool{
		Toolset: metaMetadata,
		Tool: mcp.Tool{
			Name:        w.name,
			Description: w.description,
			Annotations: w.annotations,
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"itemIds": itemIDsSchema(),
					w.plural:  valuesSchema(w.plural),
				},
				Required: []string{"itemIds", w.plural},
			},
		},
		Handler: func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, error) {
			itemIDs, err := requiredStrings(args, "itemIds", "itemId")
			if err != nil {
				return errResult(err.Error())
			}
			values, err := requiredStrings(args, w.plural, w.noun)
			if err != nil {
				return errResult(err.Error())
			}

			res, err := w.change(svc, ctx, itemIDs, values)
			if err != nil {
				return writeFailure(w.errorPrefix, err, toListPrevious(res.Items))
			}

			return marshalledResult(listChangeResultOutput{Changed: res.Changed, Items: toListChanges(res.Items)})
		},
	}
}

// requiredStrings reads a required list-of-strings parameter; noun is how
// its items are named in errors.
func requiredStrings(args map[string]any, name, noun string) ([]string, error) {
	raw, err := RequiredParam[[]any](args, name)
	if err != nil {
		return nil, err
	}
	return parseStrings(raw, noun)
}

func addGenres(svc MetadataService) ServerTool {
	return listWrite{
		name:        "add_genres",
		description: `Add genres to one or more books (up to 100). Genres a book already has are skipped; a genre it has in another case is an error. Show the user the books and the genres and get their confirmation before calling. To rename a genre, call remove_genres with the old name, then add_genres with the new one. To undo, call remove_genres with the same genres on the books that changed.`,
		plural:      "genres",
		noun:        "genre",
		errorPrefix: "failed to add genres: ",
		annotations: additiveWriteAnnotations(),
		change:      MetadataService.AddGenres,
	}.tool(svc)
}

func removeGenres(svc MetadataService) ServerTool {
	return listWrite{
		name:        "remove_genres",
		description: `Remove genres from one or more books (up to 100). Matching is exact; books without the genre are left unchanged. Show the user the books and the genres and get their confirmation before calling. To undo, call add_genres with the same genres on the books that changed.`,
		plural:      "genres",
		noun:        "genre",
		errorPrefix: "failed to remove genres: ",
		annotations: destructiveWriteAnnotations(),
		change:      MetadataService.RemoveGenres,
	}.tool(svc)
}

func addTags(svc MetadataService) ServerTool {
	return listWrite{
		name:        "add_tags",
		description: `Add tags to one or more books (up to 100). Tags a book already has are skipped; a tag it has in another case is an error. Show the user the books and the tags and get their confirmation before calling. To rename a tag, call remove_tags with the old name, then add_tags with the new one. To undo, call remove_tags with the same tags on the books that changed.`,
		plural:      "tags",
		noun:        "tag",
		errorPrefix: "failed to add tags: ",
		annotations: additiveWriteAnnotations(),
		change:      MetadataService.AddTags,
	}.tool(svc)
}

func removeTags(svc MetadataService) ServerTool {
	return listWrite{
		name:        "remove_tags",
		description: `Remove tags from one or more books (up to 100). Matching is exact; books without the tag are left unchanged. Show the user the books and the tags and get their confirmation before calling. To undo, call add_tags with the same tags on the books that changed.`,
		plural:      "tags",
		noun:        "tag",
		errorPrefix: "failed to remove tags: ",
		annotations: destructiveWriteAnnotations(),
		change:      MetadataService.RemoveTags,
	}.tool(svc)
}

// itemFieldsOutput is a book's language and description. A field that was not
// part of the change is left out; one that was sent and is empty shows as "".
type itemFieldsOutput struct {
	Language    *string `json:"language,omitempty"`
	Description *string `json:"description,omitempty"`
}

type itemUpdateOutput struct {
	ItemID   string           `json:"itemId"`
	Title    string           `json:"title"`
	Previous itemFieldsOutput `json:"previous"`
	Now      itemFieldsOutput `json:"now"`
}

type updateResultOutput struct {
	Changed int                `json:"changed"`
	Items   []itemUpdateOutput `json:"items"`
}

func toUpdateItems(items []service.ItemUpdateResult) []itemUpdateOutput {
	out := make([]itemUpdateOutput, 0, len(items))
	for _, it := range items {
		out = append(out, itemUpdateOutput{
			ItemID:   it.ItemID,
			Title:    it.Title,
			Previous: itemFieldsOutput(it.Previous),
			Now:      itemFieldsOutput(it.Now),
		})
	}
	return out
}

func toUpdatePrevious(items []service.ItemUpdateResult) []previousOutput[itemFieldsOutput] {
	out := make([]previousOutput[itemFieldsOutput], 0, len(items))
	for _, it := range items {
		out = append(out, previousOutput[itemFieldsOutput]{ItemID: it.ItemID, Title: it.Title, Previous: itemFieldsOutput(it.Previous)})
	}
	return out
}

// parseItemChanges reads the items of update_items. Items are numbered from 1
// in errors. A field that is present must be a string, so an explicit null is
// an error and not a request to clear the field.
func parseItemChanges(raw []any) ([]service.ItemChange, error) {
	out := make([]service.ItemChange, 0, len(raw))
	for i, entry := range raw {
		n := i + 1
		obj, ok := entry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("item %d must be an object with itemId", n)
		}
		rawID, ok := obj["itemId"]
		if !ok {
			return nil, fmt.Errorf("item %d must be an object with itemId", n)
		}
		id, ok := rawID.(string)
		if !ok {
			return nil, fmt.Errorf("item %d: itemId must be a string", n)
		}
		language, hasLanguage, err := optionalString(obj, "language", n)
		if err != nil {
			return nil, err
		}
		description, hasDescription, err := optionalString(obj, "description", n)
		if err != nil {
			return nil, err
		}
		change := service.ItemChange{ItemID: id}
		if hasLanguage {
			change.Language = &language
		}
		if hasDescription {
			change.Description = &description
		}
		out = append(out, change)
	}
	return out, nil
}

// optionalString reads a field of item n that may be left out, and must be a
// string if present. present tells a field sent as "" from one left out.
func optionalString(obj map[string]any, name string, n int) (value string, present bool, err error) {
	raw, ok := obj[name]
	if !ok {
		return "", false, nil
	}
	value, ok = raw.(string)
	if !ok {
		return "", false, fmt.Errorf("item %d: %s must be a string", n, name)
	}
	return value, true, nil
}

func updateItems(svc MetadataService) ServerTool {
	return ServerTool{
		Toolset: metaMetadata,
		Tool: mcp.Tool{
			Name:        "update_items",
			Description: `Set the language and/or description of one or more books (up to 100, at most 10 descriptions per call). Fields left out are not touched; an empty string clears a field. Descriptions may use simple HTML: p, br, b, i, em, strong, ul, ol, li, a. Show the user every change, per book from old to new, and get their confirmation before calling. For genres and tags use add_genres, remove_genres, add_tags and remove_tags. Returns each book's previous values; to undo, call again with them. Example items: [{"itemId":"li_1","language":"Russian"},{"itemId":"li_2","description":"<p>A blurb.</p>"}]`,
			Annotations: destructiveWriteAnnotations(),
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"items": {
						Type:        "array",
						Description: "The books to change, at most 100",
						Items: &jsonschema.Schema{
							Type: "object",
							Properties: map[string]*jsonschema.Schema{
								"itemId":      {Type: "string", Description: "Library item ID of the book (from find_items)"},
								"language":    {Type: "string", Description: "New language; an empty string clears it"},
								"description": {Type: "string", Description: "New description, simple HTML; an empty string clears it"},
							},
							Required: []string{"itemId"},
						},
					},
				},
				Required: []string{"items"},
			},
		},
		Handler: func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, error) {
			raw, err := RequiredParam[[]any](args, "items")
			if err != nil {
				return errResult(err.Error())
			}
			changes, err := parseItemChanges(raw)
			if err != nil {
				return errResult(err.Error())
			}

			res, err := svc.Update(ctx, changes)
			if err != nil {
				return writeFailure("failed to update items: ", err, toUpdatePrevious(res.Items))
			}

			return marshalledResult(updateResultOutput{Changed: res.Changed, Items: toUpdateItems(res.Items)})
		},
	}
}
