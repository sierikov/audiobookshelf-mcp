package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
)

func TestPlayback_Stats(t *testing.T) {
	api := NewMockPlaybackAPI(t)
	api.EXPECT().ListeningStats(mock.Anything).Return(abs.ListeningStats{
		TotalTime: 3660,
		Today:     1800,
		DayOfWeek: map[string]float64{"Sunday": 3600, "Monday": 60},
		RecentSessions: []abs.ListeningSession{
			{DisplayTitle: "Katabasis", DisplayAuthor: "R.F. Kuang", Duration: 74100, StartedAt: 1707350400000},
		},
	}, nil)

	stats, err := NewPlayback(api).Stats(context.Background())

	require.NoError(t, err)
	assert.Equal(t, Stats{
		Total:     61 * time.Minute,
		Today:     30 * time.Minute,
		DayOfWeek: map[string]time.Duration{"Sunday": time.Hour, "Monday": time.Minute},
		RecentSessions: []Session{
			{Title: "Katabasis", Author: "R.F. Kuang", Duration: 74100 * time.Second, StartedAt: time.UnixMilli(1707350400000)},
		},
	}, stats)
}

func TestPlayback_Stats_EmptyStatsHaveNoBreakdown(t *testing.T) {
	api := NewMockPlaybackAPI(t)
	api.EXPECT().ListeningStats(mock.Anything).Return(abs.ListeningStats{}, nil)

	stats, err := NewPlayback(api).Stats(context.Background())

	require.NoError(t, err)
	assert.Nil(t, stats.DayOfWeek)
	assert.Nil(t, stats.RecentSessions)
}

func TestPlayback_Stats_ReturnsAPIErrorUnchanged(t *testing.T) {
	api := NewMockPlaybackAPI(t)
	api.EXPECT().ListeningStats(mock.Anything).Return(abs.ListeningStats{}, errAPI)

	_, err := NewPlayback(api).Stats(context.Background())

	require.Equal(t, errAPI, err)
}

func TestPlayback_Sessions(t *testing.T) {
	api := NewMockPlaybackAPI(t)
	api.EXPECT().ListeningSessions(mock.Anything, 25).Return(abs.SessionsPage{
		Sessions: []abs.ListeningSession{
			{DisplayTitle: "Dune", DisplayAuthor: "Frank Herbert", Duration: 3600, StartedAt: 1707350400000},
			{DisplayTitle: "Foundation", Duration: 1800},
		},
		Total: 9,
	}, nil)

	page, err := NewPlayback(api).Sessions(context.Background(), 25)

	require.NoError(t, err)
	assert.Equal(t, SessionsPage{
		Sessions: []Session{
			{Title: "Dune", Author: "Frank Herbert", Duration: time.Hour, StartedAt: time.UnixMilli(1707350400000)},
			{Title: "Foundation", Duration: 30 * time.Minute},
		},
		Total: 9,
	}, page)
}

func TestPlayback_Sessions_ReturnsAPIErrorUnchanged(t *testing.T) {
	api := NewMockPlaybackAPI(t)
	api.EXPECT().ListeningSessions(mock.Anything, 25).Return(abs.SessionsPage{}, errAPI)

	_, err := NewPlayback(api).Sessions(context.Background(), 25)

	require.Equal(t, errAPI, err)
}

func TestPlayback_Progress(t *testing.T) {
	api := NewMockPlaybackAPI(t)
	api.EXPECT().Progress(mock.Anything, "item-1", "ep-1").Return(abs.MediaProgress{
		Duration: 43200, Progress: 0.5, CurrentTime: 21600, IsFinished: true,
		LastUpdate: 1668586015691, StartedAt: 1668120083771,
	}, nil)

	prog, err := NewPlayback(api).Progress(context.Background(), "item-1", "ep-1")

	require.NoError(t, err)
	assert.Equal(t, Progress{
		Fraction:    0.5,
		CurrentTime: 6 * time.Hour,
		Duration:    12 * time.Hour,
		Remaining:   6 * time.Hour,
		IsFinished:  true,
		LastUpdate:  time.UnixMilli(1668586015691),
		StartedAt:   time.UnixMilli(1668120083771),
	}, prog)
}

func TestPlayback_Progress_UnknownTimestampsAreZero(t *testing.T) {
	api := NewMockPlaybackAPI(t)
	api.EXPECT().Progress(mock.Anything, "item-1", "").Return(abs.MediaProgress{}, nil)

	prog, err := NewPlayback(api).Progress(context.Background(), "item-1", "")

	require.NoError(t, err)
	assert.True(t, prog.LastUpdate.IsZero())
	assert.True(t, prog.StartedAt.IsZero())
}

func TestPlayback_Progress_ReturnsAPIErrorUnchanged(t *testing.T) {
	api := NewMockPlaybackAPI(t)
	api.EXPECT().Progress(mock.Anything, "item-1", "").Return(abs.MediaProgress{}, errAPI)

	_, err := NewPlayback(api).Progress(context.Background(), "item-1", "")

	require.Equal(t, errAPI, err)
}
