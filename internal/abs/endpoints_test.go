package abs_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
	"github.com/sierikov/audiobookshelf-mcp/internal/abs/abstest"
)

func newClient(t *testing.T, routes map[string]any) (*abs.Client, *abstest.Server) {
	t.Helper()
	srv := abstest.NewServer(t, routes)
	return abs.NewClient(srv.URL, abstest.Token), srv
}

func query(srv *abstest.Server) url.Values {
	return srv.LastRequest().URL.Query()
}

func TestLibraries(t *testing.T) {
	c, _ := newClient(t, map[string]any{"/api/libraries": map[string]any{
		"libraries": []any{abstest.Library("lib-1", "Audiobooks", "book")},
	}})

	libs, err := c.Libraries(context.Background())

	require.NoError(t, err)
	assert.Equal(t, []abs.Library{{ID: "lib-1", Name: "Audiobooks", MediaType: "book"}}, libs)
}

func TestSearchLibrary(t *testing.T) {
	c, srv := newClient(t, map[string]any{"/api/libraries/lib-1/search": map[string]any{
		"book":    []any{abstest.SearchBookHit("item-1", "Dune", "Frank Herbert", 60)},
		"authors": []any{map[string]any{"id": "a-1", "name": "Frank Herbert"}},
	}})

	res, err := c.SearchLibrary(context.Background(), "lib-1", "dune", 10)

	require.NoError(t, err)
	assert.Equal(t, url.Values{"q": {"dune"}, "limit": {"10"}}, query(srv))
	require.Len(t, res.Book, 1)
	assert.Equal(t, "item-1", res.Book[0].LibraryItem.ID)
	assert.Equal(t, []abs.SearchAuthorHit{{ID: "a-1", Name: "Frank Herbert"}}, res.Authors)
}

func TestLibraryItems(t *testing.T) {
	c, srv := newClient(t, map[string]any{"/api/libraries/lib-1/items": map[string]any{
		"results": []any{abstest.BookItem("item-1", "Dune", "Frank Herbert", 60)},
		"total":   7,
		"page":    2,
	}})

	page, err := c.LibraryItems(context.Background(), "lib-1", abs.Page{Limit: 5, Page: 2}, "media.metadata.title")

	require.NoError(t, err)
	assert.Equal(t, url.Values{"limit": {"5"}, "page": {"2"}, "sort": {"media.metadata.title"}}, query(srv))
	assert.Equal(t, 7, page.Total)
	assert.Equal(t, 2, page.Page)
	require.Len(t, page.Results, 1)
	assert.Equal(t, "item-1", page.Results[0].ID)
}

func TestLibraryItems_OmitsEmptySort(t *testing.T) {
	c, srv := newClient(t, map[string]any{"/api/libraries/lib-1/items": map[string]any{"results": []any{}}})

	_, err := c.LibraryItems(context.Background(), "lib-1", abs.Page{Limit: 20}, "")

	require.NoError(t, err)
	assert.Equal(t, url.Values{"limit": {"20"}, "page": {"0"}}, query(srv))
}

func TestItem(t *testing.T) {
	c, srv := newClient(t, map[string]any{"/api/items/item-1": abstest.BookItem("item-1", "Dune", "Frank Herbert", 60)})

	item, err := c.Item(context.Background(), "item-1")

	require.NoError(t, err)
	assert.Equal(t, url.Values{"expanded": {"1"}}, query(srv))
	assert.Equal(t, "item-1", item.ID)
	book, err := item.AsBook()
	require.NoError(t, err)
	assert.Equal(t, "Dune", book.Metadata.Title)
}

func TestItemsInProgress(t *testing.T) {
	c, srv := newClient(t, map[string]any{"/api/me/items-in-progress": map[string]any{
		"libraryItems": []any{abstest.PodcastItem("pod-1", "Hardcore History", "Dan Carlin", 68)},
	}})

	items, err := c.ItemsInProgress(context.Background(), 25)

	require.NoError(t, err)
	assert.Equal(t, url.Values{"limit": {"25"}}, query(srv))
	require.Len(t, items, 1)
	assert.Equal(t, abs.MediaTypePodcast, items[0].MediaType)
}

