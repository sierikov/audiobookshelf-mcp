package tools

import (
	"context"

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
		if data, mErr := marshalJSON(previous); mErr == nil {
			msg += "; previous values: " + string(data)
		}
	}
	return errResult(msg)
}

// listChangeOutput is one book's outcome of a list tool. The add tools set
// Added and the remove tools set Removed, always non-nil; omitzero leaves out
// the nil one, while an empty list still shows as [].
type listChangeOutput struct {
	ItemID   string   `json:"itemId"`
	Title    string   `json:"title"`
	Previous []string `json:"previous"`
	Now      []string `json:"now"`
	Added    []string `json:"added,omitzero"`
	Removed  []string `json:"removed,omitzero"`
}

type listChangeResultOutput struct {
	Changed int                `json:"changed"`
	Items   []listChangeOutput `json:"items"`
}

// toListChanges maps a list tool's books, listing each book's values under
// removed if removes is set and under added otherwise.
func toListChanges(items []service.ListChange, removes bool) []listChangeOutput {
	out := make([]listChangeOutput, 0, len(items))
	for _, it := range items {
		o := listChangeOutput{
			ItemID:   it.ItemID,
			Title:    it.Title,
			Previous: nonNil(it.Previous),
			Now:      nonNil(it.Now),
		}
		if removes {
			o.Removed = nonNil(it.Values)
		} else {
			o.Added = nonNil(it.Values)
		}
		out = append(out, o)
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
	// plural names the values parameter, as in "genres".
	plural      string
	errorPrefix string
	// removes is set for the remove tools, whose books list the values they
	// lost under removed; the add tools list the values gained under added.
	removes bool
	// maxLength limits each value in the schema; 0, for the remove tools,
	// accepts any stored value.
	maxLength   int
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
					w.plural:  valuesSchema(w.plural, w.maxLength),
				},
				Required: []string{"itemIds", w.plural},
			},
		},
		Handler: func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, error) {
			itemIDs, err := requiredStrings(args, "itemIds")
			if err != nil {
				return errResult(err.Error())
			}
			values, err := requiredStrings(args, w.plural)
			if err != nil {
				return errResult(err.Error())
			}

			res, err := w.change(svc, ctx, itemIDs, values)
			if err != nil {
				return writeFailure(w.errorPrefix, err, toListPrevious(res.Items))
			}

			return marshalledResult(listChangeResultOutput{Changed: res.Changed, Items: toListChanges(res.Items, w.removes)})
		},
	}
}

func addGenres(svc MetadataService) ServerTool {
	return listWrite{
		name:        "add_genres",
		description: `Add genres to one or more books (up to 100). Genres are plain text, at most 64 characters each; markup (< or >), control and invisible characters are refused. Genres a book already has are skipped; a genre it has in another case is an error. Show the user the books and the genres and get their confirmation before calling. To rename a genre, call add_genres with the new name first, then remove_genres with the old one, so no book is left without it; only for a change of case (fiction → Fiction), remove first, because adding a case variant is an error. Each changed book lists the genres it gained in added; to undo, call remove_genres with exactly those genres on that book (books with the same added genres can share one call).`,
		plural:      "genres",
		errorPrefix: "failed to add genres: ",
		maxLength:   service.MaxValueLength,
		annotations: additiveWriteAnnotations(),
		change:      MetadataService.AddGenres,
	}.tool(svc)
}

func removeGenres(svc MetadataService) ServerTool {
	return listWrite{
		name:        "remove_genres",
		description: `Remove genres from one or more books (up to 100). Matching is exact, ignoring surrounding spaces; books without the genre are left unchanged. Show the user the books and the genres and get their confirmation before calling. For a rename, add the new name with add_genres first. Any genre can be removed, including ones add_genres would refuse. Each changed book lists the genres it lost in removed; to undo, call add_genres with exactly those genres on that book (books with the same removed genres can share one call); a genre add_genres refuses can't be added back.`,
		plural:      "genres",
		errorPrefix: "failed to remove genres: ",
		removes:     true,
		annotations: destructiveWriteAnnotations(),
		change:      MetadataService.RemoveGenres,
	}.tool(svc)
}

func addTags(svc MetadataService) ServerTool {
	return listWrite{
		name:        "add_tags",
		description: `Add tags to one or more books (up to 100). Tags are plain text, at most 64 characters each; markup (< or >), control and invisible characters are refused. Tags a book already has are skipped; a tag it has in another case is an error. Show the user the books and the tags and get their confirmation before calling. To rename a tag, call add_tags with the new name first, then remove_tags with the old one, so no book is left without it; only for a change of case (fiction → Fiction), remove first, because adding a case variant is an error. Each changed book lists the tags it gained in added; to undo, call remove_tags with exactly those tags on that book (books with the same added tags can share one call).`,
		plural:      "tags",
		errorPrefix: "failed to add tags: ",
		maxLength:   service.MaxValueLength,
		annotations: additiveWriteAnnotations(),
		change:      MetadataService.AddTags,
	}.tool(svc)
}

