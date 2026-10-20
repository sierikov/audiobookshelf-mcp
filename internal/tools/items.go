package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sierikov/audiobookshelf-mcp/internal/service"
)

// ItemTools returns the tools in the items toolset.
func ItemTools(svc ItemService) []ServerTool {
	return []ServerTool{
		getItem(svc),
		getItemsInProgress(svc),
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

func getItem(svc ItemService) ServerTool {
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

			d, err := svc.Get(ctx, itemID)
			var mediaErr *service.MediaError
			switch {
			case errors.As(err, &mediaErr):
				return errResult(fmt.Sprintf("failed to parse %s media: %v", mediaErr.MediaType, mediaErr.Err))
			case err != nil:
				return errResult("failed to get item: " + err.Error())
			}

			return marshalledResult(itemDetail{
				ID:            d.ID,
				MediaType:     d.MediaType,
				Title:         d.Title,
				Subtitle:      d.Subtitle,
				Author:        d.Author,
				Narrator:      d.Narrator,
				SeriesName:    d.SeriesName,
				Description:   d.Description,
				Duration:      itemDuration(d.Item),
				PublishedYear: d.PublishedYear,
				Publisher:     d.Publisher,
				Genres:        d.Genres,
				Language:      d.Language,
				NumEpisodes:   d.NumEpisodes,
			})
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

func toInProgressItem(it service.InProgressItem) inProgressItem {
	out := inProgressItem{
		ID:       it.ID,
		Title:    it.Title,
		Author:   it.Author,
		Duration: itemDuration(it.Item),
	}
	if p := it.Progress; p != nil {
		out.Progress = FormatProgress(p.Fraction)
		if p.Remaining > 0 {
			out.TimeRemaining = FormatDuration(p.Remaining)
		}
	}
	return out
}

func getItemsInProgress(svc ItemService) ServerTool {
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

			inProgress, err := svc.InProgress(ctx, limit)
			if err != nil {
				return errResult("failed to get items in progress: " + err.Error())
			}

			items := make([]inProgressItem, 0, len(inProgress))
			for _, it := range inProgress {
				items = append(items, toInProgressItem(it))
			}
			return marshalledResult(items)
		},
	}
}