func TestProgress(t *testing.T) {
	c, _ := newClient(t, map[string]any{
		"/api/me/progress/item-1":      abstest.Progress("p-1", "item-1", 100, 0.5, 50, false),
		"/api/me/progress/item-1/ep-1": abstest.Progress("p-2", "item-1", 100, 1, 100, true),
	})

	prog, err := c.Progress(context.Background(), "item-1", "")
	require.NoError(t, err)
	assert.Equal(t, "p-1", prog.ID)
	assert.InDelta(t, 0.5, prog.Progress, 0)
	assert.Equal(t, int64(1668586015691), prog.LastUpdate)

	prog, err = c.Progress(context.Background(), "item-1", "ep-1")
	require.NoError(t, err)
	assert.Equal(t, "p-2", prog.ID)
	assert.True(t, prog.IsFinished)
}

func TestListeningStats(t *testing.T) {
	c, _ := newClient(t, map[string]any{"/api/me/listening-stats": map[string]any{
		"totalTime": 3660.0,
		"today":     1800.0,
		"dayOfWeek": map[string]any{"Sunday": 3600.0},
	}})

	stats, err := c.ListeningStats(context.Background())

	require.NoError(t, err)
	assert.InDelta(t, 3660.0, stats.TotalTime, 0)
	assert.InDelta(t, 1800.0, stats.Today, 0)
	assert.Equal(t, map[string]float64{"Sunday": 3600}, stats.DayOfWeek)
}

func TestListeningSessions(t *testing.T) {
	c, srv := newClient(t, map[string]any{"/api/me/listening-sessions": map[string]any{
		"sessions": []any{abstest.Session("s-1", "item-1", "Dune", "Frank Herbert", 60, 1707350400000)},
		"total":    3,
	}})

	page, err := c.ListeningSessions(context.Background(), 25)

	require.NoError(t, err)
	assert.Equal(t, url.Values{"itemsPerPage": {"25"}}, query(srv))
	assert.Equal(t, 3, page.Total)
	require.Len(t, page.Sessions, 1)
	assert.Equal(t, "Dune", page.Sessions[0].DisplayTitle)
}

func TestLibrarySeries(t *testing.T) {
	c, srv := newClient(t, map[string]any{"/api/libraries/lib-1/series": map[string]any{
		"results": []any{map[string]any{"id": "s-1", "name": "Dune", "books": []any{abstest.BookItem("b1", "Dune", "Frank Herbert", 60)}}},
		"total":   1,
	}})

	page, err := c.LibrarySeries(context.Background(), "lib-1", abs.Page{Limit: 20, Page: 1})

	require.NoError(t, err)
	assert.Equal(t, url.Values{"limit": {"20"}, "page": {"1"}}, query(srv))
	assert.Equal(t, 1, page.Total)
	require.Len(t, page.Results, 1)
	assert.Equal(t, "Dune", page.Results[0].Name)
	assert.Len(t, page.Results[0].Books, 1)
}

func TestSeries(t *testing.T) {
	c, _ := newClient(t, map[string]any{"/api/series/s-1": map[string]any{
		"id": "s-1", "name": "Dune", "description": "Spice",
		"books": []any{abstest.BookItem("b1", "Dune", "Frank Herbert", 60)},
	}})

	series, err := c.Series(context.Background(), "s-1")

	require.NoError(t, err)
	assert.Equal(t, "Spice", series.Description)
	assert.Len(t, series.Books, 1)
}

func TestAuthor(t *testing.T) {
	c, srv := newClient(t, map[string]any{"/api/authors/a-1": map[string]any{
		"id": "a-1", "name": "Frank Herbert", "numBooks": 6,
		"libraryItems": []any{abstest.BookItem("b1", "Dune", "Frank Herbert", 60)},
	}})

	author, err := c.Author(context.Background(), "a-1")

	require.NoError(t, err)
	assert.Equal(t, url.Values{"expand": {"1"}}, query(srv))
	assert.Equal(t, 6, author.NumBooks)
	assert.Len(t, author.LibraryItems, 1)
}

