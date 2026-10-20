package service

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
)

// duneWithChapters is a one-hour book with two chapters and two audio files.
func duneWithChapters() abs.LibraryItem {
	return bookWithChapters("item-1", "Dune", "Frank Herbert", "B002V1OF70", 3600,
		[]abs.Chapter{
			{ID: 0, Start: 0, End: 1800, Title: "Chapter 1"},
			{ID: 1, Start: 1800, End: 3600, Title: "Chapter 2"},
		},
		[]abs.AudioTrack{
			{Index: 1, Title: "m001.mp3", StartOffset: 0, Duration: 1200.5},
			{Index: 2, Title: "m002.mp3", StartOffset: 1200.5, Duration: 2399.5},
		},
	)
}

func TestChapters_Get(t *testing.T) {
	api := NewMockChaptersAPI(t)
	api.EXPECT().Item(mock.Anything, "item-1").Return(duneWithChapters(), nil)

	got, err := NewChapters(api).Get(t.Context(), "item-1")

	require.NoError(t, err)
	assert.Equal(t, BookChapters{
		ID: "item-1", Title: "Dune", Author: "Frank Herbert", ASIN: "B002V1OF70", Duration: time.Hour,
		Chapters: []Chapter{
			{Start: 0, End: 30 * time.Minute, Title: "Chapter 1"},
			{Start: 30 * time.Minute, End: time.Hour, Title: "Chapter 2"},
		},
		Files: []AudioFile{
			{Index: 1, Filename: "m001.mp3", Start: 0, Duration: 1200500 * time.Millisecond},
			{Index: 2, Filename: "m002.mp3", Start: 1200500 * time.Millisecond, Duration: 2399500 * time.Millisecond},
		},
	}, got)
}

func TestChapters_Get_NoChaptersIsEmpty(t *testing.T) {
	api := NewMockChaptersAPI(t)
	api.EXPECT().Item(mock.Anything, "item-1").Return(bookWithChapters("item-1", "Dune", "Frank Herbert", "", 3600, nil, nil), nil)

	got, err := NewChapters(api).Get(t.Context(), "item-1")

	require.NoError(t, err)
	assert.Empty(t, got.Chapters)
	assert.Empty(t, got.Files)
}

func TestChapters_Get_PodcastIsNotABook(t *testing.T) {
	api := NewMockChaptersAPI(t)
	api.EXPECT().Item(mock.Anything, "pod-1").Return(podcastItem("pod-1", "Hardcore History", "Dan Carlin", 68), nil)

	_, err := NewChapters(api).Get(t.Context(), "pod-1")

	require.ErrorIs(t, err, ErrNotABook)
	assert.EqualError(t, err, "item pod-1 is a podcast: only books have chapters")
}

func TestChapters_Get_ReturnsAPIErrorUnchanged(t *testing.T) {
	api := NewMockChaptersAPI(t)
	api.EXPECT().Item(mock.Anything, "nope").Return(abs.LibraryItem{}, errAPI)

	_, err := NewChapters(api).Get(t.Context(), "nope")

	require.Equal(t, errAPI, err)
}

func TestChapters_Get_UndecodableMedia(t *testing.T) {
	api := NewMockChaptersAPI(t)
	api.EXPECT().Item(mock.Anything, "item-1").Return(brokenItem("item-1", abs.MediaTypeBook), nil)

	_, err := NewChapters(api).Get(t.Context(), "item-1")

	_, ok := errors.AsType[*MediaError](err)
	assert.True(t, ok, "want *MediaError, got %v", err)
}

