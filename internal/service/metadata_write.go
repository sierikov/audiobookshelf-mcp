package service

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
)

// AddGenres adds genres to books, as [Metadata.changeLists] describes. A
// genre a book already has is skipped; one it has in another case is an
// [*ItemError], and then nothing is saved.
func (s *Metadata) AddGenres(ctx context.Context, itemIDs, genres []string) (ListChangeResult, error) {
	return s.changeLists(ctx, itemIDs, genres, genresList, addToList)
}

// RemoveGenres removes genres from books, as [Metadata.changeLists]
// describes. Genres match exactly, ignoring the spaces around a stored one;
// a book without them is left unchanged.
func (s *Metadata) RemoveGenres(ctx context.Context, itemIDs, genres []string) (ListChangeResult, error) {
	return s.changeLists(ctx, itemIDs, genres, genresList, removeFromList)
}

// AddTags adds tags to books, as [Metadata.changeLists] describes. A tag a
// book already has is skipped; one it has in another case is an
// [*ItemError], and then nothing is saved.
func (s *Metadata) AddTags(ctx context.Context, itemIDs, tags []string) (ListChangeResult, error) {
	return s.changeLists(ctx, itemIDs, tags, tagsList, addToList)
}

// RemoveTags removes tags from books, as [Metadata.changeLists] describes.
// Tags match exactly, ignoring the spaces around a stored one; a book
// without them is left unchanged.
func (s *Metadata) RemoveTags(ctx context.Context, itemIDs, tags []string) (ListChangeResult, error) {
	return s.changeLists(ctx, itemIDs, tags, tagsList, removeFromList)
}

// listField is the list on a book that a list write changes.
type listField int

const (
	genresList listField = iota // the genres, under the book's metadata
	tagsList                    // the tags, at the top level of its media
)

// of returns the book's list. A list stored as null is empty, never nil.
func (f listField) of(b *abs.BookMedia) []string {
	list := b.Metadata.Genres
	if f == tagsList {
		list = b.Tags
	}
	if list == nil {
		return []string{}
	}
	return list
}

// payload is an update that sends list as this field and nothing else.
func (f listField) payload(list []string) abs.MediaPayload {
	if f == tagsList {
		return abs.MediaPayload{Tags: &list}
	}
	return abs.MediaPayload{Metadata: &abs.MetadataPayload{Genres: &list}}
}

// listOp is what a list write does with its values.
type listOp int

const (
	addToList listOp = iota
	removeFromList
)

// apply returns current after the op and the values the op added or
// removed, in the order of values; neither is ever nil. On add it also
// returns the value current has, as stored, in another case of a value being
// added, or "". Stored values match without their surrounding spaces.
func (op listOp) apply(current, values []string) (list, changed []string, conflict string) {
	if op == removeFromList {
		list, changed = removeValues(current, values)
		return list, changed, ""
	}
	return addValues(current, values)
}

// changeLists adds values to, or removes them from, one list on each book.
//
// Before any call it checks the ids (1 to [MaxItemsPerCall], each non-empty
// and unique after trimming) and the values (at least one, each non-empty,
// no two equal ignoring case), and uses them trimmed. One call
// then reads every book: an unknown id is an [ErrUnknownItems] error and an
// item that is not a book an [ErrBooksOnly] one. Each book's new list is
// worked out in the order given; on add, a value the book has in another
// case is an [*ItemError], and nothing is sent.
//
// Only the books whose list changed are saved, in one call carrying just
// that list; if none changed, nothing is sent. Changed counts those books,
// and Items has every book in the order given, each with the values it
// gained or lost in Values, so the opposite call with those values undoes
// it. If the save fails, Now and Values are nil throughout, Previous is
// kept, and the error wraps [ErrWriteIncomplete] and the API error.
func (s *Metadata) changeLists(ctx context.Context, itemIDs, values []string, field listField, op listOp) (ListChangeResult, error) {
	ids, err := checkItemIDs(itemIDs)
	if err != nil {
		return ListChangeResult{}, err
	}
	if values, err = checkValues(values); err != nil {
		return ListChangeResult{}, err
	}
	books, err := s.readBooks(ctx, ids)
	if err != nil {
		return ListChangeResult{}, err
	}
	res := ListChangeResult{Items: make([]ListChange, len(ids))}
	updates := make([]abs.ItemUpdate, 0, len(ids))
	for i, b := range books {
		previous := field.of(b)
		now, changed, conflict := op.apply(previous, values)
		if conflict != "" {
			return ListChangeResult{}, &ItemError{Position: i + 1, ItemID: ids[i], Reason: caseConflict(conflict, values)}
		}
		res.Items[i] = ListChange{ItemID: ids[i], Title: b.Metadata.Title, Previous: previous, Now: now, Values: changed}
		if !slices.Equal(previous, now) {
			updates = append(updates, abs.ItemUpdate{ID: ids[i], MediaPayload: field.payload(now)})
		}
	}
	if len(updates) == 0 {
		return res, nil
	}
	if _, err := s.api.UpdateItems(ctx, updates); err != nil {
		for i := range res.Items {
			res.Items[i].Now, res.Items[i].Values = nil, nil
		}
		return res, fmt.Errorf("%w: %w", ErrWriteIncomplete, err)
	}
	res.Changed = len(updates)
	return res, nil
}

