package tools

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "< 1m"},
		{30 * time.Second, "< 1m"},
		{59 * time.Second, "< 1m"},
		{60 * time.Second, "1m"},
		{90 * time.Second, "1m"},
		{3600 * time.Second, "1h 0m"},
		{3661 * time.Second, "1h 1m"},
		{7380 * time.Second, "2h 3m"},
		{74100 * time.Second, "20h 35m"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, FormatDuration(tt.d), "FormatDuration(%v)", tt.d)
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
		assert.Equal(t, tt.want, FormatProgress(tt.progress), "FormatProgress(%v)", tt.progress)
	}
}

func TestFormatTimestamp(t *testing.T) {
	tests := []struct {
		at   time.Time
		want string
	}{
		{time.Time{}, ""},
		{time.UnixMilli(1668586015691), "2022-11-16"},
		{time.UnixMilli(1707350400000), "2024-02-08"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, FormatTimestamp(tt.at), "FormatTimestamp(%v)", tt.at)
	}
}

func TestFormatClock(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0:00:00"},
		{59 * time.Second, "0:00:59"},
		{3723 * time.Second, "1:02:03"},
		{45000*time.Second + 900*time.Millisecond, "12:30:00"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, FormatClock(tt.d), "FormatClock(%v)", tt.d)
	}
}