func TestChapters_SearchAudible(t *testing.T) {
	api := NewMockChaptersAPI(t)
	api.EXPECT().Item(mock.Anything, "item-1").Return(duneWithChapters(), nil)
	api.EXPECT().SearchBooks(mock.Anything, "audible.de", "Dune", "Frank Herbert", "item-1").Return([]abs.BookMatch{
		{Title: "Dune", Subtitle: "Der Wüstenplanet", Author: "Frank Herbert", Narrator: "Jürgen Holdorf",
			ASIN: "B07X1DUNE1", Language: "German", Duration: 61, MatchConfidence: new(0.9)},
		{Title: "Dune", Author: "Frank Herbert", ASIN: "B07X1DUNE2", Duration: 30, Abridged: true},
	}, nil)

	got, err := NewChapters(api).SearchAudible(t.Context(), "item-1", "de", "", "")

	require.NoError(t, err)
	assert.Equal(t, AudibleSearch{
		Book:   book("item-1", "Dune", "Frank Herbert", time.Hour),
		Region: "de",
		Matches: []AudibleMatch{
			{ASIN: "B07X1DUNE1", Title: "Dune", Subtitle: "Der Wüstenplanet", Author: "Frank Herbert",
				Narrator: "Jürgen Holdorf", Language: "German",
				Runtime: 61 * time.Minute, RuntimeDiff: time.Minute, MatchConfidence: new(0.9)},
			{ASIN: "B07X1DUNE2", Title: "Dune", Author: "Frank Herbert", Abridged: true,
				Runtime: 30 * time.Minute, RuntimeDiff: -30 * time.Minute},
		},
	}, got)
}

func TestChapters_SearchAudible_ExplicitTitleAndAuthor(t *testing.T) {
	api := NewMockChaptersAPI(t)
	api.EXPECT().Item(mock.Anything, "item-1").Return(duneWithChapters(), nil)
	api.EXPECT().SearchBooks(mock.Anything, "audible", "Der Wüstenplanet", "Herbert", "item-1").Return(nil, nil)

	got, err := NewChapters(api).SearchAudible(t.Context(), "item-1", "", "Der Wüstenplanet", "Herbert")

	require.NoError(t, err)
	assert.Equal(t, "us", got.Region)
	assert.Empty(t, got.Matches)
}

func TestChapters_SearchAudible_UnknownRegion(t *testing.T) {
	api := NewMockChaptersAPI(t)

	_, err := NewChapters(api).SearchAudible(t.Context(), "item-1", "xx", "", "")

	require.ErrorIs(t, err, ErrUnknownRegion)
}

func TestChapters_SearchAudible_PodcastIsNotABook(t *testing.T) {
	api := NewMockChaptersAPI(t)
	api.EXPECT().Item(mock.Anything, "pod-1").Return(podcastItem("pod-1", "Hardcore History", "Dan Carlin", 68), nil)

	_, err := NewChapters(api).SearchAudible(t.Context(), "pod-1", "us", "", "")

	require.ErrorIs(t, err, ErrNotABook)
}

func TestChapters_AudibleChapters(t *testing.T) {
	api := NewMockChaptersAPI(t)
	api.EXPECT().AudibleChapters(mock.Anything, "B002V1OF70", "us").Return(abs.AudibleChapters{
		ASIN: "B002V1OF70", Region: "us",
		BrandIntroDurationMs: 2043, BrandOutroDurationMs: 5061, RuntimeLengthMs: 3600000,
		IsAccurate: true,
		Chapters: []abs.AudibleChapter{
			{StartOffsetMs: 0, LengthMs: 600000, Title: "Opening Credits"},
			{StartOffsetMs: 600000, LengthMs: 3000000, Title: "Chapter 1"},
		},
	}, nil)

	got, err := NewChapters(api).AudibleChapters(t.Context(), "B002V1OF70", " US ")

	require.NoError(t, err)
	assert.Equal(t, AudibleChapters{
		ASIN: "B002V1OF70", Region: "us",
		Runtime: time.Hour, BrandIntro: 2043 * time.Millisecond, BrandOutro: 5061 * time.Millisecond,
		Accurate: true,
		Chapters: []Chapter{
			{Start: 0, End: 10 * time.Minute, Title: "Opening Credits"},
			{Start: 10 * time.Minute, End: time.Hour, Title: "Chapter 1"},
		},
	}, got)
}

