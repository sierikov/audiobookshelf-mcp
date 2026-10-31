package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		}, nil, "item 2 (li_2): sets neither language nor description"},
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