func TestLibraryCollections(t *testing.T) {
	c, srv := newClient(t, map[string]any{"/api/libraries/lib-1/collections": map[string]any{
		"results": []any{map[string]any{"id": "c-1", "name": "Favorites", "description": "Best", "books": []any{}}},
		"total":   1,
	}})

	page, err := c.LibraryCollections(context.Background(), "lib-1", abs.Page{Limit: 20})

	require.NoError(t, err)
	assert.Equal(t, url.Values{"limit": {"20"}, "page": {"0"}}, query(srv))
	assert.Equal(t, 1, page.Total)
	require.Len(t, page.Results, 1)
	assert.Equal(t, "Best", page.Results[0].Description)
}

func TestLibraryItem_AsBookAndAsPodcast(t *testing.T) {
	c, _ := newClient(t, map[string]any{"/api/items/pod-1": abstest.PodcastItem("pod-1", "Hardcore History", "Dan Carlin", 68)})
	item, err := c.Item(context.Background(), "pod-1")
	require.NoError(t, err)

	_, err = item.AsBook()
	require.EqualError(t, err, "item pod-1 is not a book (mediaType=podcast)")

	pod, err := item.AsPodcast()
	require.NoError(t, err)
	assert.Equal(t, 68, pod.NumEpisodes)
}

func TestItem_DecodesChaptersTracksAndASIN(t *testing.T) {
	c, _ := newClient(t, map[string]any{"/api/items/item-1": abstest.BookWithChapters(
		"item-1", "Dune", "Frank Herbert", "B002V1OF70", 3600,
		[]any{abstest.Chapter(0, 0, 600, "Opening")},
		[]any{abstest.Track(1, "m001.mp3", 0, 3600)},
	)})

	item, err := c.Item(t.Context(), "item-1")

	require.NoError(t, err)
	book, err := item.AsBook()
	require.NoError(t, err)
	assert.Equal(t, "B002V1OF70", book.Metadata.ASIN)
	assert.Equal(t, []abs.Chapter{{ID: 0, Start: 0, End: 600, Title: "Opening"}}, book.Chapters)
	assert.Equal(t, []abs.AudioTrack{{Index: 1, Title: "m001.mp3", StartOffset: 0, Duration: 3600}}, book.Tracks)
}

func TestSearchBooks(t *testing.T) {
	c, srv := newClient(t, map[string]any{"/api/search/books": []any{
		map[string]any{
			"title": "Dune", "subtitle": "Book 1", "author": "Frank Herbert", "narrator": "Simon Vance",
			"asin": "B002V1OF70", "language": "English", "duration": 1260, "abridged": false,
			"matchConfidence": 0.9, "region": "de",
		},
		map[string]any{
			"title": "Dune", "subtitle": nil, "author": "Frank Herbert", "narrator": nil,
			"asin": "B0000DUNE2", "language": nil, "duration": 600, "abridged": true,
		},
	}})

	matches, err := c.SearchBooks(t.Context(), "audible.de", "Dune", "Frank Herbert", "item-1")

	require.NoError(t, err)
	assert.Equal(t, url.Values{
		"provider": {"audible.de"}, "title": {"Dune"}, "author": {"Frank Herbert"}, "id": {"item-1"},
	}, query(srv))
	require.Len(t, matches, 2)
	assert.Equal(t, abs.BookMatch{
		Title: "Dune", Subtitle: "Book 1", Author: "Frank Herbert", Narrator: "Simon Vance",
		ASIN: "B002V1OF70", Language: "English", Duration: 1260, MatchConfidence: new(0.9),
	}, matches[0])
	assert.Equal(t, abs.BookMatch{
		Title: "Dune", Author: "Frank Herbert", ASIN: "B0000DUNE2", Duration: 600, Abridged: true,
	}, matches[1])
}

func TestSearchBooks_OmitsEmptyAuthorAndID(t *testing.T) {
	c, srv := newClient(t, map[string]any{"/api/search/books": []any{}})

	_, err := c.SearchBooks(t.Context(), "audible", "Dune", "", "")

	require.NoError(t, err)
	assert.Equal(t, url.Values{"provider": {"audible"}, "title": {"Dune"}}, query(srv))
}