func TestChapters_AudibleChapters_UnknownRegion(t *testing.T) {
	_, err := NewChapters(NewMockChaptersAPI(t)).AudibleChapters(t.Context(), "B002V1OF70", "mars")

	require.ErrorIs(t, err, ErrUnknownRegion)
}

func TestChapters_AudibleChapters_ReturnsAPIErrorUnchanged(t *testing.T) {
	api := NewMockChaptersAPI(t)
	api.EXPECT().AudibleChapters(mock.Anything, "B000000000", "us").Return(abs.AudibleChapters{}, errAPI)

	_, err := NewChapters(api).AudibleChapters(t.Context(), "B000000000", "")

	require.Equal(t, errAPI, err)
}

func TestChapters_Set(t *testing.T) {
	api := NewMockChaptersAPI(t)
	api.EXPECT().Item(mock.Anything, "item-1").Return(duneWithChapters(), nil)
	api.EXPECT().SaveChapters(mock.Anything, "item-1", []abs.Chapter{
		{ID: 0, Start: 0, End: 600, Title: "Opening"},
		{ID: 1, Start: 600, End: 3600, Title: "One"},
	}).Return(true, nil)

	got, err := NewChapters(api).Set(t.Context(), "item-1", []ChapterInput{
		{Start: 0, Title: "Opening"},
		{Start: 10 * time.Minute, Title: "One"},
	})

	require.NoError(t, err)
	assert.Equal(t, ChapterChange{
		Updated: true,
		Previous: []Chapter{
			{Start: 0, End: 30 * time.Minute, Title: "Chapter 1"},
			{Start: 30 * time.Minute, End: time.Hour, Title: "Chapter 2"},
		},
		Saved: []Chapter{
			{Start: 0, End: 10 * time.Minute, Title: "Opening"},
			{Start: 10 * time.Minute, End: time.Hour, Title: "One"},
		},
	}, got)
}

func TestChapters_Set_EmptyClearsChapters(t *testing.T) {
	api := NewMockChaptersAPI(t)
	api.EXPECT().Item(mock.Anything, "item-1").Return(duneWithChapters(), nil)
	api.EXPECT().SaveChapters(mock.Anything, "item-1", []abs.Chapter{}).Return(true, nil)

	got, err := NewChapters(api).Set(t.Context(), "item-1", nil)

	require.NoError(t, err)
	assert.Empty(t, got.Saved)
	assert.Len(t, got.Previous, 2)
}

func TestChapters_Set_InvalidListIsNotSent(t *testing.T) {
	api := NewMockChaptersAPI(t)
	api.EXPECT().Item(mock.Anything, "item-1").Return(duneWithChapters(), nil)

	_, err := NewChapters(api).Set(t.Context(), "item-1", []ChapterInput{
		{Start: 0, Title: "Opening"},
		{Start: 2 * time.Hour, Title: "Past the end"},
	})

	_, ok := errors.AsType[*ChapterError](err)
	require.True(t, ok, "want *ChapterError, got %v", err)
	assert.EqualError(t, err, "chapter 2 starts at 7200s, at or after the end of the book (3600s)")
}

func TestChapters_Set_NoAudio(t *testing.T) {
	api := NewMockChaptersAPI(t)
	api.EXPECT().Item(mock.Anything, "item-1").Return(bookWithChapters("item-1", "Dune", "Frank Herbert", "", 0, nil, nil), nil)

	_, err := NewChapters(api).Set(t.Context(), "item-1", []ChapterInput{{Start: 0, Title: "Opening"}})

	require.ErrorIs(t, err, ErrNoAudio)
	assert.EqualError(t, err, "item item-1: the book has no audio files")
}

func TestChapters_Set_PodcastIsNotABook(t *testing.T) {
	api := NewMockChaptersAPI(t)
	api.EXPECT().Item(mock.Anything, "pod-1").Return(podcastItem("pod-1", "Hardcore History", "Dan Carlin", 68), nil)

	_, err := NewChapters(api).Set(t.Context(), "pod-1", []ChapterInput{{Start: 0, Title: "Opening"}})

	require.ErrorIs(t, err, ErrNotABook)
}