func removeTags(svc MetadataService) ServerTool {
	return listWrite{
		name:        "remove_tags",
		description: `Remove tags from one or more books (up to 100). Matching is exact, ignoring surrounding spaces; books without the tag are left unchanged. Show the user the books and the tags and get their confirmation before calling. For a rename, add the new name with add_tags first. Any tag can be removed, including ones add_tags would refuse. Each changed book lists the tags it lost in removed; to undo, call add_tags with exactly those tags on that book (books with the same removed tags can share one call); a tag add_tags refuses can't be added back.`,
		plural:      "tags",
		errorPrefix: "failed to remove tags: ",
		removes:     true,
		annotations: destructiveWriteAnnotations(),
		change:      MetadataService.RemoveTags,
	}.tool(svc)
}

// seriesOutput is a book's place in one series; a book with no number leaves
// the sequence out, as update_items takes it.
type seriesOutput struct {
	Name     string `json:"name"`
	Sequence string `json:"sequence,omitempty"`
}

// itemFieldsOutput is a book's fields as update_items takes them. A field
// that was not part of the change is left out; one that was sent and is empty
// shows as "" or [].
type itemFieldsOutput struct {
	Language    *string        `json:"language,omitempty"`
	Description *string        `json:"description,omitempty"`
	Series      []seriesOutput `json:"series,omitzero"`
	Authors     []string       `json:"authors,omitzero"`
	Narrators   []string       `json:"narrators,omitzero"`
}

// toSeriesOutput maps a book's series. Nil stays nil, which omitzero leaves
// out; any other list, even an empty one, shows as a list.
func toSeriesOutput(list []service.SeriesEntry) []seriesOutput {
	if list == nil {
		return nil
	}
	out := make([]seriesOutput, 0, len(list))
	for _, e := range list {
		out = append(out, seriesOutput(e))
	}
	return out
}

// toFieldsOutput maps a book's fields. A list the service left nil was not
// part of the change; the others are never nil, so an empty one shows as [].
func toFieldsOutput(f service.ItemFields) itemFieldsOutput {
	out := itemFieldsOutput{Language: f.Language, Description: f.Description}
	if f.Series != nil {
		out.Series = toSeriesOutput(*f.Series)
	}
	if f.Authors != nil {
		out.Authors = nonNil(*f.Authors)
	}
	if f.Narrators != nil {
		out.Narrators = nonNil(*f.Narrators)
	}
	return out
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
			Previous: toFieldsOutput(it.Previous),
			Now:      toFieldsOutput(it.Now),
		})
	}
	return out
}

func toUpdatePrevious(items []service.ItemUpdateResult) []previousOutput[itemFieldsOutput] {
	out := make([]previousOutput[itemFieldsOutput], 0, len(items))
	for _, it := range items {
		out = append(out, previousOutput[itemFieldsOutput]{ItemID: it.ItemID, Title: it.Title, Previous: toFieldsOutput(it.Previous)})
	}
	return out
}

// parseItemChanges reads the items of update_items. The schema has checked
// their shape; a field left out stays nil, so it is not touched.
func parseItemChanges(raw []any) []service.ItemChange {
	out := make([]service.ItemChange, 0, len(raw))
	for _, entry := range raw {
		obj, _ := entry.(map[string]any)
		id, _ := obj["itemId"].(string)
		out = append(out, service.ItemChange{
			ItemID:      id,
			Language:    optionalString(obj, "language"),
			Description: optionalString(obj, "description"),
			Series:      optionalSeries(obj),
			Authors:     optionalList(obj, "authors"),
			Narrators:   optionalList(obj, "narrators"),
		})
	}
	return out
}

// optionalString returns the named field, or nil when it is left out. A field
// sent as "" is a pointer to "", which clears it.
func optionalString(obj map[string]any, name string) *string {
	s, ok := obj[name].(string)
	if !ok {
		return nil
	}
	return &s
}

// optionalList returns the named list of strings, or nil when it is left out.
// A list sent as [] is a pointer to an empty list, which removes the values.
func optionalList(obj map[string]any, name string) *[]string {
	raw, ok := obj[name].([]any)
	if !ok {
		return nil
	}
	return new(stringsOf(raw))
}

