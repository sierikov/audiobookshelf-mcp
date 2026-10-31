package abs

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// Libraries lists all libraries the token can see.
func (c *Client) Libraries(ctx context.Context) ([]Library, error) {
	var resp struct {
		Libraries []Library `json:"libraries"`
	}
	err := c.get(ctx, "/api/libraries", nil, &resp)
	return resp.Libraries, err
}

// SearchLibrary searches a library by title, author or narrator.
func (c *Client) SearchLibrary(ctx context.Context, libraryID, query string, limit int) (SearchResults, error) {
	params := url.Values{
		"q":     {query},
		"limit": {strconv.Itoa(limit)},
	}
	var resp SearchResults
	err := c.get(ctx, "/api/libraries/"+pathParam(libraryID)+"/search", params, &resp)
	return resp, err
}

// LibraryItems lists one page of a library's items, sorted by sort when set.
func (c *Client) LibraryItems(ctx context.Context, libraryID string, page Page, sort string) (ItemsPage, error) {
	params := page.query()
	if sort != "" {
		params.Set("sort", sort)
	}
	var resp ItemsPage
	err := c.get(ctx, "/api/libraries/"+pathParam(libraryID)+"/items", params, &resp)
	return resp, err
}

// Item fetches one library item with expanded media.
func (c *Client) Item(ctx context.Context, itemID string) (LibraryItem, error) {
	var item LibraryItem
	err := c.get(ctx, "/api/items/"+pathParam(itemID), url.Values{"expanded": {"1"}}, &item)
	return item, err
}

// ItemsInProgress lists the items the user is currently listening to.
func (c *Client) ItemsInProgress(ctx context.Context, limit int) ([]LibraryItem, error) {
	var resp struct {
		LibraryItems []LibraryItem `json:"libraryItems"`
	}
	err := c.get(ctx, "/api/me/items-in-progress", url.Values{"limit": {strconv.Itoa(limit)}}, &resp)
	return resp.LibraryItems, err
}

// Progress fetches the user's progress on an item, or on one podcast episode
// when episodeID is set.
func (c *Client) Progress(ctx context.Context, itemID, episodeID string) (MediaProgress, error) {
	path := "/api/me/progress/" + pathParam(itemID)
	if episodeID != "" {
		path += "/" + pathParam(episodeID)
	}
	var prog MediaProgress
	err := c.get(ctx, path, nil, &prog)
	return prog, err
}

// ListeningStats fetches the user's aggregated listening statistics.
func (c *Client) ListeningStats(ctx context.Context) (ListeningStats, error) {
	var stats ListeningStats
	err := c.get(ctx, "/api/me/listening-stats", nil, &stats)
	return stats, err
}

// ListeningSessions lists the user's most recent listening sessions.
func (c *Client) ListeningSessions(ctx context.Context, limit int) (SessionsPage, error) {
	var resp SessionsPage
	err := c.get(ctx, "/api/me/listening-sessions", url.Values{"itemsPerPage": {strconv.Itoa(limit)}}, &resp)
	return resp, err
}

// LibrarySeries lists one page of a library's series.
func (c *Client) LibrarySeries(ctx context.Context, libraryID string, page Page) (SeriesPage, error) {
	var resp SeriesPage
	err := c.get(ctx, "/api/libraries/"+pathParam(libraryID)+"/series", page.query(), &resp)
	return resp, err
}

// Series fetches one series with its books.
func (c *Client) Series(ctx context.Context, seriesID string) (Series, error) {
	var series Series
	err := c.get(ctx, "/api/series/"+pathParam(seriesID), nil, &series)
	return series, err
}

// Author fetches one author with their library items.
func (c *Client) Author(ctx context.Context, authorID string) (Author, error) {
	var author Author
	err := c.get(ctx, "/api/authors/"+pathParam(authorID), url.Values{"expand": {"1"}}, &author)
	return author, err
}

// LibraryCollections lists one page of a library's collections.
func (c *Client) LibraryCollections(ctx context.Context, libraryID string, page Page) (CollectionsPage, error) {
	var resp CollectionsPage
	err := c.get(ctx, "/api/libraries/"+pathParam(libraryID)+"/collections", page.query(), &resp)
	return resp, err
}

// SearchBooks asks a metadata provider ("audible", "audible.de", …) for books
// matching title and author. When itemID is set, Audiobookshelf compares the
// results with that item and sorts Audible results by how close their
// runtime is to the item's duration.
func (c *Client) SearchBooks(ctx context.Context, provider, title, author, itemID string) ([]BookMatch, error) {
	params := url.Values{"provider": {provider}, "title": {title}}
	if author != "" {
		params.Set("author", author)
	}
	if itemID != "" {
		params.Set("id", itemID)
	}
	var matches []BookMatch
	err := c.get(ctx, "/api/search/books", params, &matches)
	return matches, err
}

// AudibleChapters fetches Audible's chapters for an ASIN in a region.
// Audiobookshelf reports a lookup failure (not found, invalid ASIN or region)
// as a successful response with an error message, which becomes an error here.
func (c *Client) AudibleChapters(ctx context.Context, asin, region string) (AudibleChapters, error) {
	var resp struct {
		AudibleChapters
		Error string `json:"error"`
	}
	err := c.get(ctx, "/api/search/chapters", url.Values{"asin": {asin}, "region": {region}}, &resp)
	if err != nil {
		return AudibleChapters{}, err
	}
	if resp.Error != "" {
		return AudibleChapters{}, fmt.Errorf("ABS API: %s", resp.Error)
	}
	return resp.AudibleChapters, nil
}

// SaveChapters replaces all of an item's chapters and reports whether
// anything changed. An empty list removes all chapters.
func (c *Client) SaveChapters(ctx context.Context, itemID string, chapters []Chapter) (bool, error) {
	if chapters == nil {
		chapters = []Chapter{} // Audiobookshelf rejects null.
	}
	body := struct {
		Chapters []Chapter `json:"chapters"`
	}{chapters}
	var resp struct {
		Updated bool `json:"updated"`
	}
	err := c.do(ctx, http.MethodPost, "/api/items/"+pathParam(itemID)+"/chapters", nil, body, &resp)
	return resp.Updated, err
}

// ItemsByID fetches several library items in one request.
func (c *Client) ItemsByID(ctx context.Context, ids []string) ([]LibraryItem, error) {
	body := struct {
		LibraryItemIDs []string `json:"libraryItemIds"`
	}{ids}
	var resp struct {
		LibraryItems []LibraryItem `json:"libraryItems"`
	}
	err := c.do(ctx, http.MethodPost, "/api/items/batch/get", nil, body, &resp)
	return resp.LibraryItems, err
}

// UpdateItems updates the media of several items in one request and returns
// the number of items Audiobookshelf changed.
func (c *Client) UpdateItems(ctx context.Context, updates []ItemUpdate) (int, error) {
	var resp struct {
		Updates int `json:"updates"`
	}
	err := c.do(ctx, http.MethodPost, "/api/items/batch/update", nil, updates, &resp)
	return resp.Updates, err
}
