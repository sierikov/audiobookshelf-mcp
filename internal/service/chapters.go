package service

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
)

// Chapters reads a book's chapters, looks chapters up on Audible, and saves
// new chapters after checking they make sense.
type Chapters struct {
	api ChaptersAPI
}

// NewChapters creates a Chapters service backed by api.
func NewChapters(api ChaptersAPI) *Chapters {
	return &Chapters{api: api}
}

// Get returns a book's chapters and audio files. Only books have chapters;
// any other item is an [ErrNotABook] error.
func (s *Chapters) Get(ctx context.Context, itemID string) (BookChapters, error) {
	b, err := s.book(ctx, itemID)
	if err != nil {
		return BookChapters{}, err
	}
	return BookChapters{
		ID:       itemID,
		Title:    b.Metadata.Title,
		Author:   b.Metadata.AuthorName,
		ASIN:     b.Metadata.ASIN,
		Duration: exactSeconds(b.Duration),
		Chapters: toChapters(b.Chapters),
		Files:    toAudioFiles(b.Tracks),
	}, nil
}

// SearchAudible looks a book up on Audible in a region ("" means
// [DefaultAudibleRegion]). Title and author default to the book's own.
// Candidates come closest runtime first, as Audiobookshelf sorts them.
func (s *Chapters) SearchAudible(ctx context.Context, itemID, region, title, author string) (AudibleSearch, error) {
	provider, region, err := audibleProvider(region)
	if err != nil {
		return AudibleSearch{}, err
	}
	b, err := s.book(ctx, itemID)
	if err != nil {
		return AudibleSearch{}, err
	}
	m := b.Metadata
	matches, err := s.api.SearchBooks(ctx, provider, cmp.Or(title, m.Title), cmp.Or(author, m.AuthorName), itemID)
	if err != nil {
		return AudibleSearch{}, err
	}
	duration := exactSeconds(b.Duration)
	return AudibleSearch{
		Book:    Item{ID: itemID, MediaType: abs.MediaTypeBook, Title: m.Title, Author: m.AuthorName, Duration: duration, HasDuration: true},
		Region:  region,
		Matches: toAudibleMatches(matches, duration),
	}, nil
}

// AudibleChapters fetches Audible's chapters for an edition by ASIN in a
// region ("" means [DefaultAudibleRegion]).
func (s *Chapters) AudibleChapters(ctx context.Context, asin, region string) (AudibleChapters, error) {
	_, region, err := audibleProvider(region)
	if err != nil {
		return AudibleChapters{}, err
	}
	ac, err := s.api.AudibleChapters(ctx, strings.TrimSpace(asin), region)
	if err != nil {
		return AudibleChapters{}, err
	}
	chapters := make([]Chapter, 0, len(ac.Chapters))
	for _, c := range ac.Chapters {
		start := millis(c.StartOffsetMs)
		chapters = append(chapters, Chapter{Start: start, End: start + millis(c.LengthMs), Title: c.Title})
	}
	return AudibleChapters{
		ASIN:       ac.ASIN,
		Region:     cmp.Or(ac.Region, region),
		Runtime:    millis(ac.RuntimeLengthMs),
		BrandIntro: millis(ac.BrandIntroDurationMs),
		BrandOutro: millis(ac.BrandOutroDurationMs),
		Accurate:   ac.IsAccurate,
		Chapters:   chapters,
	}, nil
}

// Set replaces all of a book's chapters. The list is checked first (see
// [ChapterError]); each chapter ends where the next starts and the last ends
// with the book. An empty list removes all chapters. The result carries the
// chapters that were replaced, so the change can be undone.
func (s *Chapters) Set(ctx context.Context, itemID string, chapters []ChapterInput) (ChapterChange, error) {
	b, err := s.writableBook(ctx, itemID)
	if err != nil {
		return ChapterChange{}, err
	}
	saved, err := buildChapters(chapters, exactSeconds(b.Duration))
	if err != nil {
		return ChapterChange{}, err
	}
	updated, err := s.api.SaveChapters(ctx, itemID, toABSChapters(saved))
	if err != nil {
		return ChapterChange{}, err
	}
	return ChapterChange{Updated: updated, Previous: toChapters(b.Chapters), Saved: saved}, nil
}

