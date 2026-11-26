package tools

import (
	"context"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// BrowseTools returns the tools in the browse toolset.
func BrowseTools(svc BrowseService) []ServerTool {
	return []ServerTool{
		listSeries(svc),
		getSeries(svc),
		getAuthor(svc),
		listCollections(svc),
	}
}

type seriesSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	NumBooks int    `json:"numBooks"`
}

func listSeries(svc BrowseService) ServerTool {
	return ServerTool{
		Toolset: metaBrowse,
		Tool: mcp.Tool{
			Name:        "list_series",
			Description: "List all series in a library",
			Annotations: readAnnotations(),
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"libraryId": {
						Type:        "string",
						Description: "Library ID (from list_libraries)",
					},
					"limit": limitSchema("Max results per page (default 20)"),
					"page":  pageSchema(),
				},
				Required: []string{"libraryId"},
			},
		},
		Handler: func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, error) {
			libraryID, err := RequiredParam[string](args, "libraryId")
			if err != nil {
				return errResult(err.Error())
			}
			page := pageParams(args, 20)

			res, err := svc.ListSeries(ctx, libraryID, page)
			if err != nil {
				return errResult("failed to list series: " + err.Error())
			}

			series := make([]seriesSummary, 0, len(res.Series))
			for _, s := range res.Series {
				series = append(series, seriesSummary{ID: s.ID, Name: s.Name, NumBooks: s.NumBooks})
			}
			out := struct {
				Series []seriesSummary `json:"series"`
				Total  int             `json:"total"`
			}{Series: series, Total: res.Total}
			return marshalledResult(out)
		},
	}
}

// seriesBook is a book of a series; Sequence is left out for a book without a number.
type seriesBook struct {
	libraryItemSummary
	Sequence string `json:"sequence,omitempty"`
}

type seriesDetail struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Books       []seriesBook `json:"books"`
}

func getSeries(svc BrowseService) ServerTool {
	return ServerTool{
		Toolset: metaBrowse,
		Tool: mcp.Tool{
			Name:        "get_series",
			Description: "Get details of a series including its books, in reading order, each with its number in the series when it has one",
			Annotations: readAnnotations(),
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"seriesId": {
						Type:        "string",
						Description: "Series ID (from list_series or search_library)",
					},
				},
				Required: []string{"seriesId"},
			},
		},
		Handler: func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, error) {
			seriesID, err := RequiredParam[string](args, "seriesId")
			if err != nil {
				return errResult(err.Error())
			}

			s, err := svc.Series(ctx, seriesID)
			if err != nil {
				return errResult("failed to get series: " + err.Error())
			}

			detail := seriesDetail{ID: s.ID, Name: s.Name, Description: s.Description, Books: make([]seriesBook, 0, len(s.Books))}
			for _, b := range s.Books {
				detail.Books = append(detail.Books, seriesBook{libraryItemSummary: toLibraryItemSummary(b.Item), Sequence: b.Sequence})
			}
			return marshalledResult(detail)
		},
	}
}

type authorDetail struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	NumBooks    int                  `json:"numBooks"`
	Books       []libraryItemSummary `json:"books,omitempty"`
}

func getAuthor(svc BrowseService) ServerTool {
	return ServerTool{
		Toolset: metaBrowse,
		Tool: mcp.Tool{
			Name:        "get_author",
			Description: "Get details about an author including their books",
			Annotations: readAnnotations(),
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"authorId": {
						Type:        "string",
						Description: "Author ID",
					},
				},
				Required: []string{"authorId"},
			},
		},
		Handler: func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, error) {
			authorID, err := RequiredParam[string](args, "authorId")
			if err != nil {
				return errResult(err.Error())
			}

			a, err := svc.Author(ctx, authorID)
			if err != nil {
				return errResult("failed to get author: " + err.Error())
			}

			detail := authorDetail{ID: a.ID, Name: a.Name, Description: a.Description, NumBooks: a.NumBooks}
			for _, b := range a.Books {
				detail.Books = append(detail.Books, toLibraryItemSummary(b))
			}
			return marshalledResult(detail)
		},
	}
}

type collectionSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	NumBooks    int    `json:"numBooks"`
}

func listCollections(svc BrowseService) ServerTool {
	return ServerTool{
		Toolset: metaBrowse,
		Tool: mcp.Tool{
			Name:        "list_collections",
			Description: "List collections in a library",
			Annotations: readAnnotations(),
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"libraryId": {
						Type:        "string",
						Description: "Library ID (from list_libraries)",
					},
					"limit": limitSchema("Max results per page (default 20)"),
					"page":  pageSchema(),
				},
				Required: []string{"libraryId"},
			},
		},
		Handler: func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, error) {
			libraryID, err := RequiredParam[string](args, "libraryId")
			if err != nil {
				return errResult(err.Error())
			}
			page := pageParams(args, 20)

			res, err := svc.Collections(ctx, libraryID, page)
			if err != nil {
				return errResult("failed to list collections: " + err.Error())
			}

			collections := make([]collectionSummary, 0, len(res.Collections))
			for _, c := range res.Collections {
				collections = append(collections, collectionSummary{
					ID: c.ID, Name: c.Name, Description: c.Description, NumBooks: c.NumBooks,
				})
			}
			out := struct {
				Collections []collectionSummary `json:"collections"`
				Total       int                 `json:"total"`
			}{Collections: collections, Total: res.Total}
			return marshalledResult(out)
		},
	}
}
