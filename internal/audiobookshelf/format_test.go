package audiobookshelf

import "testing"

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		seconds float64
		want    string
	}{
		{0, "< 1m"},
		{30, "< 1m"},
		{59, "< 1m"},
		{60, "1m"},
		{90, "1m"},
		{3600, "1h 0m"},
		{3661, "1h 1m"},
		{7380, "2h 3m"},
		{74100, "20h 35m"},
	}
	for _, tt := range tests {
		got := FormatDuration(tt.seconds)
		if got != tt.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", tt.seconds, got, tt.want)
		}
	}
}

func TestFormatProgress(t *testing.T) {
	tests := []struct {
		progress float64
		want     string
	}{
		{0, "0.0%"},
		{0.5, "50.0%"},
		{0.048, "4.8%"},
		{1.0, "100.0%"},
		{0.999, "99.9%"},
	}
	for _, tt := range tests {
		got := FormatProgress(tt.progress)
		if got != tt.want {
			t.Errorf("FormatProgress(%v) = %q, want %q", tt.progress, got, tt.want)
		}
	}
}

func TestFormatTimestamp(t *testing.T) {
	tests := []struct {
		epochMs int64
		want    string
	}{
		{0, ""},
		{1668586015691, "2022-11-16"},
		{1707350400000, "2024-02-08"},
	}
	for _, tt := range tests {
		got := FormatTimestamp(tt.epochMs)
		if got != tt.want {
			t.Errorf("FormatTimestamp(%v) = %q, want %q", tt.epochMs, got, tt.want)
		}
	}
}
