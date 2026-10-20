package service

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildChapters_ComputesEnds(t *testing.T) {
	got, err := buildChapters([]ChapterInput{
		{Start: 0, Title: "Opening"},
		{Start: 600 * time.Second, Title: "One"},
		{Start: 3000 * time.Second, Title: "Two"},
	}, time.Hour)

	require.NoError(t, err)
	assert.Equal(t, []Chapter{
		{Start: 0, End: 600 * time.Second, Title: "Opening"},
		{Start: 600 * time.Second, End: 3000 * time.Second, Title: "One"},
		{Start: 3000 * time.Second, End: time.Hour, Title: "Two"},
	}, got)
}

func TestBuildChapters_TrimsTitles(t *testing.T) {
	got, err := buildChapters([]ChapterInput{{Start: 0, Title: "  Opening "}}, time.Hour)

	require.NoError(t, err)
	assert.Equal(t, []Chapter{{Start: 0, End: time.Hour, Title: "Opening"}}, got)
}

func TestBuildChapters_EmptyIsAllowed(t *testing.T) {
	got, err := buildChapters(nil, time.Hour)

	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

func TestBuildChapters_Rejects(t *testing.T) {
	at := func(secs ...int) []ChapterInput {
		in := make([]ChapterInput, 0, len(secs))
		for _, s := range secs {
			in = append(in, ChapterInput{Start: time.Duration(s) * time.Second, Title: "T"})
		}
		return in
	}
	blankSecond := at(0, 600)
	blankSecond[1].Title = "   "

	tests := []struct {
		name    string
		in      []ChapterInput
		chapter int
		want    string
	}{
		{"first not at zero", at(5), 1, "chapter 1 starts at 5s; the first chapter must start at 0s"},
		{"same start as previous", at(0, 600, 600), 3, "chapter 3 starts at 600s, not after chapter 2 (600s)"},
		{"before previous", at(0, 600, 300), 3, "chapter 3 starts at 300s, not after chapter 2 (600s)"},
		{"at the end of the book", at(0, 3600), 2, "chapter 2 starts at 3600s, at or after the end of the book (3600s)"},
		{"blank title", blankSecond, 2, "chapter 2 has no title"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildChapters(tt.in, time.Hour)

			chErr, ok := errors.AsType[*ChapterError](err)
			require.True(t, ok, "want *ChapterError, got %v", err)
			assert.Equal(t, tt.chapter, chErr.Chapter)
			assert.EqualError(t, err, tt.want)
		})
	}
}

func TestBuildChapters_FractionalSecondsInErrors(t *testing.T) {
	_, err := buildChapters([]ChapterInput{{Start: 2500 * time.Millisecond, Title: "A"}}, time.Hour)

	require.EqualError(t, err, "chapter 1 starts at 2.5s; the first chapter must start at 0s")
}

func TestAudibleProvider(t *testing.T) {
	tests := []struct {
		in, provider, region string
	}{
		{"", "audible", "us"},
		{"us", "audible", "us"},
		{"de", "audible.de", "de"},
		{" DE ", "audible.de", "de"},
		{"es", "audible.es", "es"},
	}
	for _, tt := range tests {
		provider, region, err := audibleProvider(tt.in)

		require.NoError(t, err, "region %q", tt.in)
		assert.Equal(t, tt.provider, provider, "region %q", tt.in)
		assert.Equal(t, tt.region, region, "region %q", tt.in)
	}
}

func TestAudibleProvider_UnknownRegion(t *testing.T) {
	_, _, err := audibleProvider("xx")

	require.ErrorIs(t, err, ErrUnknownRegion)
	assert.EqualError(t, err, `unknown Audible region "xx" (use one of: us, ca, uk, au, fr, de, jp, it, in, es)`)
}

func TestExactSeconds_Rounds(t *testing.T) {
	assert.Equal(t, 1234567*time.Millisecond, exactSeconds(1234.567))
}

func TestCheckTitles(t *testing.T) {
	got, err := checkTitles([]string{" Opening ", "The Storm"})

	require.NoError(t, err)
	assert.Equal(t, []string{"Opening", "The Storm"}, got)
}

func TestCheckTitles_Blank(t *testing.T) {
	_, err := checkTitles([]string{"A", ""})

	chErr, ok := errors.AsType[*ChapterError](err)
	require.True(t, ok, "want *ChapterError, got %v", err)
	assert.Equal(t, 2, chErr.Chapter)
	assert.EqualError(t, err, "chapter 2 has no title")
}
