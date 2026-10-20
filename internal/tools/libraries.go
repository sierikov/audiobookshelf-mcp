package tools

import (
	"context"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sierikov/audiobookshelf-mcp/internal/service"
)

// LibraryTools returns the tools in the libraries toolset.
func LibraryTools(svc LibraryService) []ServerTool {
	return []ServerTool{
		listLibraries(svc),
		searchLibrary(svc),
		listLibraryItems(svc),
	}
}

type librarySummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	MediaType string `json:"mediaType"`
}

func listLibraries(svc LibraryService) ServerTool {
	return ServerTool{
		Toolset: metaLibraries,
		Tool: mcp.Tool{
			Name:        "list_libraries",
			Description: "List all audiobook and podcast libraries",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: map[string]*jsonschema.Schema{},
			},
		},
		Handler: func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, error) {
			libs, err := svc.List(ctx)
			if err != nil {
				return errResult("failed to list libraries: " + err.Error())
			}

			out := make([]librarySummary, len(libs))
			for i, lib := range libs {
				out[i] = librarySummary{ID: lib.ID, Name: lib.Name, MediaType: lib.MediaType}
			}
			return marshalledResult(out)
		},
	}
}

type searchResultItem struct {
	Type     string `json:"type"`
	ID       string `json:"id"`
	Title    string `json:"title"`
	Author   string `json:"author,omitempty"`
	Duration string `json:"duration,omitempty"`
}

func searchLibrary(svc LibraryService) ServerTool {
	return ServerTool{
		Toolset: metaLibraries,
		Tool: mcp.Tool{
			Name:        "search_library",
			Description: "Search a library for audiobooks or podcasts by title, author, or narrator",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"libraryId": {
						Type:        "string",
						Description: "Library ID (from list_libraries)",
					},
					"query": {
						Type:        "string",
						Description: "Search term (title, author, narrator)",
					},
					"limit": {
						Type:        "number",
						Description: "Max results (default 10)",
					},
				},
				Required: []string{"libraryId", "query"},
			},
		},
		Handler: func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, error) {
			libraryID, err := RequiredParam[string](args, "libraryId")
			if err != nil {
				return errResult(err.Error())
			}
			query, err := RequiredParam[string](args, "query")
			if err != nil {
				return errResult(err.Error())
			}
			limit, err := OptionalIntParam(args, "limit", 10)
			if err != nil {
				return errResult(err.Error())
			}

			hits, err := svc.Search(ctx, libraryID, query, limit)
			if err != nil {
				return errResult("search failed: " + err.Error())
			}
			if len(hits) == 0 {
				return textResult(fmt.Sprintf("No results found for %q", query))
			}

			results := make([]searchResultItem, len(hits))
			for i, h := range hits {
				results[i] = searchResultItem{
					Type:     h.Type,
					ID:       h.ID,
					Title:    h.Title,
					Author:   h.Author,
					Duration: itemDuration(h.Item),
				}
			}
			return marshalledResult(results)
		},
	}
}

type libraryItemSummary struct {
	ID        string `json:"id"`
	MediaType string `json:"mediaType"`
	Title     string `json:"title"`
	Author    string `json:"author,omitempty"`
	Duration  string `json:"duration,omitempty"`
}

func toLibraryItemSummary(it service.Item) libraryItemSummary {
	return libraryItemSummary{
		ID:        it.ID,
		MediaType: it.MediaType,
		Title:     it.Title,
		Author:    it.Author,
		Duration:  itemDuration(it),
	}
}

func listLibraryItems(svc LibraryService) ServerTool {
	return ServerTool{
		Toolset: metaLibraries,
		Tool: mcp.Tool{
			Name:        "list_library_items",
			Description: "List items in a library with pagination",
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
						Description: "Max items per page (default 20)",
					},
					"page": {
						Type:        "number",
						Description: "Page number starting from 0 (default 0)",
					},
					"sort": {
						Type:        "string",
						Description: "Sort field (e.g. media.metadata.title)",
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
			page, err := pageParams(args)
			if err != nil {
				return errResult(err.Error())
			}
			sort, _, _ := OptionalParam[string](args, "sort")

			res, err := svc.Items(ctx, libraryID, page, sort)
			if err != nil {
				return errResult("failed to list items: " + err.Error())
			}

			items := make([]libraryItemSummary, 0, len(res.Items))
			for _, it := range res.Items {
				items = append(items, toLibraryItemSummary(it))
			}
			out := struct {
				Items []libraryItemSummary `json:"items"`
				Total int                  `json:"total"`
				Page  int                  `json:"page"`
			}{Items: items, Total: res.Total, Page: res.Page}
			return marshalledResult(out)
		},
	}
}

// pageParams reads the optional "limit" (default 20) and "page" (default 0)
// arguments shared by the paginated list tools.
func pageParams(args map[string]any) (service.Page, error) {
	limit, err := OptionalIntParam(args, "limit", 20)
	if err != nil {
		return service.Page{}, err
	}
	page, err := OptionalIntParam(args, "page", 0)
	if err != nil {
		return service.Page{}, err
	}
	return service.Page{Limit: limit, Page: page}, nil
}