func TestAudibleChapters(t *testing.T) {
	c, srv := newClient(t, map[string]any{"/api/search/chapters": map[string]any{
		"asin": "B002V1OF70", "region": "us",
		"brandIntroDurationMs": 2043, "brandOutroDurationMs": 5061,
		"isAccurate": true, "runtimeLengthMs": 3600000, "runtimeLengthSec": 3600,
		"chapters": []any{
			map[string]any{"lengthMs": 600000, "startOffsetMs": 0, "startOffsetSec": 0, "title": "Opening Credits"},
			map[string]any{"lengthMs": 3000000, "startOffsetMs": 600000, "startOffsetSec": 600, "title": "Chapter 1"},
		},
	}})

	got, err := c.AudibleChapters(t.Context(), "B002V1OF70", "us")

	require.NoError(t, err)
	assert.Equal(t, url.Values{"asin": {"B002V1OF70"}, "region": {"us"}}, query(srv))
	assert.Equal(t, abs.AudibleChapters{
		ASIN: "B002V1OF70", Region: "us",
		BrandIntroDurationMs: 2043, BrandOutroDurationMs: 5061, RuntimeLengthMs: 3600000,
		IsAccurate: true,
		Chapters: []abs.AudibleChapter{
			{StartOffsetMs: 0, LengthMs: 600000, Title: "Opening Credits"},
			{StartOffsetMs: 600000, LengthMs: 3000000, Title: "Chapter 1"},
		},
	}, got)
}

func TestAudibleChapters_ErrorBodyIsError(t *testing.T) {
	c, _ := newClient(t, map[string]any{"/api/search/chapters": map[string]any{
		"error": "Chapters not found", "stringKey": "MessageChaptersNotFound",
	}})

	_, err := c.AudibleChapters(t.Context(), "B000000000", "us")

	require.EqualError(t, err, "ABS API: Chapters not found")
}

func TestSaveChapters(t *testing.T) {
	c, srv := newClient(t, map[string]any{"/api/items/item-1/chapters": map[string]any{"success": true, "updated": true}})

	updated, err := c.SaveChapters(t.Context(), "item-1", []abs.Chapter{{ID: 0, Start: 0, End: 600, Title: "Opening"}})

	require.NoError(t, err)
	assert.True(t, updated)
	assert.Equal(t, http.MethodPost, srv.LastRequest().Method)
	assert.Equal(t, "application/json", srv.LastRequest().Header.Get("Content-Type"))
	assert.JSONEq(t, `{"chapters":[{"id":0,"start":0,"end":600,"title":"Opening"}]}`, string(srv.LastBody()))
}

func TestSaveChapters_NilSendsEmptyArray(t *testing.T) {
	c, srv := newClient(t, map[string]any{"/api/items/item-1/chapters": map[string]any{"success": true, "updated": false}})

	updated, err := c.SaveChapters(t.Context(), "item-1", nil)

	require.NoError(t, err)
	assert.False(t, updated)
	assert.JSONEq(t, `{"chapters":[]}`, string(srv.LastBody()))
}

func TestSaveChapters_Non200IsError(t *testing.T) {
	c, _ := newClient(t, map[string]any{})

	_, err := c.SaveChapters(t.Context(), "item-1", []abs.Chapter{{Start: 0, End: 1, Title: "A"}})

	require.ErrorContains(t, err, "ABS API 404")
}

func TestSearchBooks_DecodesProviderShapes(t *testing.T) {
	c, _ := newClient(t, map[string]any{"/api/search/books": []any{
		map[string]any{
			"title": "Солярис", "author": "Станислав Лем",
			"publishedYear": 1961, "genres": []any{"Фантастика"},
			"description": "<p>Солярис…</p>",
		},
		map[string]any{
			"title": "Dune", "author": "Frank Herbert",
			"publishedYear": "2009", "tags": "Genre Fiction, Literary Fiction", "publisher": "Penguin",
		},
	}})

	matches, err := c.SearchBooks(t.Context(), "fantlab", "x", "", "")

	require.NoError(t, err)
	require.Len(t, matches, 2)
	assert.Equal(t, abs.FlexString("1961"), matches[0].PublishedYear)
	assert.Equal(t, []string{"Фантастика"}, matches[0].Genres)
	assert.Equal(t, "<p>Солярис…</p>", matches[0].Description)
	assert.Nil(t, matches[0].Tags)
	assert.Equal(t, abs.FlexString("2009"), matches[1].PublishedYear)
	assert.Equal(t, abs.FlexStrings{"Genre Fiction", "Literary Fiction"}, matches[1].Tags)
	assert.Equal(t, "Penguin", matches[1].Publisher)
}

