package audiobookshelf

import (
	"context"
	"net/url"
	"strconv"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ItemTools returns the tools in the items toolset.
func ItemTools(client *ABSClient) []ServerTool {
	return []ServerTool{
		getItem(client),
		getItemsInProgress(client),
	}
}

type itemDetail struct {
	ID            string   `json:"id"`
	MediaType     string   `json:"mediaType"`
	Title         string   `json:"title"`
	Subtitle      string   `json:"subtitle,omitempty"`
	Author        string   `json:"author,omitempty"`
	Narrator      string   `json:"narrator,omitempty"`
	SeriesName    string   `json:"seriesName,omitempty"`
	Description   string   `json:"description,omitempty"`
	Duration      string   `json:"duration,omitempty"`
	PublishedYear string   `json:"publishedYear,omitempty"`
	Publisher     string   `json:"publisher,omitempty"`
	Genres        []string `json:"genres,omitempty"`
	Language      string   `json:"language,omitempty"`
	NumEpisodes   int      `json:"numEpisodes,omitempty"`
}

func getItem(client *ABSClient) ServerTool {
	return ServerTool{
		Toolset: metaItems,
		Tool: mcp.Tool{
			Name:        "get_item",
			Description: "Get detailed information about an audiobook or podcast by its item ID",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"itemId": {
						Type:        "string",
						Description: "Library item ID",
					},
				},
				Required: []string{"itemId"},
			},
		},
		Handler: func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, error) {
			itemID, err := RequiredParam[string](args, "itemId")
			if err != nil {
				return errResult(err.Error())
			}

			params := url.Values{"expanded": {"1"}}
			var item LibraryItem
			if err := client.getJSON(ctx, "/api/items/"+sanitizePathParam(itemID), params, &item); err != nil {
				return errResult("failed to get item: " + err.Error())
			}

			detail := itemDetail{ID: item.ID, MediaType: item.MediaType}
			switch item.MediaType {
			case "book":
				book, err := item.AsBook()
				if err != nil {
					return errResult("failed to parse book media: " + err.Error())
				}
				m := book.Metadata
				detail.Title = m.Title
				detail.Subtitle = m.Subtitle
				detail.Author = m.AuthorName
				detail.Narrator = m.NarratorName
				detail.SeriesName = m.SeriesName
				detail.Description = m.Description
				detail.Duration = FormatDuration(book.Duration)
				detail.PublishedYear = m.PublishedYear
				detail.Publisher = m.Publisher
				detail.Genres = m.Genres
				detail.Language = m.Language
			case "podcast":
				pod, err := item.AsPodcast()
				if err != nil {
					return errResult("failed to parse podcast media: " + err.Error())
				}
				m := pod.Metadata
				detail.Title = m.Title
				detail.Author = m.Author
				detail.Description = m.Description
				detail.Genres = m.Genres
				detail.Language = m.Language
				detail.NumEpisodes = pod.NumEpisodes
			}
			return marshalledResult(detail)
		},
	}
}

type inProgressItem struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Author        string `json:"author,omitempty"`
	Duration      string `json:"duration,omitempty"`
	Progress      string `json:"progress"`
	TimeRemaining string `json:"timeRemaining,omitempty"`
}

func getItemsInProgress(client *ABSClient) ServerTool {
	return ServerTool{
		Toolset: metaItems,
		Tool: mcp.Tool{
			Name:        "get_items_in_progress",
			Description: "Get audiobooks and podcasts currently being listened to",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"limit": {
						Type:        "number",
						Description: "Max items to return (default 25)",
					},
				},
			},
		},
		Handler: func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, error) {
			limit, err := OptionalIntParam(args, "limit", 25)
			if err != nil {
				return errResult(err.Error())
			}

			params := url.Values{"limit": {strconv.Itoa(limit)}}
			// The API returns LibraryItem objects directly in libraryItems[]
			var resp struct {
				LibraryItems []LibraryItem `json:"libraryItems"`
			}
			if err := client.getJSON(ctx, "/api/me/items-in-progress", params, &resp); err != nil {
				return errResult("failed to get items in progress: " + err.Error())
			}

			items := make([]inProgressItem, 0, len(resp.LibraryItems))
			for _, item := range resp.LibraryItems {
				ip := inProgressItem{ID: item.ID}

				// Fetch progress separately
				var prog MediaProgress
				if err := client.getJSON(ctx, "/api/me/progress/"+sanitizePathParam(item.ID), nil, &prog); err == nil {
					ip.Progress = FormatProgress(prog.Progress)
					remaining := prog.Duration - prog.CurrentTime
					if remaining > 0 {
						ip.TimeRemaining = FormatDuration(remaining)
					}
				}

				switch item.MediaType {
				case "book":
					book, err := item.AsBook()
					if err != nil {
						continue // skip unparseable items
					}
					ip.Title = book.Metadata.Title
					ip.Author = book.Metadata.AuthorName
					ip.Duration = FormatDuration(book.Duration)
				case "podcast":
					pod, err := item.AsPodcast()
					if err != nil {
						continue // skip unparseable items
					}
					ip.Title = pod.Metadata.Title
					ip.Author = pod.Metadata.Author
				}
				items = append(items, ip)
			}
			return marshalledResult(items)
		},
	}
}
