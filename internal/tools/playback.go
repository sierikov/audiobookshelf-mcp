package tools

import (
	"context"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sierikov/audiobookshelf-mcp/internal/service"
)

// PlaybackTools returns the tools in the playback toolset.
func PlaybackTools(svc PlaybackService) []ServerTool {
	return []ServerTool{
		getListeningStats(svc),
		getListeningSessions(svc),
		getMediaProgress(svc),
	}
}

type listeningStatsOutput struct {
	TotalTime      string            `json:"totalTime"`
	Today          string            `json:"today"`
	DayOfWeek      map[string]string `json:"dayOfWeek,omitempty"`
	RecentSessions []sessionSummary  `json:"recentSessions,omitempty"`
}

func getListeningStats(svc PlaybackService) ServerTool {
	return ServerTool{
		Toolset: metaPlayback,
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
			stats, err := svc.Stats(ctx)
			if err != nil {
				return errResult("failed to get listening stats: " + err.Error())
			}

			out := listeningStatsOutput{
				TotalTime: FormatDuration(stats.Total),
				Today:     FormatDuration(stats.Today),
			}
			if len(stats.DayOfWeek) > 0 {
				out.DayOfWeek = make(map[string]string, len(stats.DayOfWeek))
				for day, d := range stats.DayOfWeek {
					out.DayOfWeek[day] = FormatDuration(d)
				}
			}
			for _, s := range stats.RecentSessions {
				out.RecentSessions = append(out.RecentSessions, toSessionSummary(s))
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

func toSessionSummary(s service.Session) sessionSummary {
	return sessionSummary{
		DisplayTitle:  s.Title,
		DisplayAuthor: s.Author,
		Duration:      FormatDuration(s.Duration),
		Date:          FormatTimestamp(s.StartedAt),
	}
}

func getListeningSessions(svc PlaybackService) ServerTool {
	return ServerTool{
		Toolset: metaPlayback,
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

			page, err := svc.Sessions(ctx, limit)
			if err != nil {
				return errResult("failed to get listening sessions: " + err.Error())
			}

			sessions := make([]sessionSummary, 0, len(page.Sessions))
			for _, s := range page.Sessions {
				sessions = append(sessions, toSessionSummary(s))
			}
			out := struct {
				Sessions []sessionSummary `json:"sessions"`
				Total    int              `json:"total"`
			}{Sessions: sessions, Total: page.Total}
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

func getMediaProgress(svc PlaybackService) ServerTool {
	return ServerTool{
		Toolset: metaPlayback,
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

			p, err := svc.Progress(ctx, itemID, episodeID)
			if err != nil {
				return errResult("failed to get media progress: " + err.Error())
			}

			return marshalledResult(progressOutput{
				Progress:    FormatProgress(p.Fraction),
				CurrentTime: FormatDuration(p.CurrentTime),
				Duration:    FormatDuration(p.Duration),
				IsFinished:  p.IsFinished,
				LastUpdate:  FormatTimestamp(p.LastUpdate),
				StartedAt:   FormatTimestamp(p.StartedAt),
			})
		},
	}
}