// Rename gives a book's chapters new titles, one per existing chapter in
// order. Times are sent back exactly as they are, unchecked.
func (s *Chapters) Rename(ctx context.Context, itemID string, titles []string) (ChapterChange, error) {
	b, err := s.writableBook(ctx, itemID)
	if err != nil {
		return ChapterChange{}, err
	}
	if len(b.Chapters) == 0 {
		return ChapterChange{}, fmt.Errorf("item %s: %w", itemID, ErrNoChapters)
	}
	if len(titles) != len(b.Chapters) {
		return ChapterChange{}, fmt.Errorf("%w: got %d titles for %d chapters", ErrTitleCount, len(titles), len(b.Chapters))
	}
	titles, err = checkTitles(titles)
	if err != nil {
		return ChapterChange{}, err
	}
	renamed := slices.Clone(b.Chapters)
	for i := range renamed {
		renamed[i].Title = titles[i]
	}
	updated, err := s.api.SaveChapters(ctx, itemID, renamed)
	if err != nil {
		return ChapterChange{}, err
	}
	return ChapterChange{Updated: updated, Previous: toChapters(b.Chapters), Saved: toChapters(renamed)}, nil
}

// book fetches an item and decodes it as a book.
func (s *Chapters) book(ctx context.Context, itemID string) (*abs.BookMedia, error) {
	li, err := s.api.Item(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if li.MediaType != abs.MediaTypeBook {
		return nil, fmt.Errorf("item %s is a %s: %w", itemID, li.MediaType, ErrNotABook)
	}
	b, err := li.AsBook()
	if err != nil {
		return nil, &MediaError{ItemID: li.ID, MediaType: li.MediaType, Err: err}
	}
	return b, nil
}

// writableBook fetches a book whose chapters can be saved: Audiobookshelf
// refuses, with a bare server error, to save chapters on a book without audio.
func (s *Chapters) writableBook(ctx context.Context, itemID string) (*abs.BookMedia, error) {
	b, err := s.book(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if len(b.Tracks) == 0 {
		return nil, fmt.Errorf("item %s: %w", itemID, ErrNoAudio)
	}
	return b, nil
}

func millis(ms int64) time.Duration {
	return time.Duration(ms) * time.Millisecond
}

// minutes converts Audible's runtime in minutes to a duration.
func minutes(m float64) time.Duration {
	return time.Duration(math.Round(m * float64(time.Minute)))
}

func toChapters(cs []abs.Chapter) []Chapter {
	out := make([]Chapter, 0, len(cs))
	for _, c := range cs {
		out = append(out, Chapter{Start: exactSeconds(c.Start), End: exactSeconds(c.End), Title: c.Title})
	}
	return out
}

func toABSChapters(cs []Chapter) []abs.Chapter {
	out := make([]abs.Chapter, 0, len(cs))
	for i, c := range cs {
		out = append(out, abs.Chapter{ID: i, Start: c.Start.Seconds(), End: c.End.Seconds(), Title: c.Title})
	}
	return out
}

func toAudioFiles(ts []abs.AudioTrack) []AudioFile {
	out := make([]AudioFile, 0, len(ts))
	for _, t := range ts {
		out = append(out, AudioFile{Index: t.Index, Filename: t.Title, Start: exactSeconds(t.StartOffset), Duration: exactSeconds(t.Duration)})
	}
	return out
}

func toAudibleMatches(ms []abs.BookMatch, bookDuration time.Duration) []AudibleMatch {
	out := make([]AudibleMatch, 0, len(ms))
	for _, m := range ms {
		runtime := minutes(m.Duration)
		out = append(out, AudibleMatch{
			ASIN: m.ASIN, Title: m.Title, Subtitle: m.Subtitle,
			Author: m.Author, Narrator: m.Narrator, Language: m.Language,
			Abridged: m.Abridged, Runtime: runtime, RuntimeDiff: runtime - bookDuration,
			MatchConfidence: m.MatchConfidence,
		})
	}
	return out
}
