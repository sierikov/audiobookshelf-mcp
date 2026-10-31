// Package abstest provides a fake Audiobookshelf API server and JSON fixtures
// for tests that exercise the real HTTP client.
package abstest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// Token is the bearer token the fake server accepts.
const Token = "test-token"

// Server is a fake Audiobookshelf API that answers each path with canned JSON.
type Server struct {
	*httptest.Server

	routes map[string]any

	mu       sync.Mutex
	requests []*http.Request
	bodies   [][]byte
	paths    []string
}

// NewServer starts a fake API serving routes (URL path → JSON value).
// Requests without the bearer Token get 401, unknown paths get 404.
// The server is closed when the test ends.
func NewServer(tb testing.TB, routes map[string]any) *Server {
	tb.Helper()
	s := &Server{routes: routes}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	tb.Cleanup(s.Close)
	return s
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.requests = append(s.requests, r)
	s.bodies = append(s.bodies, body)
	s.paths = append(s.paths, r.URL.Path)
	s.mu.Unlock()

	if r.Header.Get("Authorization") != "Bearer "+Token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	resp, ok := s.routes[r.URL.Path]
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// LastRequest returns the most recent request the server received, or nil.
func (s *Server) LastRequest() *http.Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.requests) == 0 {
		return nil
	}
	return s.requests[len(s.requests)-1]
}

// LastBody returns the body of the most recent request, or nil.
func (s *Server) LastBody() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.bodies) == 0 {
		return nil
	}
	return s.bodies[len(s.bodies)-1]
}

// LastBodyFor returns the body of the most recent request to path, or nil.
func (s *Server) LastBodyFor(path string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.paths) - 1; i >= 0; i-- {
		if s.paths[i] == path {
			return s.bodies[i]
		}
	}
	return nil
}

// BookItem builds a book library item fixture.
func BookItem(id, title, author string, duration float64) map[string]any {
	return map[string]any{
		"id":        id,
		"mediaType": "book",
		"media": map[string]any{
			"metadata": map[string]any{
				"title":      title,
				"authorName": author,
			},
			"duration": duration,
		},
	}
}

// BookItemFull builds a book item with arbitrary metadata fields.
func BookItemFull(id string, metadata map[string]any, duration float64) map[string]any {
	return map[string]any{
		"id":        id,
		"mediaType": "book",
		"media": map[string]any{
			"metadata": metadata,
			"duration": duration,
		},
	}
}

// PodcastItem builds a podcast library item fixture.
func PodcastItem(id, title, author string, numEpisodes int) map[string]any {
	return map[string]any{
		"id":        id,
		"mediaType": "podcast",
		"media": map[string]any{
			"metadata": map[string]any{
				"title":  title,
				"author": author,
			},
			"numEpisodes": numEpisodes,
		},
	}
}

// SearchBookHit wraps a book item in the search response format.
func SearchBookHit(id, title, author string, duration float64) map[string]any {
	return map[string]any{"libraryItem": BookItem(id, title, author, duration)}
}

// Library builds a library fixture.
func Library(id, name, mediaType string) map[string]any {
	return map[string]any{"id": id, "name": name, "mediaType": mediaType}
}

// Session builds a listening session fixture.
func Session(id, itemID, title, author string, duration float64, startedAt int64) map[string]any {
	return map[string]any{
		"id": id, "libraryItemId": itemID,
		"displayTitle": title, "displayAuthor": author,
		"duration": duration, "startedAt": startedAt,
	}
}

// Progress builds a media progress fixture.
func Progress(id, itemID string, duration, prog, currentTime float64, finished bool) map[string]any {
	return map[string]any{
		"id": id, "libraryItemId": itemID,
		"duration": duration, "progress": prog, "currentTime": currentTime,
		"isFinished": finished, "lastUpdate": int64(1668586015691), "startedAt": int64(1668120083771),
	}
}

// EmptySearchResults builds an empty search response.
func EmptySearchResults() map[string]any {
	return map[string]any{
		"book": []any{}, "podcast": []any{}, "authors": []any{}, "series": []any{},
	}
}

// Chapter builds a book chapter fixture. Times are in seconds.
func Chapter(id int, start, end float64, title string) map[string]any {
	return map[string]any{"id": id, "start": start, "end": end, "title": title}
}

// Track builds an audio track fixture: one audio file placed on the book's
// timeline. Audiobookshelf puts the file name in the track's title.
func Track(index int, filename string, startOffset, duration float64) map[string]any {
	return map[string]any{"index": index, "title": filename, "startOffset": startOffset, "duration": duration}
}

// BookWithChapters builds an expanded book item with an ASIN, chapters and
// audio tracks, as GET /api/items/:id?expanded=1 returns it.
func BookWithChapters(id, title, author, asin string, duration float64, chapters, tracks []any) map[string]any {
	return map[string]any{
		"id":        id,
		"mediaType": "book",
		"media": map[string]any{
			"metadata": map[string]any{"title": title, "authorName": author, "asin": asin},
			"duration": duration,
			"chapters": chapters,
			"tracks":   tracks,
		},
	}
}

// BookWithMetadata builds a book item with the fields a metadata survey
// reads: genres, language and description in the metadata, tags on the media.
func BookWithMetadata(id, title string, genres, tags []string, language, description string) map[string]any {
	return map[string]any{
		"id":        id,
		"mediaType": "book",
		"media": map[string]any{
			"metadata": map[string]any{
				"title":       title,
				"genres":      genres,
				"language":    language,
				"description": description,
			},
			"tags": tags,
		},
	}
}
