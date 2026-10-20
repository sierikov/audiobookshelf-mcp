package service

import (
	"time"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
)

// seconds converts the API's float seconds to a duration.
func seconds(s float64) time.Duration {
	return time.Duration(s * float64(time.Second))
}

// epochMillis converts the API's millisecond timestamps; 0 means unknown.
func epochMillis(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}

// toItemDetail decodes a library item's media. Items of an unknown media
// type keep only their ID and media type.
func toItemDetail(li abs.LibraryItem) (ItemDetail, error) {
	d := ItemDetail{Item: Item{ID: li.ID, MediaType: li.MediaType}}
	switch li.MediaType {
	case abs.MediaTypeBook:
		b, err := li.AsBook()
		if err != nil {
			return ItemDetail{}, &MediaError{ItemID: li.ID, MediaType: li.MediaType, Err: err}
		}
		m := b.Metadata
		d.Title, d.Author = m.Title, m.AuthorName
		d.Duration, d.HasDuration = seconds(b.Duration), true
		d.Subtitle, d.Narrator, d.SeriesName = m.Subtitle, m.NarratorName, m.SeriesName
		d.Description, d.PublishedYear, d.Publisher = m.Description, m.PublishedYear, m.Publisher
		d.Genres, d.Language = m.Genres, m.Language
	case abs.MediaTypePodcast:
		p, err := li.AsPodcast()
		if err != nil {
			return ItemDetail{}, &MediaError{ItemID: li.ID, MediaType: li.MediaType, Err: err}
		}
		m := p.Metadata
		d.Title, d.Author = m.Title, m.Author
		d.Description, d.Genres, d.Language = m.Description, m.Genres, m.Language
		d.NumEpisodes = p.NumEpisodes
	}
	return d, nil
}

func toItem(li abs.LibraryItem) (Item, error) {
	d, err := toItemDetail(li)
	return d.Item, err
}

// toItems maps library items, skipping those whose media cannot be decoded.
func toItems(lis []abs.LibraryItem) []Item {
	items := make([]Item, 0, len(lis))
	for _, li := range lis {
		if it, err := toItem(li); err == nil {
			items = append(items, it)
		}
	}
	return items
}

// itemOfType maps li only if it has the given media type and decodes.
func itemOfType(li abs.LibraryItem, mediaType string) (Item, bool) {
	if li.MediaType != mediaType {
		return Item{}, false
	}
	it, err := toItem(li)
	return it, err == nil
}

// itemsOfType keeps the decodable items of one media type; nil when none.
func itemsOfType(lis []abs.LibraryItem, mediaType string) []Item {
	var items []Item
	for _, li := range lis {
		if it, ok := itemOfType(li, mediaType); ok {
			items = append(items, it)
		}
	}
	return items
}

func toProgress(p abs.MediaProgress) Progress {
	return Progress{
		Fraction:    p.Progress,
		CurrentTime: seconds(p.CurrentTime),
		Duration:    seconds(p.Duration),
		Remaining:   seconds(p.Duration - p.CurrentTime),
		IsFinished:  p.IsFinished,
		LastUpdate:  epochMillis(p.LastUpdate),
		StartedAt:   epochMillis(p.StartedAt),
	}
}

// toSessions maps listening sessions; nil when there are none.
func toSessions(ss []abs.ListeningSession) []Session {
	if len(ss) == 0 {
		return nil
	}
	sessions := make([]Session, 0, len(ss))
	for _, s := range ss {
		sessions = append(sessions, Session{
			Title:     s.DisplayTitle,
			Author:    s.DisplayAuthor,
			Duration:  seconds(s.Duration),
			StartedAt: epochMillis(s.StartedAt),
		})
	}
	return sessions
}
