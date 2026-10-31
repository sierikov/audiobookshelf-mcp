package service

import (
	"cmp"
	"fmt"
	"html"
	"regexp"
	"slices"
	"strings"
)

var htmlTag = regexp.MustCompile(`<[^>]*>`)

// plainText turns an HTML description into plain text. Every tag becomes a
// space, so block tags never glue words together; entities are decoded and
// runs of whitespace collapse to one space.
func plainText(s string) string {
	s = htmlTag.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(html.UnescapeString(s)), " ")
}

// checkItemIDs returns the ids trimmed. It fails with [ErrNoItems] for none,
// [ErrTooManyItems] for more than [MaxItemsPerCall], and an [*ItemError] for
// the first empty or repeated id. Ids are case-sensitive.
func checkItemIDs(ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, ErrNoItems
	}
	if len(ids) > MaxItemsPerCall {
		return nil, fmt.Errorf("%w: %d, at most %d per call", ErrTooManyItems, len(ids), MaxItemsPerCall)
	}
	out := make([]string, len(ids))
	seen := make(map[string]bool, len(ids))
	for i, id := range ids {
		id = strings.TrimSpace(id)
		switch {
		case id == "":
			return nil, &ItemError{Position: i + 1, Reason: "has no id"}
		case seen[id]:
			return nil, &ItemError{Position: i + 1, ItemID: id, Reason: "is listed twice"}
		}
		seen[id] = true
		out[i] = id
	}
	return out, nil
}

// checkValues returns the values trimmed. It fails with [ErrNoValues] for
// none, and a [*ValueError] for the first value that is empty or differs
// from an earlier one only in case, which includes an exact repeat.
func checkValues(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, ErrNoValues
	}
	out := make([]string, len(values))
	for j, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			return nil, &ValueError{Position: j + 1, Reason: "is empty"}
		}
		for i, earlier := range out[:j] {
			if strings.EqualFold(earlier, v) {
				return nil, &ValueError{
					Position: j + 1,
					Reason:   fmt.Sprintf("differs only in case from value %d (%q)", i+1, earlier),
				}
			}
		}
		out[j] = v
	}
	return out, nil
}

// hasValue reports whether list holds v, a trimmed value. Stored values are
// compared without their surrounding spaces, which Audiobookshelf's scanner
// can leave, but otherwise exactly, so case matters.
func hasValue(list []string, v string) bool {
	return slices.ContainsFunc(list, func(e string) bool { return strings.TrimSpace(e) == v })
}

// addValues returns current with each of add appended unless it is already
// there, as [hasValue] matches, and the values it appended, in the order of
// add. It also returns the existing value, as stored, that differs only in
// case from one being added once its surrounding spaces are removed, or ""
// if there is none. Neither list is ever nil.
func addValues(current, add []string) (list, added []string, conflict string) {
	list = append([]string{}, current...)
	added = []string{}
	for _, v := range add {
		if hasValue(list, v) {
			continue
		}
		if conflict == "" {
			if i := slices.IndexFunc(list, func(e string) bool { return strings.EqualFold(strings.TrimSpace(e), v) }); i >= 0 {
				conflict = list[i]
			}
		}
		list = append(list, v)
		added = append(added, v)
	}
	return list, added, conflict
}

// removeValues returns current without the values in remove, as [hasValue]
// matches them, and the values of remove it dropped, in the order of
// remove. Neither list is ever nil.
func removeValues(current, remove []string) (list, removed []string) {
	list = make([]string, 0, len(current))
	for _, v := range current {
		if !slices.Contains(remove, strings.TrimSpace(v)) {
			list = append(list, v)
		}
	}
	removed = []string{}
	for _, v := range remove {
		if hasValue(current, v) {
			removed = append(removed, v)
		}
	}
	return list, removed
}

// isMissing reports whether a book lacks the field. A description counts as
// missing when it has no text once its HTML is removed.
func isMissing(m MetadataItem, f MissingField) bool {
	switch f {
	case MissingGenres:
		return len(m.Genres) == 0
	case MissingTags:
		return len(m.Tags) == 0
	case MissingLanguage:
		return strings.TrimSpace(m.Language) == ""
	case MissingDescription:
		return plainText(m.Description) == ""
	}
	return false
}

// matchesFilter reports whether a book passes every filter that is set.
// Genre, tag and language match exactly.
func matchesFilter(m MetadataItem, f ItemFilter) bool {
	switch {
	case f.Genre != "" && !slices.Contains(m.Genres, f.Genre):
		return false
	case f.Tag != "" && !slices.Contains(m.Tags, f.Tag):
		return false
	case f.Language != "" && m.Language != f.Language:
		return false
	case f.Missing != "" && !isMissing(m, f.Missing):
		return false
	}
	return true
}

// countValues counts the number of books carrying each value, given each
// book's list, most common first and by name, in byte order, among equals.
// A book whose list repeats a value counts once for it.
func countValues(lists [][]string) []ValueCount {
	counts := map[string]int{}
	for _, l := range lists {
		for _, v := range slices.Compact(slices.Sorted(slices.Values(l))) {
			counts[v]++
		}
	}
	out := make([]ValueCount, 0, len(counts))
	for name, n := range counts {
		out = append(out, ValueCount{Name: name, Count: n})
	}
	slices.SortFunc(out, func(a, b ValueCount) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.Name, b.Name))
	})
	return out
}
