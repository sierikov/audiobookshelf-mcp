package service

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
)

// exactSeconds converts the API's float seconds to a duration, rounding to
// the nearest nanosecond so values survive a round trip.
func exactSeconds(s float64) time.Duration {
	return time.Duration(math.Round(s * float64(time.Second)))
}

// secs renders a duration as the seconds a caller sent, for error texts.
func secs(d time.Duration) string {
	return strconv.FormatFloat(d.Seconds(), 'f', -1, 64) + "s"
}

// audibleProvider validates an Audible region and returns Audiobookshelf's
// provider name for it with the normalised region. An empty region means
// [DefaultAudibleRegion].
func audibleProvider(region string) (provider, normalised string, err error) {
	r := strings.ToLower(strings.TrimSpace(region))
	if r == "" {
		r = DefaultAudibleRegion
	}
	if !slices.Contains(AudibleRegions(), r) {
		return "", "", fmt.Errorf("%w %q (use one of: %s)", ErrUnknownRegion, region, strings.Join(AudibleRegions(), ", "))
	}
	if r == DefaultAudibleRegion {
		return "audible", r, nil
	}
	return "audible." + r, r, nil
}

// buildChapters checks a proposed chapter list against a book that ends at
// bookEnd and returns it with trimmed titles and computed ends. Each chapter
// ends where the next starts; the last ends with the book. The first failing
// chapter is reported as a [*ChapterError]. An empty list is valid.
func buildChapters(in []ChapterInput, bookEnd time.Duration) ([]Chapter, error) {
	out := make([]Chapter, len(in))
	for i, c := range in {
		if err := checkStart(in, i, bookEnd); err != nil {
			return nil, err
		}
		title := strings.TrimSpace(c.Title)
		if title == "" {
			return nil, &ChapterError{Chapter: i + 1, Reason: "has no title"}
		}
		end := bookEnd
		if i+1 < len(in) {
			end = in[i+1].Start
		}
		out[i] = Chapter{Start: c.Start, End: end, Title: title}
	}
	return out, nil
}

// checkStart checks where chapter i starts: the first at zero, every other
// after the one before it, and all of them before the end of the book.
func checkStart(in []ChapterInput, i int, bookEnd time.Duration) error {
	start := in[i].Start
	switch {
	case i == 0 && start != 0:
		return &ChapterError{Chapter: 1, Reason: fmt.Sprintf("starts at %s; the first chapter must start at 0s", secs(start))}
	case i > 0 && start <= in[i-1].Start:
		return &ChapterError{Chapter: i + 1, Reason: fmt.Sprintf("starts at %s, not after chapter %d (%s)", secs(start), i, secs(in[i-1].Start))}
	case start >= bookEnd:
		return &ChapterError{Chapter: i + 1, Reason: fmt.Sprintf("starts at %s, at or after the end of the book (%s)", secs(start), secs(bookEnd))}
	}
	return nil
}

// checkTitles returns the titles trimmed, or a [*ChapterError] for the first
// one that is blank.
func checkTitles(titles []string) ([]string, error) {
	out := make([]string, len(titles))
	for i, t := range titles {
		out[i] = strings.TrimSpace(t)
		if out[i] == "" {
			return nil, &ChapterError{Chapter: i + 1, Reason: "has no title"}
		}
	}
	return out, nil
}