func TestItem_DecodesTags(t *testing.T) {
	c, _ := newClient(t, map[string]any{
		"/api/items/li_1": abstest.BookWithMetadata("li_1", "It", []string{"Horror"}, []string{"horror", "thriller"}, "English", "<p>d</p>"),
		"/api/items/li_2": map[string]any{
			"id": "li_2", "mediaType": "book",
			"media": map[string]any{
				"metadata": map[string]any{"title": "Null", "genres": nil},
				"tags":     nil,
			},
		},
	})

	item, err := c.Item(t.Context(), "li_1")
	require.NoError(t, err)
	book, err := item.AsBook()
	require.NoError(t, err)
	assert.Equal(t, []string{"horror", "thriller"}, book.Tags)
	assert.Equal(t, []string{"Horror"}, book.Metadata.Genres)
	assert.Equal(t, "English", book.Metadata.Language)
	assert.Equal(t, "<p>d</p>", book.Metadata.Description)

	item, err = c.Item(t.Context(), "li_2")
	require.NoError(t, err)
	book, err = item.AsBook()
	require.NoError(t, err)
	assert.Nil(t, book.Tags)
	assert.Nil(t, book.Metadata.Genres)
}

func TestItemsByID(t *testing.T) {
	c, srv := newClient(t, map[string]any{"/api/items/batch/get": map[string]any{
		"libraryItems": []any{abstest.BookWithMetadata("li_1", "It", nil, nil, "", "")},
	}})

	items, err := c.ItemsByID(t.Context(), []string{"li_1", "li_2"})

	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, srv.LastRequest().Method)
	assert.JSONEq(t, `{"libraryItemIds":["li_1","li_2"]}`, string(srv.LastBody()))
	require.Len(t, items, 1)
	assert.Equal(t, "li_1", items[0].ID)
}

func TestUpdateItems(t *testing.T) {
	c, srv := newClient(t, map[string]any{"/api/items/batch/update": map[string]any{"success": true, "updates": 2}})

	n, err := c.UpdateItems(t.Context(), []abs.ItemUpdate{
		{ID: "li_1", MediaPayload: abs.MediaPayload{Tags: &[]string{}}},
		{ID: "li_2", MediaPayload: abs.MediaPayload{Metadata: &abs.MetadataPayload{Genres: &[]string{"Science Fiction"}}}},
		{ID: "li_3", MediaPayload: abs.MediaPayload{Metadata: &abs.MetadataPayload{Language: new(""), Description: new("<p>x</p>")}}},
	})

	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, http.MethodPost, srv.LastRequest().Method)
	assert.Equal(t, "application/json", srv.LastRequest().Header.Get("Content-Type"))
	assert.JSONEq(t, `[
		{"id":"li_1","mediaPayload":{"tags":[]}},
		{"id":"li_2","mediaPayload":{"metadata":{"genres":["Science Fiction"]}}},
		{"id":"li_3","mediaPayload":{"metadata":{"language":"","description":"<p>x</p>"}}}
	]`, string(srv.LastBody()))
}

func TestUpdateItems_Non200IsError(t *testing.T) {
	c, _ := newClient(t, map[string]any{})

	_, err := c.UpdateItems(t.Context(), []abs.ItemUpdate{{ID: "li_1"}})

	require.ErrorContains(t, err, "ABS API 404")
}

func TestLastBodyFor_ReturnsBodyOfThatPath(t *testing.T) {
	c, srv := newClient(t, map[string]any{
		"/api/items/batch/get":    map[string]any{"libraryItems": []any{}},
		"/api/items/batch/update": map[string]any{"success": true, "updates": 0},
	})
	_, err := c.UpdateItems(t.Context(), []abs.ItemUpdate{{ID: "li_1"}})
	require.NoError(t, err)
	_, err = c.ItemsByID(t.Context(), []string{"li_1"})
	require.NoError(t, err)

	assert.JSONEq(t, `[{"id":"li_1","mediaPayload":{}}]`, string(srv.LastBodyFor("/api/items/batch/update")))
	assert.JSONEq(t, `{"libraryItemIds":["li_1"]}`, string(srv.LastBody()))
	assert.Nil(t, srv.LastBodyFor("/api/other"))
}
