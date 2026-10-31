package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
)

func metaBook(id, title, author, series, language, description string, genres, tags []string) abs.LibraryItem {
	return libraryItem(id, abs.MediaTypeBook, abs.BookMedia{
		Metadata: abs.BookMetadata{
			Title: title, AuthorName: author, SeriesName: series,
			Language: language, Description: description, Genres: genres,
		},
		Tags: tags,
	})
}

// metaLibrary is three books: Solaris (two genres, no tags, plain blurb),
// "alpha" (one genre, a tag, an empty paragraph) and Zeta (nothing set).
func metaLibrary() abs.ItemsPage {
	return abs.ItemsPage{Total: 3, Results: []abs.LibraryItem{
		metaBook("li_1", "Solaris", "Станислав Лем", "Cycle", "english", "Real blurb",
			[]string{"Научная фантастика", "Fiction"}, nil),
		metaBook("li_2", "alpha", "A. Writer", "", "English", "<p></p>",
			[]string{"Научная фантастика"}, []string{"horror"}),
		metaBook("li_3", "Zeta", "Z. Writer", "", "", "", nil, nil),
	}}
}

func TestMetadataOverview(t *testing.T) {
	api := NewMockMetadataAPI(t)
	api.EXPECT().LibraryItems(mock.Anything, "lib_1", abs.Page{Limit: 0}, "").Return(metaLibrary(), nil)

	got, err := NewMetadata(api).Overview(t.Context(), "lib_1")

	require.NoError(t, err)
	assert.Equal(t, MetadataOverview{
		LibraryID: "lib_1",
		Books:     3,
		Genres:    []ValueCount{{"Научная фантастика", 2}, {"Fiction", 1}},
		Tags:      []ValueCount{{"horror", 1}},
		Languages: []ValueCount{{"English", 1}, {"english", 1}},
		Missing:   MissingCounts{Genres: 1, Tags: 2, Language: 1, Description: 2},
	}, got)
}

func TestMetadataOverview_PodcastLibraryRejected(t *testing.T) {
	api := NewMockMetadataAPI(t)
	api.EXPECT().LibraryItems(mock.Anything, "lib_p", abs.Page{Limit: 0}, "").
		Return(abs.ItemsPage{Results: []abs.LibraryItem{podcastItem("pod_1", "Show", "Host", 3)}}, nil)

	_, err := NewMetadata(api).Overview(t.Context(), "lib_p")

	require.ErrorIs(t, err, ErrBooksOnly)
	assert.Contains(t, err.Error(), "pod_1")
}

func TestMetadataOverview_UndecodableBook(t *testing.T) {
	api := NewMockMetadataAPI(t)
	api.EXPECT().LibraryItems(mock.Anything, "lib_1", abs.Page{Limit: 0}, "").
		Return(abs.ItemsPage{Results: []abs.LibraryItem{brokenItem("li_1", abs.MediaTypeBook)}}, nil)

	_, err := NewMetadata(api).Overview(t.Context(), "lib_1")

	var me *MediaError
	require.ErrorAs(t, err, &me)
	assert.Equal(t, "li_1", me.ItemID)
}

func TestMetadataOverview_APIError(t *testing.T) {
	api := NewMockMetadataAPI(t)
	api.EXPECT().LibraryItems(mock.Anything, "lib_1", abs.Page{Limit: 0}, "").Return(abs.ItemsPage{}, errAPI)

	_, err := NewMetadata(api).Overview(t.Context(), "lib_1")

	assert.Equal(t, errAPI, err)
}

func TestMetadataFind(t *testing.T) {
	tests := []struct {
		name   string
		filter ItemFilter
		page   Page
		ids    []string
		total  int
		pageNo int
	}{
		{"genre", ItemFilter{Genre: "Fiction"}, Page{Limit: 50}, []string{"li_1"}, 1, 0},
		{"missing description", ItemFilter{Missing: MissingDescription}, Page{Limit: 50}, []string{"li_2", "li_3"}, 2, 0},
		{"genre and language", ItemFilter{Genre: "Научная фантастика", Language: "English"}, Page{Limit: 50}, []string{"li_2"}, 1, 0},
		{"no filter sorted by lowercased title", ItemFilter{}, Page{Limit: 50}, []string{"li_2", "li_1", "li_3"}, 3, 0},
		{"second page", ItemFilter{}, Page{Limit: 2, Page: 1}, []string{"li_3"}, 3, 1},
		{"page past the end", ItemFilter{}, Page{Limit: 2, Page: 5}, nil, 3, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := NewMockMetadataAPI(t)
			api.EXPECT().LibraryItems(mock.Anything, "lib_1", abs.Page{Limit: 0}, "").Return(metaLibrary(), nil)

			got, err := NewMetadata(api).Find(t.Context(), "lib_1", tt.filter, tt.page)

			require.NoError(t, err)
			var ids []string
			for _, it := range got.Items {
				ids = append(ids, it.ID)
			}
			assert.Equal(t, tt.ids, ids)
			assert.Equal(t, tt.total, got.Total)
			assert.Equal(t, tt.pageNo, got.Page)
		})
	}
}

