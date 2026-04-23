package audiobookshelf

import (
	"context"
	"net/url"
	"strconv"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// BrowseTools returns the tools in the browse toolset.
func BrowseTools(client *ABSClient) []ServerTool {
	return []ServerTool{
		listSeries(client),
		getSeries(client),
		getAuthor(client),
		listCollections(client),
	}
}

type seriesSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	NumBooks int    `json:"numBooks"`
}

func listSeries(client *ABSClient) ServerTool {
	return ServerTool{
		Toolset: MetaBrowse,
		Tool: mcp.Tool{
			Name:        "list_series",
			Description: "List all series in a library",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"libraryId": {
						Type:        "string",
						Description: "Library ID (from list_libraries)",
					},
					"limit": {
						Type:        "number",
						Description: "Max results per page (default 20)",
					},
					"page": {
						Type:        "number",
						Description: "Page number starting from 0 (default 0)",
					},
				},
				Required: []string{"libraryId"},
			},
		},
		Handler: func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, error) {
			libraryID, err := RequiredParam[string](args, "libraryId")
			if err != nil {
				return errResult(err.Error())
			}
			limit, err := OptionalIntParam(args, "limit", 20)
			if err != nil {
				return errResult(err.Error())
			}
			page, err := OptionalIntParam(args, "page", 0)
			if err != nil {
				return errResult(err.Error())
			}

			params := url.Values{
				"limit": {strconv.Itoa(limit)},
				"page":  {strconv.Itoa(page)},
			}

			var resp struct {
				Results []struct {
					ID    string        `json:"id"`
					Name  string        `json:"name"`
					Books []LibraryItem `json:"books"`
				} `json:"results"`
				Total int `json:"total"`
			}
			if err := client.getJSON(ctx, "/api/libraries/"+sanitizePathParam(libraryID)+"/series", params, &resp); err != nil {
				return errResult("failed to list series: " + err.Error())
			}

			series := make([]seriesSummary, 0, len(resp.Results))
			for _, s := range resp.Results {
				series = append(series, seriesSummary{
					ID: s.ID, Name: s.Name, NumBooks: len(s.Books),
				})
			}

			out := struct {
				Series []seriesSummary `json:"series"`
				Total  int             `json:"total"`
			}{Series: series, Total: resp.Total}
			return marshalledResult(out)
		},
	}
}

type seriesDetail struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	Books       []libraryItemSummary `json:"books"`
}

func getSeries(client *ABSClient) ServerTool {
	return ServerTool{
		Toolset: MetaBrowse,
		Tool: mcp.Tool{
			Name:        "get_series",
			Description: "Get details of a series including its books",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"seriesId": {
						Type:        "string",
						Description: "Series ID",
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

			var resp struct {
				ID          string        `json:"id"`
				Name        string        `json:"name"`
				Description string        `json:"description"`
				Books       []LibraryItem `json:"books"`
			}
			if err := client.getJSON(ctx, "/api/series/"+sanitizePathParam(seriesID), nil, &resp); err != nil {
				return errResult("failed to get series: " + err.Error())
			}

			detail := seriesDetail{
				ID:          resp.ID,
				Name:        resp.Name,
				Description: resp.Description,
			}
			for _, item := range resp.Books {
				book, err := item.AsBook()
				if err != nil {
					continue // skip unparseable items
				}
				detail.Books = append(detail.Books, libraryItemSummary{
					ID:        item.ID,
					MediaType: item.MediaType,
					Title:     book.Metadata.Title,
					Author:    book.Metadata.AuthorName,
					Duration:  FormatDuration(book.Duration),
				})
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

func getAuthor(client *ABSClient) ServerTool {
	return ServerTool{
		Toolset: MetaBrowse,
		Tool: mcp.Tool{
			Name:        "get_author",
			Description: "Get details about an author including their books",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
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

			params := url.Values{"expand": {"1"}}
			var author Author
			if err := client.getJSON(ctx, "/api/authors/"+sanitizePathParam(authorID), params, &author); err != nil {
				return errResult("failed to get author: " + err.Error())
			}

			detail := authorDetail{
				ID:          author.ID,
				Name:        author.Name,
				Description: author.Description,
				NumBooks:    author.NumBooks,
			}
			for _, item := range author.LibraryItems {
				book, err := item.AsBook()
				if err != nil {
					continue // skip unparseable items
				}
				detail.Books = append(detail.Books, libraryItemSummary{
					ID:        item.ID,
					MediaType: item.MediaType,
					Title:     book.Metadata.Title,
					Author:    book.Metadata.AuthorName,
					Duration:  FormatDuration(book.Duration),
				})
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

func listCollections(client *ABSClient) ServerTool {
	return ServerTool{
		Toolset: MetaBrowse,
		Tool: mcp.Tool{
			Name:        "list_collections",
			Description: "List collections in a library",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"libraryId": {
						Type:        "string",
						Description: "Library ID (from list_libraries)",
					},
					"limit": {
						Type:        "number",
						Description: "Max results per page (default 20)",
					},
					"page": {
						Type:        "number",
						Description: "Page number starting from 0 (default 0)",
					},
				},
				Required: []string{"libraryId"},
			},
		},
		Handler: func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, error) {
			libraryID, err := RequiredParam[string](args, "libraryId")
			if err != nil {
				return errResult(err.Error())
			}
			limit, err := OptionalIntParam(args, "limit", 20)
			if err != nil {
				return errResult(err.Error())
			}
			page, err := OptionalIntParam(args, "page", 0)
			if err != nil {
				return errResult(err.Error())
			}

			params := url.Values{
				"limit": {strconv.Itoa(limit)},
				"page":  {strconv.Itoa(page)},
			}

			var resp struct {
				Results []struct {
					ID          string        `json:"id"`
					Name        string        `json:"name"`
					Description string        `json:"description"`
					Books       []LibraryItem `json:"books"`
				} `json:"results"`
				Total int `json:"total"`
			}
			if err := client.getJSON(ctx, "/api/libraries/"+sanitizePathParam(libraryID)+"/collections", params, &resp); err != nil {
				return errResult("failed to list collections: " + err.Error())
			}

			collections := make([]collectionSummary, 0, len(resp.Results))
			for _, c := range resp.Results {
				collections = append(collections, collectionSummary{
					ID: c.ID, Name: c.Name, Description: c.Description, NumBooks: len(c.Books),
				})
			}

			out := struct {
				Collections []collectionSummary `json:"collections"`
				Total       int                 `json:"total"`
			}{Collections: collections, Total: resp.Total}
			return marshalledResult(out)
		},
	}
}