// optionalSeries returns the series list, or nil when it is left out. The
// schema has checked that each entry has a name; a sequence left out is "".
func optionalSeries(obj map[string]any) *[]service.SeriesEntry {
	raw, ok := obj["series"].([]any)
	if !ok {
		return nil
	}
	out := make([]service.SeriesEntry, 0, len(raw))
	for _, entry := range raw {
		e, _ := entry.(map[string]any)
		name, _ := e["name"].(string)
		sequence, _ := e["sequence"].(string)
		out = append(out, service.SeriesEntry{Name: name, Sequence: sequence})
	}
	return &out
}

// The most series, and the most authors or narrators, a book may be given.
const (
	maxSeriesPerBook = 10
	maxPeoplePerBook = 20
)

func updateItems(svc MetadataService) ServerTool {
	return ServerTool{
		Toolset: metaMetadata,
		Tool: mcp.Tool{
			Name:        "update_items",
			Description: `Set the language, description, series, authors and/or narrators of one or more books (up to 100, at most 10 descriptions per call). Fields left out are not touched; an empty string clears a language or description. series, authors and narrators replace the book's whole list, so send every entry the book should keep; an empty list removes them. Read the book's current lists with get_item and send them back with your change. series is a list of {name, sequence}, the sequence being the book's number as free text such as "1", "1.5" or "0", at most 16 characters, and may be left out; authors and narrators are lists of names. Names match regardless of case. Audiobookshelf creates a series or author that doesn't exist yet and deletes a series left with no books. Use the exact name of an existing series or author (search_library finds them); a misspelt name creates a new one. Audiobookshelf also deletes an author left with no books unless it has a photo, ASIN or description. Descriptions may use simple HTML: p, br, b, i, em, strong, del, ul, ol, li, and a with an http, https or mailto link; other markup, scripts, comments and invisible characters are removed before saving, and now shows what was stored. A description is at most 10,000 characters; a language, series name, author or narrator is plain text, at most 64 characters, without markup, control or invisible characters. A value equal to what the book already has passes as it is, in a list too. Show the user every change, per book from old to new, and get their confirmation before calling. For genres and tags use add_genres, remove_genres, add_tags and remove_tags. Returns each book's previous values; to undo, call again with them, except that a description comes back cleaned, a value these checks refuse can't be restored, and a series Audiobookshelf deleted comes back as a new one. Example items: [{"itemId":"li_1","language":"Russian"},{"itemId":"li_2","description":"<p>A blurb.</p>"},{"itemId":"li_3","series":[{"name":"Holly Gibney","sequence":"3"}],"authors":["Stephen King"],"narrators":["Will Patton"]}]`,
			Annotations: destructiveWriteAnnotations(),
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"items": {
						Type:        "array",
						Description: "The books to change, at most 100",
						MinItems:    new(1),
						MaxItems:    new(service.MaxItemsPerCall),
						Items: &jsonschema.Schema{
							Type: "object",
							Properties: map[string]*jsonschema.Schema{
								"itemId":      {Type: "string", Description: "Library item ID of the book (from find_items, search_library or get_series)"},
								"language":    {Type: "string", Description: "New language, plain text, at most 64 characters; an empty string clears it"},
								"description": {Type: "string", Description: "New description, simple HTML, at most 10,000 characters; an empty string clears it"},
								"series": {
									Type:        "array",
									Description: "The book's series, replacing the ones it has, at most 10; [] removes the book from all of them",
									MaxItems:    new(maxSeriesPerBook),
									Items: &jsonschema.Schema{
										Type: "object",
										Properties: map[string]*jsonschema.Schema{
											"name":     {Type: "string", Description: "Series name, plain text, at most 64 characters; matched regardless of case, created if it doesn't exist yet"},
											"sequence": {Type: "string", Description: `The book's number in the series, such as "1", "1.5" or "0", plain text, at most 16 characters; leave it out for none`},
										},
										Required: []string{"name"},
									},
								},
								"authors": {
									Type:        "array",
									Description: "The book's authors, replacing the ones it has, at most 20; each plain text, at most 64 characters, matched regardless of case, created if it doesn't exist yet; [] removes them all",
									MaxItems:    new(maxPeoplePerBook),
									Items:       &jsonschema.Schema{Type: "string"},
								},
								"narrators": {
									Type:        "array",
									Description: "The book's narrators, replacing the ones it has, at most 20; each plain text, at most 64 characters; [] removes them all",
									MaxItems:    new(maxPeoplePerBook),
									Items:       &jsonschema.Schema{Type: "string"},
								},
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
			changes := parseItemChanges(raw)

			res, err := svc.Update(ctx, changes)
			if err != nil {
				return writeFailure("failed to update items: ", err, toUpdatePrevious(res.Items))
			}

			return marshalledResult(updateResultOutput{Changed: res.Changed, Items: toUpdateItems(res.Items)})
		},
	}
}
