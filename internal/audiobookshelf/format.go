package audiobookshelf

import (
	"fmt"
	"time"
)

// FormatDuration converts seconds to a human-readable duration string.
// We wrap time.Duration rather than using .String() directly because its
// default format ("1h1m0s") is harder for LLMs to parse — spaces and
// dropping the zero-second suffix ("1h 0m") reads more naturally.
func FormatDuration(seconds float64) string {
	d := time.Duration(seconds * float64(time.Second)).Truncate(time.Minute)
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

// FormatProgress converts a 0.0–1.0 progress value to a percentage string.
func FormatProgress(progress float64) string {
	return fmt.Sprintf("%.1f%%", progress*100)
}

// FormatTimestamp converts a millisecond epoch timestamp to an ISO date string.
func FormatTimestamp(epochMs int64) string {
	if epochMs == 0 {
		return ""
	}
	return time.UnixMilli(epochMs).Format("2006-01-02")
}
