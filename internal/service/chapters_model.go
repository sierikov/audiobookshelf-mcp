package service

import (
	"errors"
	"fmt"
	"time"
)

// Chapter is one chapter on a book's timeline, which runs across all of the
// book's audio files back to back.
type Chapter struct {
	Start time.Duration
	End   time.Duration
	Title string
}

// ChapterInput is a chapter as a caller proposes it: where it starts and its
// title. Its end is the next chapter's start, or the end of the book.
type ChapterInput struct {
	Start time.Duration
	Title string
}

// AudioFile is one of a book's audio files, placed on the book's timeline.
// Files are rarely chapters; they are context for building chapters.
type AudioFile struct {
	Index    int
	Filename string
	Start    time.Duration
	Duration time.Duration
}

// BookChapters is a book's chapters and audio files.
type BookChapters struct {
	ID       string
	Title    string
	Author   string
	ASIN     string
	Duration time.Duration
	Chapters []Chapter
	Files    []AudioFile
}

// AudibleMatch is an Audible edition that may match a book. RuntimeDiff is
// Runtime minus the book's duration. MatchConfidence is Audiobookshelf's
// 0–1 score, nil when it gives none.
type AudibleMatch struct {
	ASIN            string
	Title           string
	Subtitle        string
	Author          string
	Narrator        string
	Language        string
	Abridged        bool
	Runtime         time.Duration
	RuntimeDiff     time.Duration
	MatchConfidence *float64
}

// AudibleSearch is the result of looking a book up on Audible: the book,
// the region searched, and the candidate editions, closest runtime first.
type AudibleSearch struct {
	Book    Item
	Region  string
	Matches []AudibleMatch
}

// AudibleChapters is Audible's chapter list for one edition. BrandIntro and
// BrandOutro are the lengths of Audible's spoken branding at either end.
type AudibleChapters struct {
	ASIN       string
	Region     string
	Runtime    time.Duration
	BrandIntro time.Duration
	BrandOutro time.Duration
	Accurate   bool
	Chapters   []Chapter
}

// ChapterChange is the outcome of saving a book's chapters: whether
// Audiobookshelf changed anything, the chapters replaced, and those saved.
type ChapterChange struct {
	Updated  bool
	Previous []Chapter
	Saved    []Chapter
}

// DefaultAudibleRegion is the Audible store searched when none is given.
const DefaultAudibleRegion = "us"

// AudibleRegions returns the Audible store regions Audiobookshelf can search.
func AudibleRegions() []string {
	return []string{"us", "ca", "uk", "au", "fr", "de", "jp", "it", "in", "es"}
}

// Errors returned by the Chapters service. They are wrapped with details, so
// match them with [errors.Is].
var (
	ErrNotABook      = errors.New("only books have chapters")
	ErrNoAudio       = errors.New("the book has no audio files")
	ErrNoChapters    = errors.New("the book has no chapters to rename")
	ErrTitleCount    = errors.New("need one title per chapter")
	ErrUnknownRegion = errors.New("unknown Audible region")
)

// ChapterError reports a chapter that fails the checks before saving.
// Chapter is the 1-based position in the list.
type ChapterError struct {
	Chapter int
	Reason  string
}

func (e *ChapterError) Error() string {
	return fmt.Sprintf("chapter %d %s", e.Chapter, e.Reason)
}
