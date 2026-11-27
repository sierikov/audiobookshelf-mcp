package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
)

// listBook is a book with the given genres and tags and nothing else set.
func listBook(id, title string, genres, tags []string) abs.LibraryItem {
	return metaBook(id, title, "", "", "", "", genres, tags)
}

// nullBook is a book Audiobookshelf stores with null genres, tags, language
// and description.
func nullBook(id, title string) abs.LibraryItem {
	return abs.LibraryItem{ID: id, MediaType: abs.MediaTypeBook, Media: json.RawMessage(
		`{"metadata":{"title":"` + title + `","genres":null,"language":null,"description":null},"tags":null}`)}
}

// recordUpdates expects one UpdateItems call, answers it with n and returns
// what it was sent.
func recordUpdates(api *MockMetadataAPI, n int) *[]abs.ItemUpdate {
	sent := new([]abs.ItemUpdate)
	api.EXPECT().UpdateItems(mock.Anything, mock.Anything).
		Run(func(_ context.Context, u []abs.ItemUpdate) { *sent = u }).
		Return(n, nil).Once()
	return sent
}

// assertSentJSON checks the request body Audiobookshelf would receive.
func assertSentJSON(t *testing.T, want string, sent []abs.ItemUpdate) {
	t.Helper()
	got, err := json.Marshal(sent)
	require.NoError(t, err)
	assert.JSONEq(t, want, string(got))
}

// listWrites are the four list methods, for what holds for all of them.
var listWrites = []struct {
	name string
	call func(*Metadata, context.Context, []string, []string) (ListChangeResult, error)
}{
	{"AddGenres", (*Metadata).AddGenres},
	{"RemoveGenres", (*Metadata).RemoveGenres},
	{"AddTags", (*Metadata).AddTags},
	{"RemoveTags", (*Metadata).RemoveTags},
}

func TestAddGenres(t *testing.T) {
	api := NewMockMetadataAPI(t)
	api.EXPECT().ItemsByID(mock.Anything, []string{"li_1", "li_2", "li_3"}).Return([]abs.LibraryItem{
		listBook("li_1", "Solaris", []string{"Научная фантастика"}, []string{"classic"}),
		listBook("li_2", "Dune", []string{"Science Fiction"}, nil),
		nullBook("li_3", "Untitled"),
	}, nil)
	sent := recordUpdates(api, 2)

	// Ids and genres are used trimmed.
	got, err := NewMetadata(api).AddGenres(t.Context(), []string{"li_1", " li_2", "li_3 "}, []string{" Science Fiction "})

	require.NoError(t, err)
	assert.Equal(t, []abs.ItemUpdate{
		{ID: "li_1", MediaPayload: abs.MediaPayload{Metadata: &abs.MetadataPayload{Genres: &[]string{"Научная фантастика", "Science Fiction"}}}},
		{ID: "li_3", MediaPayload: abs.MediaPayload{Metadata: &abs.MetadataPayload{Genres: &[]string{"Science Fiction"}}}},
	}, *sent)
	assertSentJSON(t, `[
		{"id":"li_1","mediaPayload":{"metadata":{"genres":["Научная фантастика","Science Fiction"]}}},
		{"id":"li_3","mediaPayload":{"metadata":{"genres":["Science Fiction"]}}}
	]`, *sent)
	assert.Equal(t, ListChangeResult{Changed: 2, Items: []ListChange{
		{
			ItemID: "li_1", Title: "Solaris", Previous: []string{"Научная фантастика"},
			Now: []string{"Научная фантастика", "Science Fiction"}, Values: []string{"Science Fiction"},
		},
		{ItemID: "li_2", Title: "Dune", Previous: []string{"Science Fiction"}, Now: []string{"Science Fiction"}, Values: []string{}},
		{ItemID: "li_3", Title: "Untitled", Previous: []string{}, Now: []string{"Science Fiction"}, Values: []string{"Science Fiction"}},
	}}, got)
}

func TestAddGenres_PartialOverlap(t *testing.T) {
	api := NewMockMetadataAPI(t)
	api.EXPECT().ItemsByID(mock.Anything, []string{"li_1", "li_2"}).Return([]abs.LibraryItem{
		listBook("li_1", "Solaris", []string{"Thriller"}, nil),
		listBook("li_2", "Dune", []string{"Horror", "Thriller"}, nil),
	}, nil)
	sent := recordUpdates(api, 1)

	got, err := NewMetadata(api).AddGenres(t.Context(), []string{"li_1", "li_2"}, []string{"Thriller", "Horror"})

	require.NoError(t, err)
	assertSentJSON(t, `[{"id":"li_1","mediaPayload":{"metadata":{"genres":["Thriller","Horror"]}}}]`, *sent)
	// Values is only what this call added, so undoing it with remove_genres
	// keeps the Thriller that li_1 had before.
	assert.Equal(t, ListChangeResult{Changed: 1, Items: []ListChange{
		{ItemID: "li_1", Title: "Solaris", Previous: []string{"Thriller"}, Now: []string{"Thriller", "Horror"}, Values: []string{"Horror"}},
		{ItemID: "li_2", Title: "Dune", Previous: []string{"Horror", "Thriller"}, Now: []string{"Horror", "Thriller"}, Values: []string{}},
	}}, got)
}

