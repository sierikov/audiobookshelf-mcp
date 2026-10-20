package tools

import (
	"fmt"
	"time"

	"github.com/sierikov/audiobookshelf-mcp/internal/service"
)

// FormatDuration renders a duration for people, truncated to minutes.
// We wrap time.Duration rather than using .String() directly because its
// default format ("1h1m0s") is harder for LLMs to parse — spaces and
// dropping the zero-second suffix ("1h 0m") reads more naturally.
func FormatDuration(d time.Duration) string {
	d = d.Truncate(time.Minute)
	if d < time.Minute {
		return "< 1m"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

// FormatClock renders a position on a book's timeline as h:mm:ss, truncated
// to whole seconds.
func FormatClock(d time.Duration) string {
	s := int(d / time.Second)
	return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
}

// FormatProgress converts a 0.0–1.0 progress value to a percentage string.
func FormatProgress(progress float64) string {
	return fmt.Sprintf("%.1f%%", progress*100)
}

// FormatTimestamp renders a timestamp as an ISO date; the zero time is "".
func FormatTimestamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

// itemDuration renders an item's duration, or "" when it has none
// (podcasts), so the JSON field is omitted.
func itemDuration(it service.Item) string {
	if !it.HasDuration {
		return ""
	}
	return FormatDuration(it.Duration)
}
