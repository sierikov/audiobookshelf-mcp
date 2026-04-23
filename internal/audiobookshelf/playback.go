package audiobookshelf

import (
	"context"
	"net/url"
	"strconv"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// PlaybackTools returns the tools in the playback toolset.
func PlaybackTools(client *ABSClient) []ServerTool {
	return []ServerTool{
		getListeningStats(client),
		getListeningSessions(client),
		getMediaProgress(client),
	}
}

type listeningStatsOutput struct {
	TotalTime      string            `json:"totalTime"`
	Today          string            `json:"today"`
	DayOfWeek      map[string]string `json:"dayOfWeek,omitempty"`
	RecentSessions []sessionSummary  `json:"recentSessions,omitempty"`
}

func getListeningStats(client *ABSClient) ServerTool {
	return ServerTool{
		Toolset: MetaPlayback,
		Tool: mcp.Tool{
			Name:        "get_listening_stats",
			Description: "Get your overall listening statistics including total time and daily breakdown",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: map[string]*jsonschema.Schema{},
			},
		},
		Handler: func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, error) {
			var stats ListeningStats
			if err := client.getJSON(ctx, "/api/me/listening-stats", nil, &stats); err != nil {
				return errResult("failed to get listening stats: " + err.Error())
			}

			out := listeningStatsOutput{
				TotalTime: FormatDuration(stats.TotalTime),
				Today:     FormatDuration(stats.Today),
			}
			if len(stats.DayOfWeek) > 0 {
				out.DayOfWeek = make(map[string]string, len(stats.DayOfWeek))
				for day, secs := range stats.DayOfWeek {
					out.DayOfWeek[day] = FormatDuration(secs)
				}
			}
			for _, s := range stats.RecentSessions {
				out.RecentSessions = append(out.RecentSessions, sessionSummary{
					DisplayTitle:  s.DisplayTitle,
					DisplayAuthor: s.DisplayAuthor,
					Duration:      FormatDuration(s.Duration),
					Date:          FormatTimestamp(s.StartedAt),
				})
			}
			return marshalledResult(out)
		},
	}
}

type sessionSummary struct {
	DisplayTitle  string `json:"displayTitle"`
	DisplayAuthor string `json:"displayAuthor,omitempty"`
	Duration      string `json:"duration"`
	Date          string `json:"date"`
}

func getListeningSessions(client *ABSClient) ServerTool {
	return ServerTool{
		Toolset: MetaPlayback,
		Tool: mcp.Tool{
			Name:        "get_listening_sessions",
			Description: "Get your recent listening sessions with titles, dates, and durations",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"limit": {
						Type:        "number",
						Description: "Max sessions to return (default 25)",
					},
				},
			},
		},
		Handler: func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, error) {
			limit, err := OptionalIntParam(args, "limit", 25)
			if err != nil {
				return errResult(err.Error())
			}

			params := url.Values{"itemsPerPage": {strconv.Itoa(limit)}}
			var resp struct {
				Sessions []ListeningSession `json:"sessions"`
				Total    int                `json:"total"`
			}
			if err := client.getJSON(ctx, "/api/me/listening-sessions", params, &resp); err != nil {
				return errResult("failed to get listening sessions: " + err.Error())
			}

			sessions := make([]sessionSummary, 0, len(resp.Sessions))
			for _, s := range resp.Sessions {
				sessions = append(sessions, sessionSummary{
					DisplayTitle:  s.DisplayTitle,
					DisplayAuthor: s.DisplayAuthor,
					Duration:      FormatDuration(s.Duration),
					Date:          FormatTimestamp(s.StartedAt),
				})
			}

			out := struct {
				Sessions []sessionSummary `json:"sessions"`
				Total    int              `json:"total"`
			}{Sessions: sessions, Total: resp.Total}
			return marshalledResult(out)
		},
	}
}

type progressOutput struct {
	Progress    string `json:"progress"`
	CurrentTime string `json:"currentTime"`
	Duration    string `json:"duration"`
	IsFinished  bool   `json:"isFinished"`
	LastUpdate  string `json:"lastUpdate,omitempty"`
	StartedAt   string `json:"startedAt,omitempty"`
}

func getMediaProgress(client *ABSClient) ServerTool {
	return ServerTool{
		Toolset: MetaPlayback,
		Tool: mcp.Tool{
			Name:        "get_media_progress",
			Description: "Get your reading/listening progress for a specific audiobook or podcast episode",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"itemId": {
						Type:        "string",
						Description: "Library item ID",
					},
					"episodeId": {
						Type:        "string",
						Description: "Episode ID (for podcasts)",
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
			episodeID, _, _ := OptionalParam[string](args, "episodeId")

			path := "/api/me/progress/" + sanitizePathParam(itemID)
			if episodeID != "" {
				path += "/" + sanitizePathParam(episodeID)
			}

			var prog MediaProgress
			if err := client.getJSON(ctx, path, nil, &prog); err != nil {
				return errResult("failed to get media progress: " + err.Error())
			}

			out := progressOutput{
				Progress:    FormatProgress(prog.Progress),
				CurrentTime: FormatDuration(prog.CurrentTime),
				Duration:    FormatDuration(prog.Duration),
				IsFinished:  prog.IsFinished,
				LastUpdate:  FormatTimestamp(prog.LastUpdate),
				StartedAt:   FormatTimestamp(prog.StartedAt),
			}
			return marshalledResult(out)
		},
	}
}
