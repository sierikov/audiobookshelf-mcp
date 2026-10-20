package service

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
)

var errAPI = errors.New("ABS API 500: boom")

func libraryItem(id, mediaType string, media any) abs.LibraryItem {
	raw, err := json.Marshal(media)
	if err != nil {
		panic(err)
	}
	return abs.LibraryItem{ID: id, MediaType: mediaType, Media: raw}
}

func bookItem(id, title, author string, seconds float64) abs.LibraryItem {
	return libraryItem(id, abs.MediaTypeBook, abs.BookMedia{
		Metadata: abs.BookMetadata{Title: title, AuthorName: author},
		Duration: seconds,
	})
}

func podcastItem(id, title, author string, episodes int) abs.LibraryItem {
	return libraryItem(id, abs.MediaTypePodcast, abs.PodcastMedia{
		Metadata:    abs.PodcastMetadata{Title: title, Author: author},
		NumEpisodes: episodes,
	})
}

// brokenItem has media that cannot be decoded as its media type.
func brokenItem(id, mediaType string) abs.LibraryItem {
	return abs.LibraryItem{ID: id, MediaType: mediaType, Media: json.RawMessage(`"not an object"`)}
}

func book(id, title, author string, d time.Duration) Item {
	return Item{ID: id, MediaType: abs.MediaTypeBook, Title: title, Author: author, Duration: d, HasDuration: true}
}

func podcast(id, title, author string) Item {
	return Item{ID: id, MediaType: abs.MediaTypePodcast, Title: title, Author: author}
}

func bookWithChapters(id, title, author, asin string, seconds float64, chapters []abs.Chapter, tracks []abs.AudioTrack) abs.LibraryItem {
	return libraryItem(id, abs.MediaTypeBook, abs.BookMedia{
		Metadata: abs.BookMetadata{Title: title, AuthorName: author, ASIN: asin},
		Duration: seconds,
		Chapters: chapters,
		Tracks:   tracks,
	})
}

// oneTrack is a single audio file covering a book of the given length.
func oneTrack(seconds float64) []abs.AudioTrack {
	return []abs.AudioTrack{{Index: 1, Title: "book.m4b", StartOffset: 0, Duration: seconds}}
}
