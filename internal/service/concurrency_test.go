package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
)

// gate holds each caller until a second one arrives, or a short time has
// passed, so two unserialized writes both read before either one saves.
type gate struct {
	mu      sync.Mutex
	arrived int
	open    chan struct{}
}

func newGate() *gate { return &gate{open: make(chan struct{})} }

func (g *gate) wait() {
	g.mu.Lock()
	g.arrived++
	if g.arrived == 2 {
		close(g.open)
	}
	g.mu.Unlock()
	select {
	case <-g.open:
	case <-time.After(200 * time.Millisecond):
	}
}

var errNotFaked = errors.New("not faked")

// fakeLibrary is an in-memory Audiobookshelf holding one book's genres. It
// replaces whole lists on save, as Audiobookshelf does.
type fakeLibrary struct {
	mu     sync.Mutex
	genres []string
	gate   *gate
}

func (f *fakeLibrary) ItemsByID(_ context.Context, _ []string) ([]abs.LibraryItem, error) {
	f.mu.Lock()
	snapshot := append([]string{}, f.genres...)
	f.mu.Unlock()
	f.gate.wait()
	return []abs.LibraryItem{libraryItem("li_1", abs.MediaTypeBook, abs.BookMedia{
		Metadata: abs.BookMetadata{Title: "Солярис", Genres: snapshot},
	})}, nil
}

func (f *fakeLibrary) UpdateItems(_ context.Context, updates []abs.ItemUpdate) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range updates {
		if m := u.MediaPayload.Metadata; m != nil && m.Genres != nil {
			f.genres = append([]string{}, *m.Genres...)
		}
	}
	return len(updates), nil
}

func (f *fakeLibrary) LibraryItems(context.Context, string, abs.Page, string) (abs.ItemsPage, error) {
	return abs.ItemsPage{}, errNotFaked
}

func (f *fakeLibrary) Item(context.Context, string) (abs.LibraryItem, error) {
	return abs.LibraryItem{}, errNotFaked
}

func (f *fakeLibrary) SearchBooks(context.Context, string, string, string, string) ([]abs.BookMatch, error) {
	return nil, errNotFaked
}

func TestMetadata_AddGenres_ConcurrentCallsKeepBoth(t *testing.T) {
	lib := &fakeLibrary{genres: []string{"Научная фантастика"}, gate: newGate()}
	svc := NewMetadata(lib)

	var wg sync.WaitGroup
	for _, genre := range []string{"Science Fiction", "Classics"} {
		wg.Go(func() {
			_, err := svc.AddGenres(t.Context(), []string{"li_1"}, []string{genre})
			assert.NoError(t, err)
		})
	}
	wg.Wait()

	assert.ElementsMatch(t, []string{"Научная фантастика", "Science Fiction", "Classics"}, lib.genres)
}

// fakeChapterStore is an in-memory Audiobookshelf holding one book's
// chapters. Saving replaces them all, as Audiobookshelf does.
type fakeChapterStore struct {
	mu       sync.Mutex
	chapters []abs.Chapter
	gate     *gate
}

func (f *fakeChapterStore) Item(_ context.Context, id string) (abs.LibraryItem, error) {
	f.mu.Lock()
	snapshot := append([]abs.Chapter{}, f.chapters...)
	f.mu.Unlock()
	f.gate.wait()
	return bookWithChapters(id, "Katabasis", "R. F. Kuang", "", 3600, snapshot, oneTrack(3600)), nil
}

func (f *fakeChapterStore) SaveChapters(_ context.Context, _ string, chapters []abs.Chapter) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.chapters = append([]abs.Chapter{}, chapters...)
	return true, nil
}

func (f *fakeChapterStore) SearchBooks(context.Context, string, string, string, string) ([]abs.BookMatch, error) {
	return nil, errNotFaked
}

func (f *fakeChapterStore) AudibleChapters(context.Context, string, string) (abs.AudibleChapters, error) {
	return abs.AudibleChapters{}, errNotFaked
}

func TestChapters_SetAndRename_ConcurrentCallsKeepNewTimes(t *testing.T) {
	store := &fakeChapterStore{
		chapters: []abs.Chapter{{ID: 0, Start: 0, End: 600, Title: "Chapter 1"}, {ID: 1, Start: 600, End: 3600, Title: "Chapter 2"}},
		gate:     newGate(),
	}
	svc := NewChapters(store)

	var wg sync.WaitGroup
	wg.Go(func() {
		_, err := svc.Set(t.Context(), "li_1", []ChapterInput{{Start: 0, Title: "Opening"}, {Start: 1200 * time.Second, Title: "One"}})
		assert.NoError(t, err)
	})
	wg.Go(func() {
		_, err := svc.Rename(t.Context(), "li_1", []string{"Prologue", "Part One"})
		assert.NoError(t, err)
	})
	wg.Wait()

	// Whichever call goes first, set_chapters' new times must survive:
	// a rename that read the chapters before the set must not write the old
	// times back.
	require.Len(t, store.chapters, 2)
	assert.InDelta(t, 1200.0, store.chapters[1].Start, 1e-9)
}
