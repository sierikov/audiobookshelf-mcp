package service

import (
	"errors"
	"strings"
)

// Limits on the text a cover prompt is made from, in characters.
const (
	// MaxStyleLength is the most characters the style of a new cover may have.
	MaxStyleLength = 300
	// MaxPremiseLength is the most characters the premise of a new cover may
	// have.
	MaxPremiseLength = 500
	// MaxTitleLength is the most characters the title text of a new cover may
	// have, when it is not the book's title.
	MaxTitleLength = 200
	// MaxEditLength is the most characters the instructions for cleaning a
	// cover may have.
	MaxEditLength = 300
)

// ErrStyleOrInstructions is the error for a request that gives both a style
// and instructions, or neither: one is a new cover, the other a cleaning.
var ErrStyleOrInstructions = errors.New("give style for a new cover or instructions to clean the current one, not both")

// PromptRequest asks [Covers.Prompt] for a prompt. Exactly one of Style and
// Instructions must be given: Style for a new cover, Instructions for cleaning
// the current one. Premise, Title and TitleText belong to a new cover and are
// not used for a cleaning. Title is the title text to show on the cover in
// place of the book's title, and may be empty. TitleText says whether the
// title and the author are on the cover as text.
type PromptRequest struct {
	ItemID       string
	Style        string
	Premise      string
	Title        string
	Instructions string
	TitleText    bool
}

// CoverPrompt is a prompt to paste into an image app, with the book it is
// for. Edit says that the prompt cleans the book's current cover, which the
// user attaches, rather than making a new one. AspectRatio is the shape the
// prompt asks for, always [squareRatio]: audiobook covers are square. BookPage
// is the address of the book's page in Audiobookshelf, where the user finds the
// current cover and uploads the result.
type CoverPrompt struct {
	ItemID      string
	Title       string
	Prompt      string
	AspectRatio string
	BookPage    string
	Edit        bool
}

// coverBook is what a new cover prompt says about the book, as stored.
type coverBook struct {
	Title  string
	Author string
	Genres []string
}

// newCoverPrompt writes the prompt for a new cover of b, one line each for the
// book, its genres, the premise, the style, the text on the cover, what to
// keep out of the picture and, when withReference, the reference image to
// match. A line with nothing to say is left out. The title and the author are
// shown as text on the cover when titleText is set, and the cover has no
// text at all otherwise. A book with no author is named by its title alone.
func newCoverPrompt(b coverBook, style, premise string, titleText, withReference bool) string {
	title, author := `"`+b.Title+`"`, `"`+b.Author+`"` // in plain quotes, which %q would escape inside the text
	byAuthor := strings.TrimSpace(b.Author) != ""
	heading := "Front cover for the audiobook " + title
	if byAuthor {
		heading += " by " + b.Author
	}
	lines := []string{heading + "."}
	if len(b.Genres) > 0 {
		lines = append(lines, "Genre: "+strings.Join(b.Genres, ", ")+".")
	}
	if premise != "" {
		lines = append(lines, "Premise: "+premise)
	}
	if style != "" {
		lines = append(lines, "Style: "+style)
	}
	switch {
	case titleText && byAuthor:
		lines = append(lines, "Show the title "+title+" and the author "+author+" as clear, correctly spelled text that is part of the design.")
	case titleText:
		lines = append(lines, "Show the title "+title+" as clear, correctly spelled text that is part of the design.")
	default:
		lines = append(lines, "No text, letters or numbers anywhere on the cover.")
	}
	lines = append(lines,
		"Show the setting, mood and premise only: no plot outcomes, twists, deaths or endings.",
		"No logos, badges, labels, borders or watermarks.",
	)
	if withReference {
		lines = append(lines, "Match the reference image's style, palette and typography only. Paint a new scene for this book, with its own subject, setting and composition, and none of the reference's text.")
	}
	return strings.Join(lines, "\n")
}

// editCoverPrompt writes the prompt that has an image app change a book's
// current cover as instructions say, keeping the rest, and make it square: a
// last line asks for that, with the background extended so that nothing is cut
// off or stretched.
func editCoverPrompt(instructions string) string {
	return "Edit this audiobook cover: " + instructions + "\n" +
		"Keep everything else exactly as it is: the artwork, the title and author text, the colours and the layout. " +
		"Fill any removed area so it matches its surroundings.\n" +
		"Make the cover square, extending the background so that nothing is cut off or stretched."
}

// Audiobook covers are square, so every cover made here is.
const (
	// squareRatio is the shape asked of the image model, and the aspect ratio
	// get_cover_prompt reports: 1:1.
	squareRatio = "1:1"
	// squareLine is the last line of a prompt for an app outside this server: no
	// API setting carries the shape of the cover there, so the prompt says it.
	squareLine = "The cover is square."
)
