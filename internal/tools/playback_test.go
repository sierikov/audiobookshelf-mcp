package tools

import (
	"testing"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/sierikov/audiobookshelf-mcp/internal/service"
)

func TestGetListeningStats(t *testing.T) {
	svc := NewMockPlaybackService(t)
	svc.EXPECT().Stats(mock.Anything).Return(service.Stats{
		Total:     61 * time.Minute,
		Today:     30 * time.Minute,
		DayOfWeek: map[string]time.Duration{"Sunday": time.Hour, "Monday": time.Minute},
		RecentSessions: []service.Session{
			{Title: "Katabasis", Author: "R.F. Kuang", Duration: 74100 * time.Second, StartedAt: time.UnixMilli(1707350400000)},
		},
	}, nil)

	result := call(t, PlaybackTools(svc), "get_listening_stats", map[string]any{})

	assertJSON(t, `{
		"totalTime":"1h 1m","today":"30m",
		"dayOfWeek":{"Sunday":"1h 0m","Monday":"1m"},
		"recentSessions":[{"displayTitle":"Katabasis","displayAuthor":"R.F. Kuang","duration":"20h 35m","date":"`+
		time.UnixMilli(1707350400000).Format("2006-01-02")+`"}]
	}`, result)
}

func TestGetListeningStats_Empty(t *testing.T) {
	svc := NewMockPlaybackService(t)
	svc.EXPECT().Stats(mock.Anything).Return(service.Stats{}, nil)

	result := call(t, PlaybackTools(svc), "get_listening_stats", map[string]any{})

	assertJSON(t, `{"totalTime":"< 1m","today":"< 1m"}`, result)
}

func TestGetListeningStats_Error(t *testing.T) {
	svc := NewMockPlaybackService(t)
	svc.EXPECT().Stats(mock.Anything).Return(service.Stats{}, errBoom)

	result := call(t, PlaybackTools(svc), "get_listening_stats", map[string]any{})

	assertToolError(t, "failed to get listening stats: ABS API 500: boom", result)
}

func TestGetListeningSessions(t *testing.T) {
	svc := NewMockPlaybackService(t)
	svc.EXPECT().Sessions(mock.Anything, 25).Return(service.SessionsPage{
		Sessions: []service.Session{{Title: "Dune", Duration: time.Hour}},
		Total:    7,
	}, nil)

	result := call(t, PlaybackTools(svc), "get_listening_sessions", map[string]any{})

	assertJSON(t, `{"sessions":[{"displayTitle":"Dune","duration":"1h 0m","date":""}],"total":7}`, result)
}

func TestGetListeningSessions_PassesLimitAndRendersEmptyList(t *testing.T) {
	svc := NewMockPlaybackService(t)
	svc.EXPECT().Sessions(mock.Anything, 3).Return(service.SessionsPage{}, nil)

	result := call(t, PlaybackTools(svc), "get_listening_sessions", map[string]any{"limit": float64(3)})

	assertJSON(t, `{"sessions":[],"total":0}`, result)
}

func TestGetListeningSessions_Error(t *testing.T) {
	svc := NewMockPlaybackService(t)
	svc.EXPECT().Sessions(mock.Anything, 25).Return(service.SessionsPage{}, errBoom)

	result := call(t, PlaybackTools(svc), "get_listening_sessions", map[string]any{})

	assertToolError(t, "failed to get listening sessions: ABS API 500: boom", result)
}

func TestGetMediaProgress(t *testing.T) {
	svc := NewMockPlaybackService(t)
	svc.EXPECT().Progress(mock.Anything, "item-1", "ep-1").Return(service.Progress{
		Fraction:    0.5,
		CurrentTime: 6 * time.Hour,
		Duration:    12 * time.Hour,
		IsFinished:  true,
		LastUpdate:  time.UnixMilli(1668586015691),
		StartedAt:   time.UnixMilli(1668120083771),
	}, nil)

	result := call(t, PlaybackTools(svc), "get_media_progress", map[string]any{"itemId": "item-1", "episodeId": "ep-1"})

	assertJSON(t, `{
		"progress":"50.0%","currentTime":"6h 0m","duration":"12h 0m","isFinished":true,
		"lastUpdate":"`+time.UnixMilli(1668586015691).Format("2006-01-02")+`",
		"startedAt":"`+time.UnixMilli(1668120083771).Format("2006-01-02")+`"
	}`, result)
}

func TestGetMediaProgress_UnknownTimestampsAreOmitted(t *testing.T) {
	svc := NewMockPlaybackService(t)
	svc.EXPECT().Progress(mock.Anything, "item-1", "").Return(service.Progress{}, nil)

	result := call(t, PlaybackTools(svc), "get_media_progress", map[string]any{"itemId": "item-1"})

	assertJSON(t, `{"progress":"0.0%","currentTime":"< 1m","duration":"< 1m","isFinished":false}`, result)
}

func TestGetMediaProgress_MissingItemID(t *testing.T) {
	result := call(t, PlaybackTools(NewMockPlaybackService(t)), "get_media_progress", map[string]any{})

	assertToolError(t, "missing required parameter: itemId", result)
}

func TestGetMediaProgress_Error(t *testing.T) {
	svc := NewMockPlaybackService(t)
	svc.EXPECT().Progress(mock.Anything, "item-1", "").Return(service.Progress{}, errBoom)

	result := call(t, PlaybackTools(svc), "get_media_progress", map[string]any{"itemId": "item-1"})

	assertToolError(t, "failed to get media progress: ABS API 500: boom", result)
}
