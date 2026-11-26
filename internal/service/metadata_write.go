package service

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

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
// no two equal ignoring case, and on add plain text as [checkPlain] says),
// and uses them trimmed. One call
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
	if op == addToList {
		if err := checkPlainValues(values); err != nil {
			return ListChangeResult{}, err
		}
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
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

// Update sets the language, description, series, authors and/or narrators of
// books.
//
// Before any call it checks the changes: 1 to [MaxItemsPerCall] of them
// ([ErrNoItems], [ErrTooManyItems]); ids non-empty and unique after
// trimming (an [*ItemError]); at most [MaxDescriptionsPerCall] setting a
// description ([ErrTooManyDescriptions]); and each setting at least one
// field (an [*ItemError]). Ids, languages, descriptions and the names and
// sequences of the lists are trimmed.
//
// One call reads every book for Previous, which holds only the fields being
// set, as stored: a null reads as "" or as an empty list and a description is
// its HTML. An unknown id is an [ErrUnknownItems] error and an item that is
// not a book an [ErrBooksOnly] one. New values are then checked as
// [checkFields] says. One call sends only the fields given; an empty string
// clears a language or description, and a list replaces the book's whole
// list, an empty one removing it. Changed is Audiobookshelf's count of books
// that changed. A second read gives Now, with the same fields as Previous.
// Items has every book in the order given.
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
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	before, err := s.readBooks(ctx, ids)
	if err != nil {
		return UpdateResult{}, err
	}
	for i, c := range changes {
		if changes[i], err = checkFields(c, before[i], i+1); err != nil {
			return UpdateResult{}, err
		}
	}
	res := UpdateResult{Items: make([]ItemUpdateResult, len(changes))}
	updates := make([]abs.ItemUpdate, len(changes))
	for i, c := range changes {
		res.Items[i] = ItemUpdateResult{ItemID: c.ItemID, Title: before[i].Metadata.Title, Previous: storedFields(before[i], c)}
		updates[i] = abs.ItemUpdate{ID: c.ItemID, MediaPayload: abs.MediaPayload{Metadata: metadataPayload(c)}}
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
		if c.setsNothing() {
			return nil, nil, &ItemError{Position: i + 1, ItemID: ids[i], Reason: "sets none of language, description, series, authors, narrators"}
		}
		out[i] = ItemChange{
			ItemID: ids[i], Language: trimmed(c.Language), Description: trimmed(c.Description),
			Series: mapPtr(c.Series, trimmedEntry), Authors: mapPtr(c.Authors, strings.TrimSpace), Narrators: mapPtr(c.Narrators, strings.TrimSpace),
		}
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

// checkFields checks the fields c sets on book b, the change at position
// pos, and returns c with its description cleaned. c is trimmed, as
// [checkChanges] returns it. An empty string, which clears, and a value equal
// to the stored one, both trimmed, pass untouched. A new language must pass
// [checkPlain]; a new description must be at most [MaxDescriptionLength]
// characters and keep some text through [cleanHTML]; the lists must pass
// [checkLists]. A failure is an [*ItemError].
func checkFields(c ItemChange, b *abs.BookMedia, pos int) (ItemChange, error) {
	if isNew(c.Language, b.Metadata.Language) {
		if reason := checkPlain(*c.Language, MaxValueLength); reason != "" {
			return c, &ItemError{Position: pos, ItemID: c.ItemID, Reason: fmt.Sprintf("language (%s) %s", quoted(*c.Language), reason)}
		}
	}
	if isNew(c.Description, b.Metadata.Description) {
		cleaned, reason := cleanDescription(*c.Description)
		if reason != "" {
			return c, &ItemError{Position: pos, ItemID: c.ItemID, Reason: reason}
		}
		c.Description = &cleaned
	}
	if reason := checkLists(c, b.Metadata); reason != "" {
		return c, &ItemError{Position: pos, ItemID: c.ItemID, Reason: reason}
	}
	return c, nil
}

// setsNothing reports whether c sets none of the fields a change can set.
func (c ItemChange) setsNothing() bool {
	return c.Language == nil && c.Description == nil && c.Series == nil && c.Authors == nil && c.Narrators == nil
}

// isNew reports whether v sets a value that is neither empty nor, trimmed,
// the stored one.
func isNew(v *string, stored string) bool {
	return v != nil && *v != "" && strings.TrimSpace(*v) != strings.TrimSpace(stored)
}

// cleanDescription returns a new description through [cleanHTML], or why it
// is refused: too long as sent, or no text left once cleaned.
func cleanDescription(s string) (cleaned, reason string) {
	if n := utf8.RuneCountInString(s); n > MaxDescriptionLength {
		return "", fmt.Sprintf("description is %d characters long; at most %d", n, MaxDescriptionLength)
	}
	cleaned = cleanHTML(s)
	if !hasVisibleText(plainText(cleaned)) {
		return "", `description has no text left once markup outside the allowlist is removed; send "" to clear it`
	}
	return cleaned, ""
}

// checkLists returns why the series, authors and narrators c sets on a book
// with metadata m are refused, or "". See [checkNames] and [checkSeries].
func checkLists(c ItemChange, m abs.BookMetadata) string {
	return cmp.Or(
		checkSeries(c.Series, storedSeries(m)),
		checkNames("author", c.Authors, storedAuthors(m)),
		checkNames("narrator", c.Narrators, m.Narrators),
	)
}

// checkNames returns why the names a change sets, as the noun they are
// ("author"), are refused, or "". A nil list, or one equal to stored, passes
// untouched, stored being compared without the spaces around its values.
// Otherwise a name must not be empty, nor repeat another ignoring case, and
// each one stored does not hold must pass [checkPlain] with [MaxValueLength].
func checkNames(noun string, names *[]string, stored []string) string {
	if names == nil || slices.EqualFunc(*names, stored, func(n, s string) bool { return n == strings.TrimSpace(s) }) {
		return ""
	}
	if reason := checkShape(noun, *names); reason != "" {
		return reason
	}
	for _, n := range *names {
		if hasValue(stored, n) {
			continue
		}
		if reason := badValue(noun, n, MaxValueLength); reason != "" {
			return reason
		}
	}
	return ""
}

// checkSeries returns why a series list a change sets is refused, or "". A
// nil list, or one equal to stored, passes untouched. Otherwise a name must
// not be empty, nor repeat another ignoring case, and each entry must pass
// [checkEntry].
func checkSeries(list *[]SeriesEntry, stored []SeriesEntry) string {
	if list == nil || slices.EqualFunc(*list, stored, sameEntry) {
		return ""
	}
	if reason := checkShape("series", mapList(*list, func(e SeriesEntry) string { return e.Name })); reason != "" {
		return reason
	}
	for _, e := range *list {
		if reason := checkEntry(e, stored); reason != "" {
			return reason
		}
	}
	return ""
}

// checkEntry returns why a series entry, a trimmed one, is refused, or "". An
// entry that is stored, a name that is, and an empty sequence pass untouched;
// a new name must pass [checkPlain] with [MaxValueLength] and a new sequence
// with [MaxSequenceLength].
func checkEntry(e SeriesEntry, stored []SeriesEntry) string {
	if slices.ContainsFunc(stored, func(s SeriesEntry) bool { return sameEntry(e, s) }) {
		return ""
	}
	var name, sequence string
	if !slices.ContainsFunc(stored, func(s SeriesEntry) bool { return e.Name == strings.TrimSpace(s.Name) }) {
		name = badValue("series name", e.Name, MaxValueLength)
	}
	if e.Sequence != "" {
		sequence = badValue("series sequence", e.Sequence, MaxSequenceLength)
	}
	return cmp.Or(name, sequence)
}

// sameEntry reports whether the entry e, a trimmed one, is the stored entry s,
// whose name and sequence are compared without surrounding spaces.
func sameEntry(e, s SeriesEntry) bool {
	return e.Name == strings.TrimSpace(s.Name) && e.Sequence == strings.TrimSpace(s.Sequence)
}

// checkShape returns why the trimmed names of a list of the noun are refused,
// or "": one is empty, or one repeats an earlier one, which includes an exact
// repeat, ignoring case. Positions are 1-based.
func checkShape(noun string, names []string) string {
	for j, n := range names {
		if n == "" {
			return fmt.Sprintf("%s %d has no name", noun, j+1)
		}
		if i := slices.IndexFunc(names[:j], func(earlier string) bool { return strings.EqualFold(earlier, n) }); i >= 0 {
			return fmt.Sprintf("%s %d (%s) repeats %s %d (%s), ignoring case", noun, j+1, quoted(n), noun, i+1, quoted(names[i]))
		}
	}
	return ""
}

// badValue returns why [checkPlain] refuses v with a limit, as a reason that
// says what v is, or "" for a good value.
func badValue(what, v string, limit int) string {
	reason := checkPlain(v, limit)
	if reason == "" {
		return ""
	}
	return fmt.Sprintf("%s (%s) %s", what, quoted(v), reason)
}

// trimmedEntry returns e without the spaces around its name and sequence.
func trimmedEntry(e SeriesEntry) SeriesEntry {
	return SeriesEntry{Name: strings.TrimSpace(e.Name), Sequence: strings.TrimSpace(e.Sequence)}
}

// storedSeries returns the book's series as stored, never nil.
func storedSeries(m abs.BookMetadata) []SeriesEntry {
	return mapList(m.Series, func(r abs.SeriesRef) SeriesEntry { return SeriesEntry{Name: r.Name, Sequence: r.Sequence} })
}

// storedAuthors returns the names of the book's authors as stored, never nil.
func storedAuthors(m abs.BookMetadata) []string {
	return mapList(m.Authors, func(a abs.AuthorRef) string { return a.Name })
}

// storedNarrators returns the book's narrators as stored, never nil.
func storedNarrators(m abs.BookMetadata) []string {
	return append([]string{}, m.Narrators...)
}

// mapList returns f of each element of list, as a list that is never nil.
func mapList[T, U any](list []T, f func(T) U) []U {
	out := make([]U, len(list))
	for i, v := range list {
		out[i] = f(v)
	}
	return out
}

// mapPtr is [mapList] for a list a change may leave out: nil stays nil, and
// the result of anything else is never a pointer to nil.
func mapPtr[T, U any](list *[]T, f func(T) U) *[]U {
	if list == nil {
		return nil
	}
	return new(mapList(*list, f))
}

// storedFields returns the book's stored values for the fields c sets. A
// null reads as "" or as an empty list; a description is its HTML.
func storedFields(b *abs.BookMedia, c ItemChange) ItemFields {
	var f ItemFields
	if c.Language != nil {
		f.Language = new(b.Metadata.Language)
	}
	if c.Description != nil {
		f.Description = new(b.Metadata.Description)
	}
	if c.Series != nil {
		f.Series = new(storedSeries(b.Metadata))
	}
	if c.Authors != nil {
		f.Authors = new(storedAuthors(b.Metadata))
	}
	if c.Narrators != nil {
		f.Narrators = new(storedNarrators(b.Metadata))
	}
	return f
}

// metadataPayload is the update that sends the fields c sets and no others.
// Series and authors go by name: Audiobookshelf finds each one regardless of
// case, or creates it.
func metadataPayload(c ItemChange) *abs.MetadataPayload {
	return &abs.MetadataPayload{
		Language:    c.Language,
		Description: c.Description,
		Series:      mapPtr(c.Series, func(e SeriesEntry) abs.SeriesRef { return abs.SeriesRef{Name: e.Name, Sequence: e.Sequence} }),
		Authors:     mapPtr(c.Authors, func(name string) abs.AuthorRef { return abs.AuthorRef{Name: name} }),
		Narrators:   c.Narrators,
	}
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
