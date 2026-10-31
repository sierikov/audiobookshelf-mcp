package tools

import (
	"context"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sierikov/audiobookshelf-mcp/internal/service"
)

// MetadataTools returns the tools in the metadata toolset.
func MetadataTools(svc MetadataService) []ServerTool {
	return []ServerTool{
		getMetadataOverview(svc),
		findItems(svc),
		searchBookMetadata(svc),
		addGenres(svc),
		removeGenres(svc),
		addTags(svc),
		removeTags(svc),
		updateItems(svc),
	}
}

// descriptionPreviewRunes is how much of a description find_items shows.
const descriptionPreviewRunes = 200

// defaultFindLimit is how many books find_items returns per page when the
// caller gives no limit.
const defaultFindLimit = 50

// readAnnotations marks a tool that only reads from the Audiobookshelf server.
func readAnnotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)}
}

// openWorldReadAnnotations marks a tool that only reads, but asks a service
// outside the Audiobookshelf server.
func openWorldReadAnnotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(true)}
}

// additiveWriteAnnotations marks a tool that only adds to what is stored, and
// is idempotent because adding a value twice leaves the same list.
func additiveWriteAnnotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(false)}
}

// destructiveWriteAnnotations marks a tool that removes or replaces stored
// values, and is idempotent because repeating a call leaves the same result.
func destructiveWriteAnnotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{DestructiveHint: new(true), IdempotentHint: true, OpenWorldHint: new(false)}
}

func libraryIDSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "string", Description: "Library ID (from list_libraries)"}
}

// itemIDsSchema describes the list of book ids the write tools take.
func itemIDsSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:        "array",
		Description: "Library item IDs of the books (from find_items), at most 100",
		Items:       &jsonschema.Schema{Type: "string"},
	}
}

// valuesSchema describes a list of genres or tags, named by noun.
func valuesSchema(noun string) *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:        "array",
		Description: "The " + noun + ", matched exactly (case and spelling count)",
		Items:       &jsonschema.Schema{Type: "string"},
	}
}

// nonNil returns s, or an empty list for nil, so JSON shows [] and not null.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// preview returns the first n runes of s, and the number of runes in s.
// It cuts between runes, so the result is always valid UTF-8.
func preview(s string, n int) (head string, runes int) {
	r := []rune(s)
	if len(r) <= n {
		return s, len(r)
	}
	return string(r[:n]), len(r)
}

// optionalStrings reads the named optional string parameters, in order.
// A missing parameter is "".
func optionalStrings(args map[string]any, names ...string) ([]string, error) {
	out := make([]string, 0, len(names))
	for _, name := range names {
		v, _, err := OptionalParam[string](args, name)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

type valueCountOutput struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func toValueCounts(vs []service.ValueCount) []valueCountOutput {
	out := make([]valueCountOutput, 0, len(vs))
	for _, v := range vs {
		out = append(out, valueCountOutput{Name: v.Name, Count: v.Count})
	}
	return out
}

type missingCountsOutput struct {
	Genres      int `json:"genres"`
	Tags        int `json:"tags"`
	Language    int `json:"language"`
	Description int `json:"description"`
}

type metadataOverviewOutput struct {
	LibraryID string              `json:"libraryId"`
	Books     int                 `json:"books"`
	Genres    []valueCountOutput  `json:"genres"`
	Tags      []valueCountOutput  `json:"tags"`
	Languages []valueCountOutput  `json:"languages"`
	Missing   missingCountsOutput `json:"missing"`
}

func getMetadataOverview(svc MetadataService) ServerTool {
	return ServerTool{
		Toolset: metaMetadata,
		Tool: mcp.Tool{
			Name:        "get_metadata_overview",
			Description: `Count every genre, tag and language used in a library, and how many books have no genres, tags, language or description. Matching is exact, so "English" and "english" are listed separately. Counts come from the books themselves, so they are current right after an edit. Use find_items to see the books behind a count.`,
			Annotations: readAnnotations(),
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: map[string]*jsonschema.Schema{"libraryId": libraryIDSchema()},
				Required:   []string{"libraryId"},
			},
		},
		Handler: func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, error) {
			libraryID, err := RequiredParam[string](args, "libraryId")
			if err != nil {
				return errResult(err.Error())
			}

			o, err := svc.Overview(ctx, libraryID)
			if err != nil {
				return errResult("failed to get metadata overview: " + err.Error())
			}

			return marshalledResult(metadataOverviewOutput{
				LibraryID: o.LibraryID,
				Books:     o.Books,
				Genres:    toValueCounts(o.Genres),
				Tags:      toValueCounts(o.Tags),
				Languages: toValueCounts(o.Languages),
				Missing:   missingCountsOutput(o.Missing),
			})
		},
	}
}

