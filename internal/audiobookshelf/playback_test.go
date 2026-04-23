package audiobookshelf

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type PlaybackSuite struct {
	ToolSuite
}

func TestPlaybackSuite(t *testing.T) {
	suite.Run(t, new(PlaybackSuite))
}

func (s *PlaybackSuite) TestGetListeningStats() {
	s.routes["/api/me/listening-stats"] = map[string]any{
		"totalTime": 3660.0,
		"today":     1800.0,
		"dayOfWeek": map[string]any{"Sunday": 3600.0, "Monday": 60.0},
		"recentSessions": []map[string]any{
			session("sess-1", "item-1", "Katabasis", "R.F. Kuang", 74100.0, 1707350400000),
		},
	}
	s.StartServer()

	tool := findTool(PlaybackTools(s.client), "get_listening_stats")
	result := s.callTool(tool, map[string]any{})

	var stats listeningStatsOutput
	s.resultJSON(result, &stats)

	s.Equal("1h 1m", stats.TotalTime)
	s.Equal("30m", stats.Today)
	s.Equal("1h 0m", stats.DayOfWeek["Sunday"])
	s.Equal("1m", stats.DayOfWeek["Monday"])
	s.Require().Len(stats.RecentSessions, 1)
	s.Equal("Katabasis", stats.RecentSessions[0].DisplayTitle)
	s.Equal("20h 35m", stats.RecentSessions[0].Duration)
}

func (s *PlaybackSuite) TestGetListeningSessions() {
	s.routes["/api/me/listening-sessions"] = map[string]any{
		"sessions": []map[string]any{
			session("sess-1", "item-1", "Dune", "Frank Herbert", 3600.0, 1707350400000),
			session("sess-2", "item-2", "Foundation", "Isaac Asimov", 1800.0, 1707264000000),
		},
		"total": 2,
	}
	s.StartServer()

	tool := findTool(PlaybackTools(s.client), "get_listening_sessions")
	result := s.callTool(tool, map[string]any{})

	var out struct {
		Sessions []sessionSummary `json:"sessions"`
		Total    int              `json:"total"`
	}
	s.resultJSON(result, &out)

	s.Equal(2, out.Total)
	s.Require().Len(out.Sessions, 2)
	s.Equal("Dune", out.Sessions[0].DisplayTitle)
	s.Equal("1h 0m", out.Sessions[0].Duration)
	s.Equal("30m", out.Sessions[1].Duration)
}

func (s *PlaybackSuite) TestGetMediaProgress() {
	s.routes["/api/me/progress/item-1"] = progress("prog-1", "item-1", 43200.0, 0.5, 21600.0, false)
	s.StartServer()

	tool := findTool(PlaybackTools(s.client), "get_media_progress")
	result := s.callTool(tool, map[string]any{"itemId": "item-1"})

	var out progressOutput
	s.resultJSON(result, &out)

	s.Equal("50.0%", out.Progress)
	s.Equal("6h 0m", out.CurrentTime)
	s.Equal("12h 0m", out.Duration)
	s.False(out.IsFinished)
	s.Equal("2022-11-16", out.LastUpdate)
}

func (s *PlaybackSuite) TestGetMediaProgress_WithEpisode() {
	s.routes["/api/me/progress/item-1/ep-1"] = progress("prog-ep-1", "item-1", 1800.0, 1.0, 1800.0, true)
	s.StartServer()

	tool := findTool(PlaybackTools(s.client), "get_media_progress")
	result := s.callTool(tool, map[string]any{"itemId": "item-1", "episodeId": "ep-1"})

	var out progressOutput
	s.resultJSON(result, &out)

	s.Equal("100.0%", out.Progress)
	s.True(out.IsFinished)
}

func (s *PlaybackSuite) TestGetMediaProgress_NotFound() {
	s.StartServer()

	tool := findTool(PlaybackTools(s.client), "get_media_progress")
	result := s.callTool(tool, map[string]any{"itemId": "nonexistent"})

	s.True(result.IsError)
}