// caseConflict says that a book has existing, as stored, which differs only
// in case from one of the values being added. addValues found existing by
// matching its trimmed form to one of adding ignoring case, so the search
// below always finds it.
func caseConflict(existing string, adding []string) string {
	i := slices.IndexFunc(adding, func(v string) bool { return strings.EqualFold(v, strings.TrimSpace(existing)) })
	return fmt.Sprintf("already has %q; remove it before adding %q", existing, adding[i])
}

// Update sets the language and/or description of books.
//
// Before any call it checks the changes: 1 to [MaxItemsPerCall] of them
// ([ErrNoItems], [ErrTooManyItems]); ids non-empty and unique after
// trimming (an [*ItemError]); at most [MaxDescriptionsPerCall] setting a
// description ([ErrTooManyDescriptions]); and each setting at least one
// field (an [*ItemError]). Ids, languages and descriptions are trimmed.
//
// One call reads every book for Previous, which holds only the fields being
// set, as stored: a null reads as "" and a description is its HTML. An
// unknown id is an [ErrUnknownItems] error and an item that is not a book
// an [ErrBooksOnly] one. One call then sends only the fields given; an
// empty string clears a field.
// Changed is Audiobookshelf's count of books that changed. A second read
// gives Now, with the same fields as Previous. Items has every book in the
// order given.
//
// If the save fails, Now is empty, Previous is kept, and the error wraps
// [ErrWriteIncomplete] and the API error. If only the second read fails,
// the save went through: Now is empty, Changed and Previous are kept, and
// the error wraps [ErrReadBack] and the read's error.
func (s *Metadata) Update(ctx context.Context, changes []ItemChange) (UpdateResult, error) {
	changes, ids, err := checkChanges(changes)
	if err != nil {
		return UpdateResult{}, err
	}
	before, err := s.readBooks(ctx, ids)
	if err != nil {
		return UpdateResult{}, err
	}
	res := UpdateResult{Items: make([]ItemUpdateResult, len(changes))}
	updates := make([]abs.ItemUpdate, len(changes))
	for i, c := range changes {
		res.Items[i] = ItemUpdateResult{ItemID: c.ItemID, Title: before[i].Metadata.Title, Previous: storedFields(before[i], c)}
		updates[i] = abs.ItemUpdate{ID: c.ItemID, MediaPayload: abs.MediaPayload{
			Metadata: &abs.MetadataPayload{Language: c.Language, Description: c.Description},
		}}
	}
	n, err := s.api.UpdateItems(ctx, updates)
	if err != nil {
		return res, fmt.Errorf("%w: %w", ErrWriteIncomplete, err)
	}
	res.Changed = n
	after, err := s.readBooks(ctx, ids)
	if err != nil {
		return res, fmt.Errorf("%w: %w", ErrReadBack, err)
	}
	for i, c := range changes {
		res.Items[i].Now = storedFields(after[i], c)
	}
	return res, nil
}

// checkChanges returns the changes trimmed and their ids, after the checks
// [Metadata.Update] lists. The caller's changes are left as they are.
func checkChanges(changes []ItemChange) ([]ItemChange, []string, error) {
	ids := make([]string, len(changes))
	descriptions := 0
	for i, c := range changes {
		ids[i] = c.ItemID
		if c.Description != nil {
			descriptions++
		}
	}
	ids, err := checkItemIDs(ids)
	if err != nil {
		return nil, nil, err
	}
	if descriptions > MaxDescriptionsPerCall {
		return nil, nil, fmt.Errorf("%w: %d, at most %d per call", ErrTooManyDescriptions, descriptions, MaxDescriptionsPerCall)
	}
	out := make([]ItemChange, len(changes))
	for i, c := range changes {
		if c.Language == nil && c.Description == nil {
			return nil, nil, &ItemError{Position: i + 1, ItemID: ids[i], Reason: "sets neither language nor description"}
		}
		out[i] = ItemChange{ItemID: ids[i], Language: trimmed(c.Language), Description: trimmed(c.Description)}
	}
	return out, ids, nil
}

// trimmed returns a new pointer to s without surrounding spaces, or nil if s
// is nil.
func trimmed(s *string) *string {
	if s == nil {
		return nil
	}
	return new(strings.TrimSpace(*s))
}

// storedFields returns the book's stored values for the fields c sets. A
// null reads as ""; a description is its HTML.
func storedFields(b *abs.BookMedia, c ItemChange) ItemFields {
	var f ItemFields
	if c.Language != nil {
		f.Language = new(b.Metadata.Language)
	}
	if c.Description != nil {
		f.Description = new(b.Metadata.Description)
	}
	return f
}

// readBooks reads books by id in one call and decodes them, in the order of
// ids. It fails as [Metadata.booksByID] does, and with a [*MediaError] for a
// book whose media cannot be decoded.
func (s *Metadata) readBooks(ctx context.Context, ids []string) ([]*abs.BookMedia, error) {
	byID, err := s.booksByID(ctx, ids)
	if err != nil {
		return nil, err
	}
	books := make([]*abs.BookMedia, len(ids))
	for i, id := range ids {
		if books[i], err = asBook(byID[id]); err != nil {
			return nil, err
		}
	}
	return books, nil
}