type metadataItemOutput struct {
	ItemID            string   `json:"itemId"`
	Title             string   `json:"title"`
	Author            string   `json:"author"`
	Series            string   `json:"series,omitempty"`
	Language          string   `json:"language,omitempty"`
	Genres            []string `json:"genres"`
	Tags              []string `json:"tags"`
	Description       string   `json:"description"`
	DescriptionLength int      `json:"descriptionLength"`
}

type metadataPageOutput struct {
	Items []metadataItemOutput `json:"items"`
	Total int                  `json:"total"`
	Page  int                  `json:"page"`
}

func toMetadataItems(items []service.MetadataItem) []metadataItemOutput {
	out := make([]metadataItemOutput, 0, len(items))
	for _, it := range items {
		head, length := preview(it.Description, descriptionPreviewRunes)
		out = append(out, metadataItemOutput{
			ItemID:            it.ID,
			Title:             it.Title,
			Author:            it.Author,
			Series:            it.Series,
			Language:          it.Language,
			Genres:            nonNil(it.Genres),
			Tags:              nonNil(it.Tags),
			Description:       head,
			DescriptionLength: length,
		})
	}
	return out
}

// findArgs are the arguments of find_items, read from the call.
type findArgs struct {
	libraryID string
	filter    service.ItemFilter
	page      service.Page
}

func parseFindArgs(args map[string]any) (findArgs, error) {
	libraryID, err := RequiredParam[string](args, "libraryId")
	if err != nil {
		return findArgs{}, err
	}
	strs, err := optionalStrings(args, "genre", "tag", "language", "missing")
	if err != nil {
		return findArgs{}, err
	}
	limit, err := OptionalIntParam(args, "limit", defaultFindLimit)
	if err != nil {
		return findArgs{}, err
	}
	page, err := OptionalIntParam(args, "page", 0)
	if err != nil {
		return findArgs{}, err
	}
	return findArgs{
		libraryID: libraryID,
		filter:    service.ItemFilter{Genre: strs[0], Tag: strs[1], Language: strs[2], Missing: service.MissingField(strs[3])},
		page:      service.Page{Limit: limit, Page: page},
	}, nil
}

func findItems(svc MetadataService) ServerTool {
	missing := make([]any, 0, len(service.MissingFields()))
	for _, f := range service.MissingFields() {
		missing = append(missing, string(f))
	}
	return ServerTool{
		Toolset: metaMetadata,
		Tool: mcp.Tool{
			Name:        "find_items",
			Description: "List a library's books with their genres, tags, language and the first 200 characters of their plain-text description. Optional filters combine: genre, tag and language match exactly, as get_metadata_overview counts them; missing picks books that lack genres, tags, a language or a description. Sorted by title. Pass the itemIds to the write tools.",
			Annotations: readAnnotations(),
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"libraryId": libraryIDSchema(),
					"genre":     {Type: "string", Description: "Only books with this genre"},
					"tag":       {Type: "string", Description: "Only books with this tag"},
					"language":  {Type: "string", Description: "Only books in this language"},
					"missing":   {Type: "string", Enum: missing, Description: "Only books that lack this field"},
					"limit":     {Type: "number", Description: "Max books per page (default 50, at most 100)"},
					"page":      {Type: "number", Description: "Page number, starting at 0 (default 0)"},
				},
				Required: []string{"libraryId"},
			},
		},
		Handler: func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, error) {
			a, err := parseFindArgs(args)
			if err != nil {
				return errResult(err.Error())
			}

			p, err := svc.Find(ctx, a.libraryID, a.filter, a.page)
			if err != nil {
				return errResult("failed to find items: " + err.Error())
			}

			return marshalledResult(metadataPageOutput{
				Items: toMetadataItems(p.Items),
				Total: p.Total,
				Page:  p.Page,
			})
		},
	}
}