func TestMetadataFind_MapsBook(t *testing.T) {
	api := NewMockMetadataAPI(t)
	api.EXPECT().LibraryItems(mock.Anything, "lib_1", abs.Page{Limit: 0}, "").Return(abs.ItemsPage{Results: []abs.LibraryItem{
		metaBook("li_1", "Solaris", "Станислав Лем", "Cycle", "English", "<p>One</p><p>Two &amp; three</p>",
			[]string{"Fiction"}, []string{"classic"}),
	}}, nil)

	got, err := NewMetadata(api).Find(t.Context(), "lib_1", ItemFilter{}, Page{Limit: 10})

	require.NoError(t, err)
	assert.Equal(t, []MetadataItem{{
		ID: "li_1", Title: "Solaris", Author: "Станислав Лем", Series: "Cycle", Language: "English",
		Genres: []string{"Fiction"}, Tags: []string{"classic"}, Description: "One Two & three",
	}}, got.Items)
}

func TestMetadataFind_BadInput(t *testing.T) {
	tests := []struct {
		name   string
		filter ItemFilter
		page   Page
		target error
	}{
		{"limit 0", ItemFilter{}, Page{Limit: 0}, ErrBadPage},
		{"limit 101", ItemFilter{}, Page{Limit: 101}, ErrBadPage},
		{"negative page", ItemFilter{}, Page{Limit: 10, Page: -1}, ErrBadPage},
		{"unknown missing", ItemFilter{Missing: "cover"}, Page{Limit: 10}, ErrUnknownMissing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := NewMockMetadataAPI(t)

			_, err := NewMetadata(api).Find(t.Context(), "lib_1", tt.filter, tt.page)

			require.ErrorIs(t, err, tt.target)
			api.AssertNotCalled(t, "LibraryItems", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestMetadataFind_APIError(t *testing.T) {
	api := NewMockMetadataAPI(t)
	api.EXPECT().LibraryItems(mock.Anything, "lib_1", abs.Page{Limit: 0}, "").Return(abs.ItemsPage{}, errAPI)

	_, err := NewMetadata(api).Find(t.Context(), "lib_1", ItemFilter{}, Page{Limit: 10})

	assert.Equal(t, errAPI, err)
}

func TestMetadataBooksByID(t *testing.T) {
	api := NewMockMetadataAPI(t)
	api.EXPECT().ItemsByID(mock.Anything, []string{"li_1", "li_2"}).
		Return([]abs.LibraryItem{bookItem("li_2", "B", "b", 1), bookItem("li_1", "A", "a", 1)}, nil)

	got, err := NewMetadata(api).booksByID(t.Context(), []string{"li_1", "li_2"})

	require.NoError(t, err)
	assert.Len(t, got, 2)
	assert.Equal(t, "li_1", got["li_1"].ID)
	assert.Equal(t, "li_2", got["li_2"].ID)
}

func TestMetadataBooksByID_UnknownIDsInInputOrder(t *testing.T) {
	api := NewMockMetadataAPI(t)
	api.EXPECT().ItemsByID(mock.Anything, []string{"li_9", "li_1", "li_10"}).
		Return([]abs.LibraryItem{bookItem("li_1", "A", "a", 1)}, nil)

	_, err := NewMetadata(api).booksByID(t.Context(), []string{"li_9", "li_1", "li_10"})

	require.ErrorIs(t, err, ErrUnknownItems)
	assert.EqualError(t, err, "unknown item ids: li_9, li_10")
}

func TestMetadataBooksByID_PodcastRejected(t *testing.T) {
	api := NewMockMetadataAPI(t)
	api.EXPECT().ItemsByID(mock.Anything, []string{"pod_1"}).
		Return([]abs.LibraryItem{podcastItem("pod_1", "Show", "Host", 1)}, nil)

	_, err := NewMetadata(api).booksByID(t.Context(), []string{"pod_1"})

	require.ErrorIs(t, err, ErrBooksOnly)
}

func TestMetadataBooksByID_APIError(t *testing.T) {
	api := NewMockMetadataAPI(t)
	api.EXPECT().ItemsByID(mock.Anything, []string{"li_1"}).Return(nil, errAPI)

	_, err := NewMetadata(api).booksByID(t.Context(), []string{"li_1"})

	assert.Equal(t, errAPI, err)
}

func solaris() abs.LibraryItem {
	return bookItem("li_1", "Солярис", "Станислав Лем", 3600)
}

func TestMetadataSearchBooks(t *testing.T) {
	matches := []abs.BookMatch{{
		Title: "Солярис", Subtitle: "Роман", Author: "Станислав Лем", Narrator: "Чтец",
		Publisher: "АСТ", PublishedYear: "1961", Language: "Russian",
		Genres: []string{"Фантастика"}, Tags: abs.FlexStrings{"classic", "scifi"},
		Description: "<p>Blurb</p>",
	}}
	want := []MetadataCandidate{{
		Title: "Солярис", Subtitle: "Роман", Author: "Станислав Лем", Narrator: "Чтец",
		Publisher: "АСТ", PublishedYear: "1961", Language: "Russian",
		Genres: []string{"Фантастика"}, Tags: []string{"classic", "scifi"},
		Description: "<p>Blurb</p>",
	}}
	tests := []struct {
		name, provider, region, title string
		apiProvider, apiTitle         string
		wantProvider, wantRegion      string
	}{
		{"loose provider", " FantLab ", "", "", "fantlab", "Солярис", "fantlab", ""},
		{"audible region", "audible", "DE", "", "audible.de", "Солярис", "audible", "de"},
		{"audible default region", "audible", "", "Other", "audible", "Other", "audible", "us"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := NewMockMetadataAPI(t)
			api.EXPECT().Item(mock.Anything, "li_1").Return(solaris(), nil)
			api.EXPECT().SearchBooks(mock.Anything, tt.apiProvider, tt.apiTitle, "Станислав Лем", "li_1").Return(matches, nil)

			got, err := NewMetadata(api).SearchBooks(t.Context(), "li_1", tt.provider, tt.region, tt.title, "")

			require.NoError(t, err)
			assert.Equal(t, MetadataSearch{
				Book:       Item{ID: "li_1", MediaType: abs.MediaTypeBook, Title: "Солярис", Author: "Станислав Лем"},
				Provider:   tt.wantProvider,
				Region:     tt.wantRegion,
				Candidates: want,
			}, got)
		})
	}
}

func TestMetadataSearchBooks_BadInput(t *testing.T) {
	tests := []struct {
		name             string
		provider, region string
		item             abs.LibraryItem
		target           error
	}{
		{"unknown provider", "goodreads", "", solaris(), ErrUnknownProvider},
		{"region without audible", "fantlab", "de", solaris(), ErrRegionNotAudible},
		{"unknown audible region", "audible", "xx", solaris(), ErrUnknownRegion},
		{"podcast", "fantlab", "", podcastItem("li_1", "Show", "Host", 3), ErrBooksOnly},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := NewMockMetadataAPI(t)
			api.EXPECT().Item(mock.Anything, "li_1").Return(tt.item, nil).Maybe()

			_, err := NewMetadata(api).SearchBooks(t.Context(), "li_1", tt.provider, tt.region, "", "")

			require.ErrorIs(t, err, tt.target)
			api.AssertNotCalled(t, "SearchBooks", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestMetadataSearchBooks_UnknownProviderListsProviders(t *testing.T) {
	_, err := NewMetadata(NewMockMetadataAPI(t)).SearchBooks(t.Context(), "li_1", "goodreads", "", "", "")

	assert.ErrorContains(t, err, "fantlab, google, audible, openlibrary, itunes")
}

func TestMetadataSearchBooks_APIError(t *testing.T) {
	api := NewMockMetadataAPI(t)
	api.EXPECT().Item(mock.Anything, "li_1").Return(solaris(), nil)
	api.EXPECT().SearchBooks(mock.Anything, "google", "Солярис", "Станислав Лем", "li_1").Return(nil, errAPI)

	_, err := NewMetadata(api).SearchBooks(t.Context(), "li_1", "google", "", "", "")

	assert.Equal(t, errAPI, err)
}
