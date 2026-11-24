package service

import (
	"fmt"
	"html"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/microcosm-cc/bluemonday"
)

// descriptionPolicy is Audiobookshelf's own allowlist for descriptions,
// applied on our side so it holds whatever Audiobookshelf version runs. A
// stripped tag leaves a space, so stripped blocks don't glue words together.
var descriptionPolicy = func() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("p", "ol", "ul", "li", "strong", "em", "del", "br", "b", "i")
	p.AllowAttrs("href").OnElements("a")
	p.AllowURLSchemes("http", "https", "mailto")
	p.RequireParseableURLs(true)
	p.AddSpaceWhenStrippingTag(true)
	return p
}()

// textPolicy strips every tag, leaving a space in its place.
var textPolicy = func() *bluemonday.Policy {
	p := bluemonday.StrictPolicy()
	p.AddSpaceWhenStrippingTag(true)
	return p
}()

// isHidden reports whether r is an invisible character that has no meaning
// in prose and is used to hide or reorder text: zero width spaces, the word
// joiner and invisible operators, the BOM, bidi embeddings, overrides and
// isolates ("Trojan Source"), and tag characters ("ASCII smuggling"). The
// zero width joiner and non-joiner, direction marks, the soft hyphen and
// variation selectors are not hidden: real scripts and emoji need them.
func isHidden(r rune) bool {
	switch {
	case r == '\u200b', r == '\ufeff':
		return true
	case r >= '\u2060' && r <= '\u2064':
		return true
	case r >= '\u202a' && r <= '\u202e':
		return true
	case r >= '\u2066' && r <= '\u2069':
		return true
	case r >= '\U000e0000' && r <= '\U000e007f':
		return true
	}
	return false
}

// dropRunes returns s without the runes drop reports.
func dropRunes(s string, drop func(rune) bool) string {
	return strings.Map(func(r rune) rune {
		if drop(r) {
			return -1
		}
		return r
	}, s)
}

// cleanHTML returns a description reduced to Audiobookshelf's allowlist: p,
// ol, ul, li, a, strong, em, del, br, b and i, links only with an http,
// https or mailto URL that parses. Everything else goes, including scripts,
// styles, comments and event attributes; text hidden with CSS stays and
// becomes visible. Hidden characters and control characters other than tab,
// line feed and carriage return are removed after sanitizing, so ones
// written as entities go too.
func cleanHTML(s string) string {
	s = descriptionPolicy.Sanitize(s)
	s = dropRunes(s, func(r rune) bool {
		return isHidden(r) || unicode.IsControl(r) && r != '\t' && r != '\n' && r != '\r'
	})
	return strings.TrimSpace(s)
}

// plainText turns HTML into plain text. Every tag becomes a space, so block
// tags never glue words together; entities are decoded, hidden and control
// characters removed, and runs of whitespace collapse to one space.
func plainText(s string) string {
	s = html.UnescapeString(textPolicy.Sanitize(s))
	s = dropRunes(s, func(r rune) bool { return isHidden(r) || unicode.IsControl(r) })
	return strings.Join(strings.Fields(s), " ")
}

// hasVisibleText reports whether s has a letter, digit, punctuation mark or
// symbol, rather than only spaces, marks and the joiners and direction marks
// [isHidden] lets through.
func hasVisibleText(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
	})
}

// quoted quotes s for an error message, cut to its first 40 characters and
// "…", so a long value can't flood the error.
func quoted(s string) string {
	const head = 40
	r := []rune(s)
	if len(r) <= head {
		return strconv.Quote(s)
	}
	return strconv.Quote(string(r[:head])) + "…"
}

// checkPlain checks a value we are about to write as plain text, such as a
// genre or a chapter title, once trimmed. It returns "" for a good value, or
// why it is refused: markup, a control or invisible character, no visible
// text, or more than limit characters.
func checkPlain(s string, limit int) string {
	s = strings.TrimSpace(s)
	for _, r := range s {
		switch {
		case r == '<' || r == '>':
			return fmt.Sprintf("contains %q; use plain text without markup", string(r))
		case unicode.IsControl(r):
			return fmt.Sprintf("contains the control character U+%04X", r)
		case isHidden(r) || r == '\u2028' || r == '\u2029':
			return fmt.Sprintf("contains the invisible character U+%04X", r)
		}
	}
	if !hasVisibleText(s) {
		return "has no visible text"
	}
	if n := utf8.RuneCountInString(s); n > limit {
		return fmt.Sprintf("is %d characters long; at most %d", n, limit)
	}
	return ""
}