type metadataBookOutput struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Author string `json:"author"`
}

type metadataCandidateOutput struct {
	Title         string   `json:"title,omitempty"`
	Subtitle      string   `json:"subtitle,omitempty"`
	Author        string   `json:"author,omitempty"`
	Narrator      string   `json:"narrator,omitempty"`
	Publisher     string   `json:"publisher,omitempty"`
	PublishedYear string   `json:"publishedYear,omitempty"`
	Language      string   `json:"language,omitempty"`
	Description   string   `json:"description,omitempty"`
	Genres        []string `json:"genres"`
	Tags          []string `json:"tags"`
}

type metadataSearchOutput struct {
	Book       metadataBookOutput        `json:"book"`
	Provider   string                    `json:"provider"`
	Region     string                    `json:"region,omitempty"`
	Candidates []metadataCandidateOutput `json:"candidates"`
}

func toMetadataCandidates(cs []service.MetadataCandidate) []metadataCandidateOutput {
	out := make([]metadataCandidateOutput, 0, len(cs))
	for _, c := range cs {
		out = append(out, metadataCandidateOutput{
			Title:         c.Title,
			Subtitle:      c.Subtitle,
			Author:        c.Author,
			Narrator:      c.Narrator,
			Publisher:     c.Publisher,
			PublishedYear: c.PublishedYear,
			Language:      c.Language,
			Description:   c.Description,
			Genres:        nonNil(c.Genres),
			Tags:          nonNil(c.Tags),
		})
	}
	return out
}

func searchBookMetadata(svc MetadataService) ServerTool {
	providers := make([]any, 0, len(service.MetadataProviders()))
	for _, p := range service.MetadataProviders() {
		providers = append(providers, p)
	}
	region := regionSchema()
	region.Description = "Audible store region, for the audible provider only (default us)"
	return ServerTool{
		Toolset: metaMetadata,
		Tool: mcp.Tool{
			Name:        "search_book_metadata",
			Description: "Look a book up in one metadata source and return candidates with their title, author, genres, tags and description (HTML). Use fantlab for Russian-language fiction, audible for English, and google as a fallback. Provider genres are hints to map onto the user's own genre list, not values to copy. If no source has a description and you write one yourself, tell the user. To find an edition's chapters by runtime, use search_audible instead.",
			Annotations: openWorldReadAnnotations(),
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"itemId":   itemIDSchema(),
					"provider": {Type: "string", Enum: providers, Description: "Metadata source to search"},
					"region":   region,
					"title":    {Type: "string", Description: "Title to search for (default: the book's title)"},
					"author":   {Type: "string", Description: "Author to search for (default: the book's author)"},
				},
				Required: []string{"itemId", "provider"},
			},
		},
		Handler: func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, error) {
			itemID, err := RequiredParam[string](args, "itemId")
			if err != nil {
				return errResult(err.Error())
			}
			provider, err := RequiredParam[string](args, "provider")
			if err != nil {
				return errResult(err.Error())
			}
			strs, err := optionalStrings(args, "region", "title", "author")
			if err != nil {
				return errResult(err.Error())
			}

			res, err := svc.SearchBooks(ctx, itemID, provider, strs[0], strs[1], strs[2])
			if err != nil {
				return errResult("metadata search failed: " + err.Error())
			}

			return marshalledResult(metadataSearchOutput{
				Book:       metadataBookOutput{ID: res.Book.ID, Title: res.Book.Title, Author: res.Book.Author},
				Provider:   res.Provider,
				Region:     res.Region,
				Candidates: toMetadataCandidates(res.Candidates),
			})
		},
	}
}
