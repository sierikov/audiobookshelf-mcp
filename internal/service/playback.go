package service

import (
	"context"
	"time"
)

// Playback reports listening statistics, sessions and progress.
type Playback struct {
	api PlaybackAPI
}

// NewPlayback creates a Playback service backed by api.
func NewPlayback(api PlaybackAPI) *Playback {
	return &Playback{api: api}
}

// Stats returns the user's aggregated listening statistics.
func (s *Playback) Stats(ctx context.Context) (Stats, error) {
	st, err := s.api.ListeningStats(ctx)
	if err != nil {
		return Stats{}, err
	}
	out := Stats{
		Total:          seconds(st.TotalTime),
		Today:          seconds(st.Today),
		RecentSessions: toSessions(st.RecentSessions),
	}
	if len(st.DayOfWeek) > 0 {
		out.DayOfWeek = make(map[string]time.Duration, len(st.DayOfWeek))
		for day, secs := range st.DayOfWeek {
			out.DayOfWeek[day] = seconds(secs)
		}
	}
	return out, nil
}

// Sessions returns up to limit of the user's most recent listening sessions.
func (s *Playback) Sessions(ctx context.Context, limit int) (SessionsPage, error) {
	page, err := s.api.ListeningSessions(ctx, limit)
	if err != nil {
		return SessionsPage{}, err
	}
	return SessionsPage{Sessions: toSessions(page.Sessions), Total: page.Total}, nil
}

// Progress returns the user's progress on an item, or on one podcast episode
// when episodeID is set.
func (s *Playback) Progress(ctx context.Context, itemID, episodeID string) (Progress, error) {
	p, err := s.api.Progress(ctx, itemID, episodeID)
	if err != nil {
		return Progress{}, err
	}
	return toProgress(p), nil
}