func TestChapters_Set_SaveErrorReturnedUnchanged(t *testing.T) {
	api := NewMockChaptersAPI(t)
	api.EXPECT().Item(mock.Anything, "item-1").Return(duneWithChapters(), nil)
	api.EXPECT().SaveChapters(mock.Anything, "item-1", mock.Anything).Return(false, errAPI)

	_, err := NewChapters(api).Set(t.Context(), "item-1", []ChapterInput{{Start: 0, Title: "Opening"}})

	require.Equal(t, errAPI, err)
}

func TestChapters_Rename_KeepsTimesExactly(t *testing.T) {
	// The second chapter ends past the book's 3600s on purpose: renaming
	// never checks or touches times, and sends them back bit for bit.
	api := NewMockChaptersAPI(t)
	api.EXPECT().Item(mock.Anything, "item-1").Return(bookWithChapters("item-1", "Dune", "Frank Herbert", "", 3600,
		[]abs.Chapter{
			{ID: 0, Start: 0, End: 600.1234567891, Title: "Chapter 1"},
			{ID: 1, Start: 600.1234567891, End: 99999, Title: "Chapter 2"},
		}, oneTrack(3600)), nil)
	api.EXPECT().SaveChapters(mock.Anything, "item-1", []abs.Chapter{
		{ID: 0, Start: 0, End: 600.1234567891, Title: "Opening"},
		{ID: 1, Start: 600.1234567891, End: 99999, Title: "The Storm"},
	}).Return(true, nil)

	got, err := NewChapters(api).Rename(t.Context(), "item-1", []string{"Opening", " The Storm "})

	require.NoError(t, err)
	assert.True(t, got.Updated)
	assert.Equal(t, []string{"Chapter 1", "Chapter 2"}, []string{got.Previous[0].Title, got.Previous[1].Title})
	assert.Equal(t, []string{"Opening", "The Storm"}, []string{got.Saved[0].Title, got.Saved[1].Title})
}

func TestChapters_Rename_CountMismatch(t *testing.T) {
	api := NewMockChaptersAPI(t)
	api.EXPECT().Item(mock.Anything, "item-1").Return(duneWithChapters(), nil)

	_, err := NewChapters(api).Rename(t.Context(), "item-1", []string{"Only one"})

	require.ErrorIs(t, err, ErrTitleCount)
	assert.EqualError(t, err, "need one title per chapter: got 1 titles for 2 chapters")
}

func TestChapters_Rename_NoChapters(t *testing.T) {
	api := NewMockChaptersAPI(t)
	api.EXPECT().Item(mock.Anything, "item-1").Return(bookWithChapters("item-1", "Dune", "Frank Herbert", "", 3600, nil, oneTrack(3600)), nil)

	_, err := NewChapters(api).Rename(t.Context(), "item-1", []string{"Opening"})

	require.ErrorIs(t, err, ErrNoChapters)
}

func TestChapters_Rename_BlankTitle(t *testing.T) {
	api := NewMockChaptersAPI(t)
	api.EXPECT().Item(mock.Anything, "item-1").Return(duneWithChapters(), nil)

	_, err := NewChapters(api).Rename(t.Context(), "item-1", []string{"Opening", " "})

	chErr, ok := errors.AsType[*ChapterError](err)
	require.True(t, ok, "want *ChapterError, got %v", err)
	assert.Equal(t, 2, chErr.Chapter)
}

func TestChapters_Rename_NoAudio(t *testing.T) {
	api := NewMockChaptersAPI(t)
	api.EXPECT().Item(mock.Anything, "item-1").Return(bookWithChapters("item-1", "Dune", "Frank Herbert", "", 3600,
		[]abs.Chapter{{ID: 0, Start: 0, End: 3600, Title: "Chapter 1"}}, nil), nil)

	_, err := NewChapters(api).Rename(t.Context(), "item-1", []string{"Opening"})

	require.ErrorIs(t, err, ErrNoAudio)
}