func TestAddGenres_CaseConflict(t *testing.T) {
	tests := []struct {
		name   string
		ids    []string
		books  []abs.LibraryItem
		genres []string
		want   string
	}{
		{
			name:   "first book",
			ids:    []string{"li_1"},
			books:  []abs.LibraryItem{listBook("li_1", "Solaris", []string{"fiction"}, nil)},
			genres: []string{"Fiction"},
			want:   `item 1 (li_1): already has "fiction"; remove it before adding "Fiction"`,
		},
		{
			name: "after a book that would change",
			ids:  []string{"li_2", "li_1"},
			books: []abs.LibraryItem{
				nullBook("li_2", "Untitled"),
				listBook("li_1", "Solaris", []string{"fiction"}, nil),
			},
			genres: []string{"Horror", "Fiction"},
			want:   `item 2 (li_1): already has "fiction"; remove it before adding "Fiction"`,
		},
		{
			name:   "stored with surrounding spaces",
			ids:    []string{"li_1"},
			books:  []abs.LibraryItem{listBook("li_1", "Solaris", []string{"fiction "}, nil)},
			genres: []string{"Fiction"},
			want:   `item 1 (li_1): already has "fiction "; remove it before adding "Fiction"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := NewMockMetadataAPI(t)
			api.EXPECT().ItemsByID(mock.Anything, tt.ids).Return(tt.books, nil)

			_, err := NewMetadata(api).AddGenres(t.Context(), tt.ids, tt.genres)

			require.EqualError(t, err, tt.want)
			_, ok := errors.AsType[*ItemError](err)
			assert.True(t, ok, "want *ItemError, got %T", err)
			api.AssertNotCalled(t, "UpdateItems", mock.Anything, mock.Anything)
		})
	}
}

func TestRemoveGenres_LastValueSendsEmptyList(t *testing.T) {
	api := NewMockMetadataAPI(t)
	api.EXPECT().ItemsByID(mock.Anything, []string{"li_1"}).
		Return([]abs.LibraryItem{listBook("li_1", "Songs", []string{"Vocal"}, []string{"Vocal"})}, nil)
	sent := recordUpdates(api, 1)

	got, err := NewMetadata(api).RemoveGenres(t.Context(), []string{"li_1"}, []string{"Vocal"})

	require.NoError(t, err)
	assert.Equal(t, []abs.ItemUpdate{
		{ID: "li_1", MediaPayload: abs.MediaPayload{Metadata: &abs.MetadataPayload{Genres: &[]string{}}}},
	}, *sent)
	assertSentJSON(t, `[{"id":"li_1","mediaPayload":{"metadata":{"genres":[]}}}]`, *sent)
	assert.Equal(t, ListChangeResult{Changed: 1, Items: []ListChange{
		{ItemID: "li_1", Title: "Songs", Previous: []string{"Vocal"}, Now: []string{}, Values: []string{"Vocal"}},
	}}, got)
}

func TestRemoveGenres_PartialOverlap(t *testing.T) {
	api := NewMockMetadataAPI(t)
	api.EXPECT().ItemsByID(mock.Anything, []string{"li_1", "li_2"}).Return([]abs.LibraryItem{
		listBook("li_1", "Solaris", []string{"Mystery"}, nil),
		listBook("li_2", "Dune", []string{"Suspense", "Drama", "Mystery"}, nil),
	}, nil)
	sent := recordUpdates(api, 2)

	got, err := NewMetadata(api).RemoveGenres(t.Context(), []string{"li_1", "li_2"}, []string{"Mystery", "Suspense"})

	require.NoError(t, err)
	assertSentJSON(t, `[
		{"id":"li_1","mediaPayload":{"metadata":{"genres":[]}}},
		{"id":"li_2","mediaPayload":{"metadata":{"genres":["Drama"]}}}
	]`, *sent)
	// Values is only what each book lost, in the order asked, so undoing it
	// with add_genres does not give li_1 a Suspense it never had.
	assert.Equal(t, ListChangeResult{Changed: 2, Items: []ListChange{
		{ItemID: "li_1", Title: "Solaris", Previous: []string{"Mystery"}, Now: []string{}, Values: []string{"Mystery"}},
		{
			ItemID: "li_2", Title: "Dune", Previous: []string{"Suspense", "Drama", "Mystery"},
			Now: []string{"Drama"}, Values: []string{"Mystery", "Suspense"},
		},
	}}, got)
}

func TestRemoveGenres_NothingToRemove(t *testing.T) {
	api := NewMockMetadataAPI(t)
	api.EXPECT().ItemsByID(mock.Anything, []string{"li_1", "li_2"}).Return([]abs.LibraryItem{
		listBook("li_1", "Solaris", []string{"Fiction"}, nil),
		nullBook("li_2", "Untitled"),
	}, nil)

	got, err := NewMetadata(api).RemoveGenres(t.Context(), []string{"li_1", "li_2"}, []string{"Vocal"})

	require.NoError(t, err)
	assert.Equal(t, ListChangeResult{Items: []ListChange{
		{ItemID: "li_1", Title: "Solaris", Previous: []string{"Fiction"}, Now: []string{"Fiction"}, Values: []string{}},
		{ItemID: "li_2", Title: "Untitled", Previous: []string{}, Now: []string{}, Values: []string{}},
	}}, got)
	api.AssertNotCalled(t, "UpdateItems", mock.Anything, mock.Anything)
}

func TestAddTags(t *testing.T) {
	api := NewMockMetadataAPI(t)
	api.EXPECT().ItemsByID(mock.Anything, []string{"li_1", "li_2"}).Return([]abs.LibraryItem{
		listBook("li_1", "Solaris", []string{"Fiction"}, []string{"classic"}),
		nullBook("li_2", "Untitled"),
	}, nil)
	sent := recordUpdates(api, 2)

	got, err := NewMetadata(api).AddTags(t.Context(), []string{"li_1", "li_2"}, []string{"favourite"})

	require.NoError(t, err)
	assert.Equal(t, []abs.ItemUpdate{
		{ID: "li_1", MediaPayload: abs.MediaPayload{Tags: &[]string{"classic", "favourite"}}},
		{ID: "li_2", MediaPayload: abs.MediaPayload{Tags: &[]string{"favourite"}}},
	}, *sent)
	assertSentJSON(t, `[
		{"id":"li_1","mediaPayload":{"tags":["classic","favourite"]}},
		{"id":"li_2","mediaPayload":{"tags":["favourite"]}}
	]`, *sent)
	assert.Equal(t, ListChangeResult{Changed: 2, Items: []ListChange{
		{ItemID: "li_1", Title: "Solaris", Previous: []string{"classic"}, Now: []string{"classic", "favourite"}, Values: []string{"favourite"}},
		{ItemID: "li_2", Title: "Untitled", Previous: []string{}, Now: []string{"favourite"}, Values: []string{"favourite"}},
	}}, got)
}

func TestRemoveTags(t *testing.T) {
	api := NewMockMetadataAPI(t)
	api.EXPECT().ItemsByID(mock.Anything, []string{"li_1"}).
		Return([]abs.LibraryItem{listBook("li_1", "Songs", []string{"Vocal"}, []string{"Vocal"})}, nil)
	sent := recordUpdates(api, 1)

	got, err := NewMetadata(api).RemoveTags(t.Context(), []string{"li_1"}, []string{"Vocal"})

	require.NoError(t, err)
	assert.Equal(t, []abs.ItemUpdate{{ID: "li_1", MediaPayload: abs.MediaPayload{Tags: &[]string{}}}}, *sent)
	assertSentJSON(t, `[{"id":"li_1","mediaPayload":{"tags":[]}}]`, *sent)
	assert.Equal(t, ListChangeResult{Changed: 1, Items: []ListChange{
		{ItemID: "li_1", Title: "Songs", Previous: []string{"Vocal"}, Now: []string{}, Values: []string{"Vocal"}},
	}}, got)
}

func TestListWrites_CheckErrors(t *testing.T) {
	tooMany := make([]string, MaxItemsPerCall+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("li_%d", i)
	}
	tests := []struct {
		name        string
		ids, values []string
		target      error
		want        string
	}{
		{"no ids", nil, []string{"Fiction"}, ErrNoItems, "no items given"},
		{"101 ids", tooMany, []string{"Fiction"}, ErrTooManyItems, "too many items: 101, at most 100 per call"},
		{"duplicate id", []string{"li_1", " li_1"}, []string{"Fiction"}, nil, "item 2 (li_1): is listed twice"},
		{"no values", []string{"li_1"}, nil, ErrNoValues, "no values given"},
		{"empty value", []string{"li_1"}, []string{" "}, nil, "value 1 is empty"},
		{"case-duplicate values", []string{"li_1"}, []string{"Fiction", "fiction"}, nil,
			`value 2 differs only in case from value 1 ("Fiction")`},
	}
	for _, w := range listWrites {
		t.Run(w.name, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					api := NewMockMetadataAPI(t)

					_, err := w.call(NewMetadata(api), t.Context(), tt.ids, tt.values)

					require.EqualError(t, err, tt.want)
					if tt.target != nil {
						require.ErrorIs(t, err, tt.target)
					}
					api.AssertNotCalled(t, "ItemsByID", mock.Anything, mock.Anything)
					api.AssertNotCalled(t, "UpdateItems", mock.Anything, mock.Anything)
				})
			}
		})
	}
}

func TestListWrites_UnknownAndPodcast(t *testing.T) {
	tests := []struct {
		name   string
		ids    []string
		found  []abs.LibraryItem
		target error
		text   string
	}{
		{"unknown id", []string{"li_1", "li_9"}, []abs.LibraryItem{listBook("li_1", "Solaris", nil, nil)}, ErrUnknownItems, "li_9"},
		{"podcast", []string{"pod_1"}, []abs.LibraryItem{podcastItem("pod_1", "Show", "Host", 3)}, ErrBooksOnly, "pod_1"},
	}
	for _, w := range listWrites {
		t.Run(w.name, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					api := NewMockMetadataAPI(t)
					api.EXPECT().ItemsByID(mock.Anything, tt.ids).Return(tt.found, nil)

					_, err := w.call(NewMetadata(api), t.Context(), tt.ids, []string{"Fiction"})

					require.ErrorIs(t, err, tt.target)
					require.ErrorContains(t, err, tt.text)
					api.AssertNotCalled(t, "UpdateItems", mock.Anything, mock.Anything)
				})
			}
		})
	}
}

func TestListWrites_UndecodableBook(t *testing.T) {
	for _, w := range listWrites {
		t.Run(w.name, func(t *testing.T) {
			api := NewMockMetadataAPI(t)
			api.EXPECT().ItemsByID(mock.Anything, []string{"li_1"}).
				Return([]abs.LibraryItem{brokenItem("li_1", abs.MediaTypeBook)}, nil)

			_, err := w.call(NewMetadata(api), t.Context(), []string{"li_1"}, []string{"Fiction"})

			me, ok := errors.AsType[*MediaError](err)
			require.True(t, ok, "want *MediaError, got %v", err)
			assert.Equal(t, "li_1", me.ItemID)
			api.AssertNotCalled(t, "UpdateItems", mock.Anything, mock.Anything)
		})
	}
}

func TestListWrites_WriteFails(t *testing.T) {
	// Whether it adds or removes "Fiction", each method changes one of the two
	// books, so it calls UpdateItems.
	for _, w := range listWrites {
		t.Run(w.name, func(t *testing.T) {
			api := NewMockMetadataAPI(t)
			api.EXPECT().ItemsByID(mock.Anything, []string{"li_1", "li_2"}).Return([]abs.LibraryItem{
				listBook("li_1", "Solaris", []string{"Fiction"}, []string{"Fiction"}),
				nullBook("li_2", "Untitled"),
			}, nil)
			api.EXPECT().UpdateItems(mock.Anything, mock.Anything).Return(0, errAPI).Once()

			got, err := w.call(NewMetadata(api), t.Context(), []string{"li_1", "li_2"}, []string{"Fiction"})

			require.ErrorIs(t, err, ErrWriteIncomplete)
			require.ErrorIs(t, err, errAPI)
			require.EqualError(t, err, "the write failed; some books may already be saved: ABS API 500: boom")
			// Now and Values are nil: what was saved is unknown.
			assert.Equal(t, ListChangeResult{Items: []ListChange{
				{ItemID: "li_1", Title: "Solaris", Previous: []string{"Fiction"}},
				{ItemID: "li_2", Title: "Untitled", Previous: []string{}},
			}}, got)
		})
	}
}

func TestUpdate(t *testing.T) {
	ids := []string{"li_1", "li_2", "li_3"}
	api := NewMockMetadataAPI(t)
	api.EXPECT().ItemsByID(mock.Anything, ids).Return([]abs.LibraryItem{
		nullBook("li_1", "Solaris"),
		metaBook("li_2", "Пикник на обочине", "", "", "Ukrainian", "Князев Игорь", nil, nil),
		metaBook("li_3", "Dune", "", "", "English", "<p>Spice</p>", nil, nil),
	}, nil).Once()
	api.EXPECT().ItemsByID(mock.Anything, ids).Return([]abs.LibraryItem{
		metaBook("li_1", "Solaris", "", "", "Russian", "", nil, nil),
		metaBook("li_2", "Пикник на обочине", "", "", "Ukrainian", "<p>Blurb</p>", nil, nil),
		metaBook("li_3", "Dune", "", "", "English", "<p>Spice</p>", nil, nil),
	}, nil).Once()
	// li_3 is already English, so Audiobookshelf counts two changed books.
	sent := recordUpdates(api, 2)
	changes := []ItemChange{
		{ItemID: "li_1", Language: new("Russian")},
		{ItemID: "li_2", Description: new(" <p>Blurb</p> ")},
		{ItemID: "li_3", Language: new(" English ")},
	}

	got, err := NewMetadata(api).Update(t.Context(), changes)

	require.NoError(t, err)
	assert.Equal(t, []abs.ItemUpdate{
		{ID: "li_1", MediaPayload: abs.MediaPayload{Metadata: &abs.MetadataPayload{Language: new("Russian")}}},
		{ID: "li_2", MediaPayload: abs.MediaPayload{Metadata: &abs.MetadataPayload{Description: new("<p>Blurb</p>")}}},
		{ID: "li_3", MediaPayload: abs.MediaPayload{Metadata: &abs.MetadataPayload{Language: new("English")}}},
	}, *sent)
	assertSentJSON(t, `[
		{"id":"li_1","mediaPayload":{"metadata":{"language":"Russian"}}},
		{"id":"li_2","mediaPayload":{"metadata":{"description":"<p>Blurb</p>"}}},
		{"id":"li_3","mediaPayload":{"metadata":{"language":"English"}}}
	]`, *sent)
	assert.Equal(t, UpdateResult{Changed: 2, Items: []ItemUpdateResult{
		{
			ItemID: "li_1", Title: "Solaris",
			Previous: ItemFields{Language: new("")}, Now: ItemFields{Language: new("Russian")},
		},
		{
			ItemID: "li_2", Title: "Пикник на обочине",
			Previous: ItemFields{Description: new("Князев Игорь")}, Now: ItemFields{Description: new("<p>Blurb</p>")},
		},
		{
			ItemID: "li_3", Title: "Dune",
			Previous: ItemFields{Language: new("English")}, Now: ItemFields{Language: new("English")},
		},
	}}, got)
	assert.Equal(t, " <p>Blurb</p> ", *changes[1].Description, "the caller's change is left as it was")
}

func TestUpdate_CheckErrors(t *testing.T) {
	tooMany := make([]ItemChange, MaxItemsPerCall+1)
	for i := range tooMany {
		tooMany[i] = ItemChange{ItemID: fmt.Sprintf("li_%d", i), Language: new("English")}
	}
	descriptions := make([]ItemChange, MaxDescriptionsPerCall+1)
	for i := range descriptions {
		descriptions[i] = ItemChange{ItemID: fmt.Sprintf("li_%d", i), Description: new("<p>Blurb</p>")}
	}
	tests := []struct {
		name    string
		changes []ItemChange
		target  error
		want    string
	}{
		{"no changes", nil, ErrNoItems, "no items given"},
		{"101 changes", tooMany, ErrTooManyItems, "too many items: 101, at most 100 per call"},
		{"11 descriptions", descriptions, ErrTooManyDescriptions, "too many descriptions: 11, at most 10 per call"},
		{"duplicate id", []ItemChange{
			{ItemID: "li_1", Language: new("English")},
			{ItemID: " li_1 ", Language: new("Russian")},
		}, nil, "item 2 (li_1): is listed twice"},
		{"neither field", []ItemChange{
			{ItemID: "li_1", Language: new("English")},
			{ItemID: " li_2 "},
		}, nil, "item 2 (li_2): sets none of language, subtitle, description, series, authors, narrators"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := NewMockMetadataAPI(t)

			_, err := NewMetadata(api).Update(t.Context(), tt.changes)

			require.EqualError(t, err, tt.want)
			if tt.target != nil {
				require.ErrorIs(t, err, tt.target)
			}
			api.AssertNotCalled(t, "ItemsByID", mock.Anything, mock.Anything)
			api.AssertNotCalled(t, "UpdateItems", mock.Anything, mock.Anything)
		})
	}
}

func TestUpdate_TenDescriptionsAllowed(t *testing.T) {
	changes := make([]ItemChange, MaxDescriptionsPerCall)
	for i := range changes {
		changes[i] = ItemChange{ItemID: fmt.Sprintf("li_%d", i), Description: new("<p>Blurb</p>")}
	}
	api := NewMockMetadataAPI(t)
	// Reaching the read means the checks passed; failing it stops the call there.
	api.EXPECT().ItemsByID(mock.Anything, mock.Anything).Return(nil, errAPI)

	_, err := NewMetadata(api).Update(t.Context(), changes)

	assert.Equal(t, errAPI, err)
}

func TestUpdate_ClearWithEmptyString(t *testing.T) {
	api := NewMockMetadataAPI(t)
	api.EXPECT().ItemsByID(mock.Anything, []string{"li_1"}).
		Return([]abs.LibraryItem{metaBook("li_1", "Solaris", "", "", "english", "<p>Old</p>", nil, nil)}, nil).Once()
	api.EXPECT().ItemsByID(mock.Anything, []string{"li_1"}).
		Return([]abs.LibraryItem{nullBook("li_1", "Solaris")}, nil).Once()
	sent := recordUpdates(api, 1)

	// A description of only spaces is trimmed to "", which clears it too.
	got, err := NewMetadata(api).Update(t.Context(), []ItemChange{{ItemID: "li_1", Language: new(""), Description: new("  ")}})

	require.NoError(t, err)
	assert.Equal(t, []abs.ItemUpdate{
		{ID: "li_1", MediaPayload: abs.MediaPayload{Metadata: &abs.MetadataPayload{Language: new(""), Description: new("")}}},
	}, *sent)
	assertSentJSON(t, `[{"id":"li_1","mediaPayload":{"metadata":{"language":"","description":""}}}]`, *sent)
	assert.Equal(t, UpdateResult{Changed: 1, Items: []ItemUpdateResult{{
		ItemID: "li_1", Title: "Solaris",
		Previous: ItemFields{Language: new("english"), Description: new("<p>Old</p>")},
		Now:      ItemFields{Language: new(""), Description: new("")},
	}}}, got)
}

func TestUpdate_UnknownItems(t *testing.T) {
	api := NewMockMetadataAPI(t)
	api.EXPECT().ItemsByID(mock.Anything, []string{"li_1", "li_9"}).
		Return([]abs.LibraryItem{listBook("li_1", "Solaris", nil, nil)}, nil)

	_, err := NewMetadata(api).Update(t.Context(), []ItemChange{
		{ItemID: "li_1", Language: new("English")},
		{ItemID: "li_9", Language: new("English")},
	})

	require.ErrorIs(t, err, ErrUnknownItems)
	require.ErrorContains(t, err, "li_9")
	api.AssertNotCalled(t, "UpdateItems", mock.Anything, mock.Anything)
}

func TestUpdate_WriteFails(t *testing.T) {
	api := NewMockMetadataAPI(t)
	api.EXPECT().ItemsByID(mock.Anything, []string{"li_1"}).
		Return([]abs.LibraryItem{metaBook("li_1", "Solaris", "", "", "English", "<p>Old</p>", nil, nil)}, nil).Once()
	api.EXPECT().UpdateItems(mock.Anything, mock.Anything).Return(0, errAPI).Once()

	got, err := NewMetadata(api).Update(t.Context(), []ItemChange{{ItemID: "li_1", Description: new("<p>New</p>")}})

	require.ErrorIs(t, err, ErrWriteIncomplete)
	require.ErrorIs(t, err, errAPI)
	assert.Equal(t, UpdateResult{Items: []ItemUpdateResult{{
		ItemID: "li_1", Title: "Solaris", Previous: ItemFields{Description: new("<p>Old</p>")},
	}}}, got)
	api.AssertNumberOfCalls(t, "ItemsByID", 1)
}

func TestUpdate_RereadFails(t *testing.T) {
	api := NewMockMetadataAPI(t)
	api.EXPECT().ItemsByID(mock.Anything, []string{"li_1"}).
		Return([]abs.LibraryItem{metaBook("li_1", "Solaris", "", "", "English", "", nil, nil)}, nil).Once()
	api.EXPECT().ItemsByID(mock.Anything, []string{"li_1"}).Return(nil, errAPI).Once()
	recordUpdates(api, 1)

	got, err := NewMetadata(api).Update(t.Context(), []ItemChange{{ItemID: "li_1", Language: new("Russian")}})

	// The write went through, so this is a failed read-back, not an
	// incomplete write, and Previous still allows an undo.
	require.ErrorIs(t, err, ErrReadBack)
	require.ErrorIs(t, err, errAPI)
	require.NotErrorIs(t, err, ErrWriteIncomplete)
	require.EqualError(t, err, "the books were saved, but reading them back failed: ABS API 500: boom")
	assert.Equal(t, UpdateResult{Changed: 1, Items: []ItemUpdateResult{{
		ItemID: "li_1", Title: "Solaris", Previous: ItemFields{Language: new("English")},
	}}}, got)
}

// addWrites are the two list methods that add values.
var addWrites = []struct {
	name string
	call func(*Metadata, context.Context, []string, []string) (ListChangeResult, error)
}{
	{"AddGenres", (*Metadata).AddGenres},
	{"AddTags", (*Metadata).AddTags},
}

func TestAddValues_RefusesUnsafeValues(t *testing.T) {
	long := strings.Repeat("я", MaxValueLength+1)
	tests := []struct {
		name   string
		values []string
		want   string
	}{
		{"markup", []string{"Fantasy", "<b>X</b>"}, `value 2 ("<b>X</b>") contains "<"; use plain text without markup`},
		{"too long", []string{long}, fmt.Sprintf(`value 1 (%q…) is 65 characters long; at most 64`, strings.Repeat("я", 40))},
		{"bidi override", []string{"X\u202e"}, `value 1 ("X\u202e") contains the invisible character U+202E`},
	}
	for _, w := range addWrites {
		for _, tt := range tests {
			t.Run(w.name+"/"+tt.name, func(t *testing.T) {
				api := NewMockMetadataAPI(t) // any call fails the test

				_, err := w.call(NewMetadata(api), t.Context(), []string{"li_1"}, tt.values)

				var valueErr *ValueError
				require.ErrorAs(t, err, &valueErr)
				assert.EqualError(t, err, tt.want)
			})
		}
	}
}

func TestRemoveValues_AcceptAnyValue(t *testing.T) {
	api := NewMockMetadataAPI(t)
	api.EXPECT().ItemsByID(mock.Anything, []string{"li_1"}).Return([]abs.LibraryItem{
		listBook("li_1", "Solaris", []string{"<b>junk</b>", "Fantasy\u200b", "Fantasy"}, nil),
	}, nil)
	sent := recordUpdates(api, 1)

	_, err := NewMetadata(api).RemoveGenres(t.Context(), []string{"li_1"}, []string{"<b>junk</b>", "Fantasy\u200b"})

	require.NoError(t, err)
	assertSentJSON(t, `[{"id":"li_1","mediaPayload":{"metadata":{"genres":["Fantasy"]}}}]`, *sent)
}

func TestUpdate_CleansNewDescription(t *testing.T) {
	api := NewMockMetadataAPI(t)
	api.EXPECT().ItemsByID(mock.Anything, []string{"li_1"}).Return([]abs.LibraryItem{nullBook("li_1", "Solaris")}, nil).Once()
	api.EXPECT().ItemsByID(mock.Anything, []string{"li_1"}).Return([]abs.LibraryItem{
		metaBook("li_1", "Solaris", "", "", "", "<p>Blurb</p>", nil, nil),
	}, nil).Once()
	sent := recordUpdates(api, 1)

	_, err := NewMetadata(api).Update(t.Context(), []ItemChange{
		{ItemID: "li_1", Description: new(`<p onclick="x">Blurb</p><script>evil()</script>`)},
	})

	require.NoError(t, err)
	assertSentJSON(t, `[{"id":"li_1","mediaPayload":{"metadata":{"description":"<p>Blurb</p>"}}}]`, *sent)
}

// updateRefused runs an update of one book stored as stored and checks it
// is refused with want before anything is sent.
func updateRefused(t *testing.T, stored abs.LibraryItem, change ItemChange, want string) {
	t.Helper()
	api := NewMockMetadataAPI(t)
	api.EXPECT().ItemsByID(mock.Anything, []string{change.ItemID}).Return([]abs.LibraryItem{stored}, nil).Once()

	_, err := NewMetadata(api).Update(t.Context(), []ItemChange{change})

	var itemErr *ItemError
	require.ErrorAs(t, err, &itemErr)
	assert.EqualError(t, err, want)
}

func TestUpdate_RefusesUnsafeLanguage(t *testing.T) {
	updateRefused(t, metaBook("li_1", "Solaris", "", "", "English", "", nil, nil),
		ItemChange{ItemID: "li_1", Language: new("Rus\tsian")},
		`item 1 (li_1): language ("Rus\tsian") contains the control character U+0009`)
}

func TestUpdate_RefusesDescriptionWithNoTextLeft(t *testing.T) {
	const want = `item 1 (li_1): description has no text left once markup outside the allowlist is removed; send "" to clear it`
	for _, d := range []string{"<div></div>", "\u200b", "<script>x</script>", "<p><img src=x></p>", "<p>\u200e\u200d</p>", "&lrm;"} {
		t.Run(d, func(t *testing.T) {
			updateRefused(t, nullBook("li_1", "Solaris"), ItemChange{ItemID: "li_1", Description: new(d)}, want)
		})
	}
}

func TestUpdate_RefusesOverlongDescription(t *testing.T) {
	updateRefused(t, nullBook("li_1", "Solaris"),
		ItemChange{ItemID: "li_1", Description: new(strings.Repeat("a", MaxDescriptionLength+1))},
		`item 1 (li_1): description is 10001 characters long; at most 10000`)
}

// subtitleBook is a book with a subtitle and nothing else set.
func subtitleBook(id, title, subtitle string) abs.LibraryItem {
	return libraryItem(id, abs.MediaTypeBook, abs.BookMedia{Metadata: abs.BookMetadata{Title: title, Subtitle: subtitle}})
}

func TestUpdate_SetsAndRemovesSubtitles(t *testing.T) {
	ids := []string{"li_1", "li_2", "li_3"}
	api := NewMockMetadataAPI(t)
	api.EXPECT().ItemsByID(mock.Anything, ids).Return([]abs.LibraryItem{
		subtitleBook("li_1", "Solaris", ""),
		subtitleBook("li_2", "Пикник на обочине", "[роман]"),
		metaBook("li_3", "Dune", "", "", "English", "", nil, nil),
	}, nil).Once()
	api.EXPECT().ItemsByID(mock.Anything, ids).Return([]abs.LibraryItem{
		subtitleBook("li_1", "Solaris", "A Novel"),
		subtitleBook("li_2", "Пикник на обочине", ""),
		metaBook("li_3", "Dune", "", "", "Russian", "", nil, nil),
	}, nil).Once()
	sent := recordUpdates(api, 3)
	changes := []ItemChange{
		{ItemID: "li_1", Subtitle: new("  A Novel \n")},
		{ItemID: "li_2", Subtitle: new("")},
		{ItemID: "li_3", Language: new("Russian")}, // no subtitle: it is left out
	}

	got, err := NewMetadata(api).Update(t.Context(), changes)

	require.NoError(t, err)
	assert.Equal(t, []abs.ItemUpdate{
		{ID: "li_1", MediaPayload: abs.MediaPayload{Metadata: &abs.MetadataPayload{Subtitle: new("A Novel")}}},
		{ID: "li_2", MediaPayload: abs.MediaPayload{Metadata: &abs.MetadataPayload{Subtitle: new("")}}},
		{ID: "li_3", MediaPayload: abs.MediaPayload{Metadata: &abs.MetadataPayload{Language: new("Russian")}}},
	}, *sent)
	assertSentJSON(t, `[
		{"id":"li_1","mediaPayload":{"metadata":{"subtitle":"A Novel"}}},
		{"id":"li_2","mediaPayload":{"metadata":{"subtitle":""}}},
		{"id":"li_3","mediaPayload":{"metadata":{"language":"Russian"}}}
	]`, *sent)
	assert.Equal(t, UpdateResult{Changed: 3, Items: []ItemUpdateResult{
		{ItemID: "li_1", Title: "Solaris", Previous: ItemFields{Subtitle: new("")}, Now: ItemFields{Subtitle: new("A Novel")}},
		{ItemID: "li_2", Title: "Пикник на обочине", Previous: ItemFields{Subtitle: new("[роман]")}, Now: ItemFields{Subtitle: new("")}},
		{ItemID: "li_3", Title: "Dune", Previous: ItemFields{Language: new("English")}, Now: ItemFields{Language: new("Russian")}},
	}}, got, "a subtitle is reported as the other fields are, and only when it was part of the change")
	assert.Equal(t, "  A Novel \n", *changes[0].Subtitle, "the caller's change is left as it was")
}

func TestUpdate_RefusesBadSubtitles(t *testing.T) {
	tests := []struct {
		name     string
		subtitle string
		want     string
	}{
		{"201 characters", strings.Repeat("a", MaxSubtitleLength+1),
			fmt.Sprintf(`item 1 (li_1): subtitle (%q…) is 201 characters long; at most 200`, strings.Repeat("a", 40))},
		{"201 Cyrillic characters", strings.Repeat("я", MaxSubtitleLength+1),
			fmt.Sprintf(`item 1 (li_1): subtitle (%q…) is 201 characters long; at most 200`, strings.Repeat("я", 40))},
		{"markup", "<b>A Novel</b>", `item 1 (li_1): subtitle ("<b>A Novel</b>") contains "<"; use plain text without markup`},
		{"a control character", "A\tNovel", `item 1 (li_1): subtitle ("A\tNovel") contains the control character U+0009`},
		{"an invisible character", "A\u200b Novel", `item 1 (li_1): subtitle ("A\u200b Novel") contains the invisible character U+200B`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateRefused(t, subtitleBook("li_1", "Solaris", "[роман]"), ItemChange{ItemID: "li_1", Subtitle: new(tt.subtitle)}, tt.want)
		})
	}
}

func TestUpdate_SubtitleAtTheLimitPasses(t *testing.T) {
	// The limit counts characters, not bytes: 200 Cyrillic letters are 400 bytes.
	subtitle := strings.Repeat("я", MaxSubtitleLength)

	sent := updateAccepted(t, subtitleBook("li_1", "Solaris", ""), ItemChange{ItemID: "li_1", Subtitle: new(subtitle)})

	assert.Equal(t, &abs.MetadataPayload{Subtitle: new(subtitle)}, sent[0].MediaPayload.Metadata)
}

func TestUpdate_AStoredSubtitleSentBackPassesAtAnyLength(t *testing.T) {
	stored := " [" + strings.Repeat("я", MaxSubtitleLength+50) + "<b>] "

	sent := updateAccepted(t, subtitleBook("li_1", "Solaris", stored), ItemChange{ItemID: "li_1", Subtitle: new(strings.TrimSpace(stored))})

	assert.Equal(t, &abs.MetadataPayload{Subtitle: new(strings.TrimSpace(stored))}, sent[0].MediaPayload.Metadata,
		"old junk never blocks an edit, as for the other fields")
}

func TestUpdate_UnchangedValuesPass(t *testing.T) {
	description := "<div>" + strings.Repeat("a", MaxDescriptionLength+1) + "</div>"
	stored := metaBook("li_1", "Solaris", "", "", " Rus\tsian ", description, nil, nil)
	api := NewMockMetadataAPI(t)
	api.EXPECT().ItemsByID(mock.Anything, []string{"li_1"}).Return([]abs.LibraryItem{stored}, nil).Twice()
	sent := recordUpdates(api, 0)

	_, err := NewMetadata(api).Update(t.Context(), []ItemChange{
		{ItemID: "li_1", Language: new("Rus\tsian"), Description: new(description)},
	})

	require.NoError(t, err)
	require.Len(t, *sent, 1)
	assert.Equal(t, &abs.MetadataPayload{Language: new("Rus\tsian"), Description: new(description)}, (*sent)[0].MediaPayload.Metadata)
}

func TestUpdate_ErrorQuotesOnlyTheStartOfALongValue(t *testing.T) {
	api := NewMockMetadataAPI(t)
	api.EXPECT().ItemsByID(mock.Anything, []string{"li_1"}).Return([]abs.LibraryItem{nullBook("li_1", "Solaris")}, nil).Once()

	_, err := NewMetadata(api).Update(t.Context(), []ItemChange{{ItemID: "li_1", Language: new(strings.Repeat("a", 16000))}})

	require.EqualError(t, err, fmt.Sprintf(`item 1 (li_1): language (%q…) is 16000 characters long; at most 64`, strings.Repeat("a", 40)))
}

// creditsBook is a book with the given series, authors and narrators and
// nothing else set.
func creditsBook(id, title string, series []abs.SeriesRef, authors []abs.AuthorRef, narrators []string) abs.LibraryItem {
	return libraryItem(id, abs.MediaTypeBook, abs.BookMedia{
		Metadata: abs.BookMetadata{Title: title, Series: series, Authors: authors, Narrators: narrators},
	})
}

// updateAccepted runs an update of one book stored as stored, read twice as
// the call does, and returns what was sent to Audiobookshelf.
func updateAccepted(t *testing.T, stored abs.LibraryItem, change ItemChange) []abs.ItemUpdate {
	t.Helper()
	api := NewMockMetadataAPI(t)
	api.EXPECT().ItemsByID(mock.Anything, []string{change.ItemID}).Return([]abs.LibraryItem{stored}, nil).Twice()
	sent := recordUpdates(api, 1)

	_, err := NewMetadata(api).Update(t.Context(), []ItemChange{change})

	require.NoError(t, err)
	require.Len(t, *sent, 1)
	return *sent
}

func TestUpdate_SetsSeriesAuthorsNarrators(t *testing.T) {
	holly := []abs.SeriesRef{{ID: "s1", Name: "Holly Gibney", Sequence: "3"}}
	king := []abs.AuthorRef{{ID: "a1", Name: "Stephen King"}}
	api := NewMockMetadataAPI(t)
	api.EXPECT().ItemsByID(mock.Anything, []string{"li_1"}).
		Return([]abs.LibraryItem{creditsBook("li_1", "Holly", holly, king, []string{})}, nil).Once()
	api.EXPECT().ItemsByID(mock.Anything, []string{"li_1"}).
		Return([]abs.LibraryItem{creditsBook("li_1", "Holly", holly, king, []string{"Will Patton"})}, nil).Once()
	sent := recordUpdates(api, 1)

	got, err := NewMetadata(api).Update(t.Context(), []ItemChange{{
		ItemID:    "li_1",
		Series:    new([]SeriesEntry{{Name: "Holly Gibney", Sequence: "3"}}),
		Authors:   new([]string{"Stephen King"}),
		Narrators: new([]string{"Will Patton"}),
	}})

	require.NoError(t, err)
	// Names go alone: Audiobookshelf finds each series and author by name, or
	// creates it.
	assert.Equal(t, []abs.ItemUpdate{{ID: "li_1", MediaPayload: abs.MediaPayload{Metadata: &abs.MetadataPayload{
		Series:    &[]abs.SeriesRef{{Name: "Holly Gibney", Sequence: "3"}},
		Authors:   &[]abs.AuthorRef{{Name: "Stephen King"}},
		Narrators: &[]string{"Will Patton"},
	}}}}, *sent)
	assertSentJSON(t, `[{"id":"li_1","mediaPayload":{"metadata":{
		"series":[{"name":"Holly Gibney","sequence":"3"}],
		"authors":[{"name":"Stephen King"}],
		"narrators":["Will Patton"]
	}}}]`, *sent)
	assert.Equal(t, UpdateResult{Changed: 1, Items: []ItemUpdateResult{{
		ItemID: "li_1", Title: "Holly",
		Previous: ItemFields{
			Series:    new([]SeriesEntry{{Name: "Holly Gibney", Sequence: "3"}}),
			Authors:   new([]string{"Stephen King"}),
			Narrators: new([]string{}),
		},
		Now: ItemFields{
			Series:    new([]SeriesEntry{{Name: "Holly Gibney", Sequence: "3"}}),
			Authors:   new([]string{"Stephen King"}),
			Narrators: new([]string{"Will Patton"}),
		},
	}}}, got)
}

func TestUpdate_TrimsListValues(t *testing.T) {
	change := ItemChange{
		ItemID:    "li_1",
		Series:    new([]SeriesEntry{{Name: " Holly Gibney ", Sequence: " 3 "}, {Name: "Bill Hodges"}}),
		Authors:   new([]string{" Stephen King "}),
		Narrators: new([]string{"\tWill Patton\n"}),
	}

	sent := updateAccepted(t, nullBook("li_1", "Holly"), change)

	assertSentJSON(t, `[{"id":"li_1","mediaPayload":{"metadata":{
		"series":[{"name":"Holly Gibney","sequence":"3"},{"name":"Bill Hodges"}],
		"authors":[{"name":"Stephen King"}],
		"narrators":["Will Patton"]
	}}}]`, sent)
	assert.Equal(t, " Holly Gibney ", (*change.Series)[0].Name, "the caller's change is left as it was")
	assert.Equal(t, " Stephen King ", (*change.Authors)[0], "the caller's change is left as it was")
}

func TestUpdate_UnchangedListPassesUnchecked(t *testing.T) {
	// The stored author ends in a right-to-left override, which a new value
	// would be refused for.
	const hidden = "Stephen King\u202e"
	stored := creditsBook("li_1", "Holly", nil, []abs.AuthorRef{{ID: "a1", Name: hidden}}, nil)

	t.Run("as it is", func(t *testing.T) {
		sent := updateAccepted(t, stored, ItemChange{ItemID: "li_1", Authors: new([]string{hidden})})

		assert.Equal(t, &[]abs.AuthorRef{{Name: hidden}}, sent[0].MediaPayload.Metadata.Authors)
	})
	t.Run("next to a new one", func(t *testing.T) {
		sent := updateAccepted(t, stored, ItemChange{ItemID: "li_1", Authors: new([]string{hidden, "Owen King"})})

		assert.Equal(t, &[]abs.AuthorRef{{Name: hidden}, {Name: "Owen King"}}, sent[0].MediaPayload.Metadata.Authors)
	})
}

func TestUpdate_UnchangedSeriesPassUnchecked(t *testing.T) {
	// A stored series that new text would be refused for, a sequence cut
	// loose from the others, and a name stored with spaces around it.
	long := strings.Repeat("1", MaxSequenceLength+1)
	stored := creditsBook("li_1", "Holly", []abs.SeriesRef{
		{ID: "s1", Name: "Holly\u202e Gibney", Sequence: long},
		{ID: "s2", Name: " Bill Hodges ", Sequence: "1"},
	}, nil, nil)

	t.Run("the same list", func(t *testing.T) {
		sent := updateAccepted(t, stored, ItemChange{ItemID: "li_1", Series: new([]SeriesEntry{
			{Name: "Holly\u202e Gibney", Sequence: long}, {Name: "Bill Hodges", Sequence: "1"},
		})})

		assert.Equal(t, &[]abs.SeriesRef{
			{Name: "Holly\u202e Gibney", Sequence: long}, {Name: "Bill Hodges", Sequence: "1"},
		}, sent[0].MediaPayload.Metadata.Series)
	})
	t.Run("in another order", func(t *testing.T) {
		sent := updateAccepted(t, stored, ItemChange{ItemID: "li_1", Series: new([]SeriesEntry{
			{Name: "Bill Hodges", Sequence: "1"}, {Name: "Holly\u202e Gibney", Sequence: long},
		})})

		assert.Equal(t, &[]abs.SeriesRef{
			{Name: "Bill Hodges", Sequence: "1"}, {Name: "Holly\u202e Gibney", Sequence: long},
		}, sent[0].MediaPayload.Metadata.Series)
	})
	t.Run("a stored name with a new sequence", func(t *testing.T) {
		sent := updateAccepted(t, stored, ItemChange{ItemID: "li_1", Series: new([]SeriesEntry{
			{Name: "Holly\u202e Gibney", Sequence: "4"},
		})})

		assert.Equal(t, &[]abs.SeriesRef{{Name: "Holly\u202e Gibney", Sequence: "4"}}, sent[0].MediaPayload.Metadata.Series)
	})
}

func TestUpdate_RefusesBadNames(t *testing.T) {
	stored := creditsBook("li_1", "Holly", []abs.SeriesRef{{ID: "s1", Name: "Holly Gibney", Sequence: "3"}},
		[]abs.AuthorRef{{ID: "a1", Name: "Stephen King"}}, []string{"Will Patton"})
	tests := []struct {
		name   string
		change ItemChange
		want   string
	}{
		{
			"series name with markup",
			ItemChange{Series: new([]SeriesEntry{{Name: "<b>Holly</b>"}})},
			`series name ("<b>Holly</b>") contains "<"; use plain text without markup`,
		},
		{
			"long series name",
			ItemChange{Series: new([]SeriesEntry{{Name: strings.Repeat("я", MaxValueLength+1)}})},
			fmt.Sprintf(`series name (%q…) is 65 characters long; at most 64`, strings.Repeat("я", 40)),
		},
		{
			"sequence with a control character",
			ItemChange{Series: new([]SeriesEntry{{Name: "Holly Gibney", Sequence: "1\t2"}})},
			`series sequence ("1\t2") contains the control character U+0009`,
		},
		{
			"long sequence",
			ItemChange{Series: new([]SeriesEntry{{Name: "Holly Gibney", Sequence: strings.Repeat("1", MaxSequenceLength+1)}})},
			`series sequence ("11111111111111111") is 17 characters long; at most 16`,
		},
		{
			"sequence with markup",
			ItemChange{Series: new([]SeriesEntry{{Name: "Holly Gibney", Sequence: "<4>"}})},
			`series sequence ("<4>") contains "<"; use plain text without markup`,
		},
		{
			"empty series name",
			ItemChange{Series: new([]SeriesEntry{{Name: "Holly Gibney"}, {Name: "  ", Sequence: "1"}})},
			`series 2 has no name`,
		},
		{
			"same series twice",
			ItemChange{Series: new([]SeriesEntry{{Name: "Holly Gibney", Sequence: "3"}, {Name: "holly gibney", Sequence: "4"}})},
			`series 2 ("holly gibney") repeats series 1 ("Holly Gibney"), ignoring case`,
		},
		{
			"long author",
			ItemChange{Authors: new([]string{strings.Repeat("я", MaxValueLength+1)})},
			fmt.Sprintf(`author (%q…) is 65 characters long; at most 64`, strings.Repeat("я", 40)),
		},
		{
			"author with a hidden character",
			ItemChange{Authors: new([]string{"Stephen King", "Owen King\u200b"})},
			`author ("Owen King\u200b") contains the invisible character U+200B`,
		},
		{
			"empty author",
			ItemChange{Authors: new([]string{""})},
			`author 1 has no name`,
		},
		{
			"same author twice",
			ItemChange{Authors: new([]string{"Stephen King", " stephen king"})},
			`author 2 ("stephen king") repeats author 1 ("Stephen King"), ignoring case`,
		},
		{
			"narrator with markup",
			ItemChange{Narrators: new([]string{"<i>Will Patton</i>"})},
			`narrator ("<i>Will Patton</i>") contains "<"; use plain text without markup`,
		},
		{
			"same narrator twice",
			ItemChange{Narrators: new([]string{"Will Patton", "Will Patton"})},
			`narrator 2 ("Will Patton") repeats narrator 1 ("Will Patton"), ignoring case`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.change.ItemID = "li_1"

			updateRefused(t, stored, tt.change, "item 1 (li_1): "+tt.want)
		})
	}
}

func TestUpdate_ListRefusalNamesTheBook(t *testing.T) {
	api := NewMockMetadataAPI(t) // an UpdateItems call fails the test
	api.EXPECT().ItemsByID(mock.Anything, []string{"li_1", "li_2"}).Return([]abs.LibraryItem{
		nullBook("li_1", "Holly"), nullBook("li_2", "Mr. Mercedes"),
	}, nil).Once()

	_, err := NewMetadata(api).Update(t.Context(), []ItemChange{
		{ItemID: "li_1", Authors: new([]string{"Stephen King"})},
		{ItemID: "li_2", Authors: new([]string{"Stephen King", "<i>x</i>"})},
	})

	var itemErr *ItemError
	require.ErrorAs(t, err, &itemErr)
	assert.Equal(t, 2, itemErr.Position)
	assert.Equal(t, "li_2", itemErr.ItemID)
	assert.Equal(t, `author ("<i>x</i>") contains "<"; use plain text without markup`, itemErr.Reason)
}

func TestUpdate_EmptySeriesClears(t *testing.T) {
	api := NewMockMetadataAPI(t)
	api.EXPECT().ItemsByID(mock.Anything, []string{"li_1"}).Return([]abs.LibraryItem{creditsBook("li_1", "Holly", []abs.SeriesRef{
		{ID: "s1", Name: "Holly Gibney", Sequence: "3"}, {ID: "s2", Name: "Mr. Mercedes"},
	}, nil, nil)}, nil).Once()
	api.EXPECT().ItemsByID(mock.Anything, []string{"li_1"}).
		Return([]abs.LibraryItem{creditsBook("li_1", "Holly", nil, nil, nil)}, nil).Once()
	sent := recordUpdates(api, 1)

	got, err := NewMetadata(api).Update(t.Context(), []ItemChange{{ItemID: "li_1", Series: new([]SeriesEntry{})}})

	require.NoError(t, err)
	assertSentJSON(t, `[{"id":"li_1","mediaPayload":{"metadata":{"series":[]}}}]`, *sent)
	assert.Equal(t, UpdateResult{Changed: 1, Items: []ItemUpdateResult{{
		ItemID: "li_1", Title: "Holly",
		Previous: ItemFields{Series: new([]SeriesEntry{{Name: "Holly Gibney", Sequence: "3"}, {Name: "Mr. Mercedes"}})},
		Now:      ItemFields{Series: new([]SeriesEntry{})},
	}}}, got)
}

func TestUpdate_ListLeftOutIsNotSent(t *testing.T) {
	stored := creditsBook("li_1", "Holly", nil, []abs.AuthorRef{{Name: "Stephen King"}}, nil)

	sent := updateAccepted(t, stored, ItemChange{ItemID: "li_1", Narrators: new([]string{"Will Patton"})})

	assertSentJSON(t, `[{"id":"li_1","mediaPayload":{"metadata":{"narrators":["Will Patton"]}}}]`, sent)
}

func TestCheckChanges_NothingSet(t *testing.T) {
	_, _, err := checkChanges([]ItemChange{{ItemID: "li_1", Language: new("English")}, {ItemID: " li_2 "}})

	var itemErr *ItemError
	require.ErrorAs(t, err, &itemErr)
	assert.Equal(t, 2, itemErr.Position)
	assert.Equal(t, "li_2", itemErr.ItemID)
	assert.Equal(t, "sets none of language, subtitle, description, series, authors, narrators", itemErr.Reason)
}

func TestCheckChanges_ASubtitleCounts(t *testing.T) {
	for _, subtitle := range []string{"A Novel", ""} {
		_, _, err := checkChanges([]ItemChange{{ItemID: "li_1", Subtitle: new(subtitle)}})

		assert.NoError(t, err, "a subtitle %q is a field set, and an empty one removes it", subtitle)
	}
}

func TestCheckChanges_EmptyListsCount(t *testing.T) {
	for _, c := range []ItemChange{
		{ItemID: "li_1", Series: new([]SeriesEntry{})},
		{ItemID: "li_1", Authors: new([]string{})},
		{ItemID: "li_1", Narrators: new([]string{})},
	} {
		_, _, err := checkChanges([]ItemChange{c})

		assert.NoError(t, err)
	}
}
