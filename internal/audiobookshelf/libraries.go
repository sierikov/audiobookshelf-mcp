package audiobookshelf

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// LibraryTools returns the tools in the libraries toolset.
func LibraryTools(client *ABSClient) []ServerTool {
	return []ServerTool{
		listLibraries(client),
		searchLibrary(client),
		listLibraryItems(client),
	}
}

type librarySummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	MediaType string `json:"mediaType"`
}

func listLibraries(client *ABSClient) ServerTool {
	return ServerTool{
		Toolset: MetaLibraries,
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
			var resp struct {
				Libraries []Library `json:"libraries"`
			}
			if err := client.getJSON(ctx, "/api/libraries", nil, &resp); err != nil {
				return errResult("failed to list libraries: " + err.Error())
			}

			out := make([]librarySummary, len(resp.Libraries))
			for i, lib := range resp.Libraries {
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

func searchLibrary(client *ABSClient) ServerTool {
	return ServerTool{
		Toolset: MetaLibraries,
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

			params := url.Values{
				"q":     {query},
				"limit": {strconv.Itoa(limit)},
			}
			var resp SearchResults
			if err := client.getJSON(ctx, "/api/libraries/"+sanitizePathParam(libraryID)+"/search", params, &resp); err != nil {
				return errResult("search failed: " + err.Error())
			}

			var results []searchResultItem
			for _, hit := range resp.Book {
				if r, err := flattenBookResult(hit.LibraryItem); err == nil {
					results = append(results, r)
				}
			}
			for _, hit := range resp.Podcast {
				if r, err := flattenPodcastResult(hit.LibraryItem); err == nil {
					results = append(results, r)
				}
			}
			for _, a := range resp.Authors {
				results = append(results, searchResultItem{Type: "author", ID: a.ID, Title: a.Name})
			}
			for _, s := range resp.Series {
				results = append(results, searchResultItem{Type: "series", ID: s.Series.ID, Title: s.Series.Name})
			}

			if len(results) == 0 {
				return textResult(fmt.Sprintf("No results found for %q", query))
			}
			return marshalledResult(results)
		},
	}
}

func flattenBookResult(item LibraryItem) (searchResultItem, error) {
	book, err := item.AsBook()
	if err != nil {
		return searchResultItem{}, fmt.Errorf("item %s: %w", item.ID, err)
	}
	return searchResultItem{
		Type:     "book",
		ID:       item.ID,
		Title:    book.Metadata.Title,
		Author:   book.Metadata.AuthorName,
		Duration: FormatDuration(book.Duration),
	}, nil
}

func flattenPodcastResult(item LibraryItem) (searchResultItem, error) {
	pod, err := item.AsPodcast()
	if err != nil {
		return searchResultItem{}, fmt.Errorf("item %s: %w", item.ID, err)
	}
	return searchResultItem{
		Type:   "podcast",
		ID:     item.ID,
		Title:  pod.Metadata.Title,
		Author: pod.Metadata.Author,
	}, nil
}

type libraryItemSummary struct {
	ID        string `json:"id"`
	MediaType string `json:"mediaType"`
	Title     string `json:"title"`
	Author    string `json:"author,omitempty"`
	Duration  string `json:"duration,omitempty"`
}

func listLibraryItems(client *ABSClient) ServerTool {
	return ServerTool{
		Toolset: MetaLibraries,
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
			limit, err := OptionalIntParam(args, "limit", 20)
			if err != nil {
				return errResult(err.Error())
			}
			page, err := OptionalIntParam(args, "page", 0)
			if err != nil {
				return errResult(err.Error())
			}
			sort, _, _ := OptionalParam[string](args, "sort")

			params := url.Values{
				"limit": {strconv.Itoa(limit)},
				"page":  {strconv.Itoa(page)},
			}
			if sort != "" {
				params.Set("sort", sort)
			}

			var resp struct {
				Results []LibraryItem `json:"results"`
				Total   int           `json:"total"`
				Page    int           `json:"page"`
			}
			if err := client.getJSON(ctx, "/api/libraries/"+sanitizePathParam(libraryID)+"/items", params, &resp); err != nil {
				return errResult("failed to list items: " + err.Error())
			}

			items := make([]libraryItemSummary, 0, len(resp.Results))
			for _, item := range resp.Results {
				s := libraryItemSummary{ID: item.ID, MediaType: item.MediaType}
				switch item.MediaType {
				case "book":
					book, err := item.AsBook()
					if err != nil {
						continue // skip unparseable items
					}
					s.Title = book.Metadata.Title
					s.Author = book.Metadata.AuthorName
					s.Duration = FormatDuration(book.Duration)
				case "podcast":
					pod, err := item.AsPodcast()
					if err != nil {
						continue // skip unparseable items
					}
					s.Title = pod.Metadata.Title
					s.Author = pod.Metadata.Author
				}
				items = append(items, s)
			}

			out := struct {
				Items []libraryItemSummary `json:"items"`
				Total int                  `json:"total"`
				Page  int                  `json:"page"`
			}{Items: items, Total: resp.Total, Page: resp.Page}
			return marshalledResult(out)
		},
	}
}
