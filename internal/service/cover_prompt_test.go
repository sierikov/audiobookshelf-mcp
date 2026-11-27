package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
)

// joinLines joins the lines of a prompt as the prompt does.
func joinLines(lines ...string) string { return strings.Join(lines, "\n") }

var hollyBook = coverBook{Title: "Holly", Author: "Stephen King", Genres: []string{"Horror", "Thriller"}}

const (
	hollyHeading = `Front cover for the audiobook "Holly" by Stephen King.`
	hollyGenre   = "Genre: Horror, Thriller."
	hollyPremise = "Premise: A retired detective takes a missing-person case in a small town."
	hollyStyle   = "Style: Moody watercolour, muted palette."
	hollyText    = `Show the title "Holly" and the author "Stephen King" as clear, correctly spelled text that is part of the design.`
	// A book with no author has no "by" and no author in the text on the cover.
	noAuthorHeading = `Front cover for the audiobook "Holly".`
	noAuthorText    = `Show the title "Holly" as clear, correctly spelled text that is part of the design.`
	noText          = "No text, letters or numbers anywhere on the cover."
	noSpoilers      = "Show the setting, mood and premise only: no plot outcomes, twists, deaths or endings."
	noExtras        = "No logos, badges, labels, borders or watermarks."
	matchRef        = "Match the reference image's style, palette and typography only. Paint a new scene for this book, with its own subject, setting and composition, and none of the reference's text."
)

func TestNewCoverPrompt(t *testing.T) {
	const premise = "A retired detective takes a missing-person case in a small town."
	const style = "Moody watercolour, muted palette."
	tests := []struct {
		name          string
		book          coverBook
		premise       string
		titleText     bool
		withReference bool
		want          string
	}{
		{
			name: "every line", book: hollyBook, premise: premise, titleText: true,
			want: joinLines(hollyHeading, hollyGenre, hollyPremise, hollyStyle, hollyText, noSpoilers, noExtras),
		},
		{
			name: "with a reference image", book: hollyBook, premise: premise, titleText: true, withReference: true,
			want: joinLines(hollyHeading, hollyGenre, hollyPremise, hollyStyle, hollyText, noSpoilers, noExtras, matchRef),
		},
		{
			name: "no text on the cover", book: hollyBook, premise: premise, titleText: false,
			want: joinLines(hollyHeading, hollyGenre, hollyPremise, hollyStyle, noText, noSpoilers, noExtras),
		},
		{
			name: "no text, with a reference image", book: hollyBook, premise: premise, titleText: false, withReference: true,
			want: joinLines(hollyHeading, hollyGenre, hollyPremise, hollyStyle, noText, noSpoilers, noExtras, matchRef),
		},
		{
			name: "no genres", book: coverBook{Title: "Holly", Author: "Stephen King"}, premise: premise, titleText: true,
			want: joinLines(hollyHeading, hollyPremise, hollyStyle, hollyText, noSpoilers, noExtras),
		},
		{
			name: "no premise", book: hollyBook, titleText: true,
			want: joinLines(hollyHeading, hollyGenre, hollyStyle, hollyText, noSpoilers, noExtras),
		},
		{
			name: "no genres and no premise", book: coverBook{Title: "Holly", Author: "Stephen King"}, titleText: true,
			want: joinLines(hollyHeading, hollyStyle, hollyText, noSpoilers, noExtras),
		},
		{
			name: "no author", book: coverBook{Title: "Holly", Genres: []string{"Horror", "Thriller"}}, premise: premise, titleText: true,
			want: joinLines(noAuthorHeading, hollyGenre, hollyPremise, hollyStyle, noAuthorText, noSpoilers, noExtras),
		},
		{
			name: "an author of only spaces", book: coverBook{Title: "Holly", Author: " \t ", Genres: []string{"Horror", "Thriller"}}, premise: premise, titleText: true,
			want: joinLines(noAuthorHeading, hollyGenre, hollyPremise, hollyStyle, noAuthorText, noSpoilers, noExtras),
		},
		{
			name: "no author and no text on the cover", book: coverBook{Title: "Holly", Genres: []string{"Horror", "Thriller"}}, premise: premise, titleText: false,
			want: joinLines(noAuthorHeading, hollyGenre, hollyPremise, hollyStyle, noText, noSpoilers, noExtras),
		},
		{
			name: "no author, with a reference image", book: coverBook{Title: "Holly"}, titleText: true, withReference: true,
			want: joinLines(noAuthorHeading, hollyStyle, noAuthorText, noSpoilers, noExtras, matchRef),
		},
		{
			name: "one genre", book: coverBook{Title: "Holly", Author: "Stephen King", Genres: []string{"Horror"}}, titleText: true,
			want: joinLines(hollyHeading, "Genre: Horror.", hollyStyle, hollyText, noSpoilers, noExtras),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, newCoverPrompt(tt.book, style, tt.premise, tt.titleText, tt.withReference))
		})
	}
}

func TestNewCoverPrompt_QuotesAreKept(t *testing.T) {
	book := coverBook{Title: `Ender's "Game"`, Author: "O. S. Card"}

	got := newCoverPrompt(book, "noir", "", true, false)

	assert.Contains(t, got, `Front cover for the audiobook "Ender's "Game"" by O. S. Card.`, "the title as stored, in plain quotes")
	assert.Contains(t, got, `Show the title "Ender's "Game"" and the author "O. S. Card" as clear`)
}

func TestEditCoverPrompt(t *testing.T) {
	want := joinLines(
		"Edit this audiobook cover: remove the store label at the bottom",
		"Keep everything else exactly as it is: the artwork, the title and author text, the colours and the layout. Fill any removed area so it matches its surroundings.",
		"Make the cover square, extending the background so that nothing is cut off or stretched.",
	)

	assert.Equal(t, want, editCoverPrompt("remove the store label at the bottom"))
}

// promptItem is the book Holly.
func promptItem() abs.LibraryItem {
	return libraryItem("li_1", abs.MediaTypeBook, abs.BookMedia{
		Metadata: abs.BookMetadata{Title: "Holly", AuthorName: "Stephen King", Genres: []string{"Horror", "Thriller"}},
	})
}

func TestPrompt_NewCover(t *testing.T) {
	tests := []struct {
		name      string
		titleText bool
	}{
		{"with the title on the cover", true},
		{"no text on the cover", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A strict mock: only the item is read, never the library.
			api := NewMockCoversAPI(t)
			api.EXPECT().Item(mock.Anything, "li_1").Return(promptItem(), nil)

			got, err := NewCovers(api, NewMockImageFetcher(t), "https://abs.example/").Prompt(t.Context(), PromptRequest{
				ItemID: "li_1", Style: "Moody watercolour, muted palette.",
				Premise: "A retired detective takes a missing-person case in a small town.", TitleText: tt.titleText,
			})

			require.NoError(t, err)
			wantPrompt := newCoverPrompt(hollyBook, "Moody watercolour, muted palette.",
				"A retired detective takes a missing-person case in a small town.", tt.titleText, false) + "\nThe cover is square."
			assert.Equal(t, CoverPrompt{
				ItemID: "li_1", Title: "Holly", Prompt: wantPrompt, AspectRatio: "1:1",
				BookPage: "https://abs.example/item/li_1",
			}, got)
			assert.False(t, got.Edit)
			assert.NotContains(t, got.Prompt, "reference image", "the outside app has no reference image to match")
			assert.True(t, strings.HasSuffix(got.Prompt, "\nThe cover is square."), "the shape is the last line")
		})
	}
}

func TestPrompt_NewCoverTrimsItsInput(t *testing.T) {
	api := NewMockCoversAPI(t)
	api.EXPECT().Item(mock.Anything, "li_1").Return(promptItem(), nil)

	got, err := NewCovers(api, NewMockImageFetcher(t), "https://abs.example").Prompt(t.Context(), PromptRequest{
		ItemID: "li_1", Style: "  noir \n", Premise: "\t A town in winter. ", TitleText: true,
	})

	require.NoError(t, err)
	assert.Contains(t, got.Prompt, "\nPremise: A town in winter.\nStyle: noir\n")
}

func TestPrompt_ATitleTakesTheLibraryTitlesPlace(t *testing.T) {
	api := NewMockCoversAPI(t)
	api.EXPECT().Item(mock.Anything, "li_1").Return(promptItem(), nil)

	got, err := NewCovers(api, NewMockImageFetcher(t), "https://abs.example").Prompt(t.Context(), PromptRequest{
		ItemID: "li_1", Style: "noir", Title: " \tХолли\n", TitleText: true,
	})

	require.NoError(t, err)
	assert.Contains(t, got.Prompt, `Front cover for the audiobook "Холли" by Stephen King.`, "the heading")
	assert.Contains(t, got.Prompt, `Show the title "Холли" and the author "Stephen King" as clear`, "the title line")
	assert.NotContains(t, got.Prompt, "Holly")
	assert.Equal(t, "Holly", got.Title, "the result keeps the library title")
}

func TestPrompt_ATitleIsNotUsedForACleaning(t *testing.T) {
	api := NewMockCoversAPI(t)
	api.EXPECT().Item(mock.Anything, "li_1").Return(promptItem(), nil)

	got, err := NewCovers(api, NewMockImageFetcher(t), "https://abs.example").Prompt(t.Context(), PromptRequest{
		ItemID: "li_1", Instructions: "remove the store label", Title: "Холли",
	})

	require.NoError(t, err)
	assert.Equal(t, editCoverPrompt("remove the store label"), got.Prompt)
	assert.True(t, got.Edit)
}

func TestPrompt_BookPage(t *testing.T) {
	tests := []struct {
		serverURL string
		want      string
	}{
		{"https://abs.example/", "https://abs.example/item/li_1"},
		{"https://abs.example", "https://abs.example/item/li_1"},
		{"https://abs.example///", "https://abs.example/item/li_1"},
		{"http://192.168.1.5:13378", "http://192.168.1.5:13378/item/li_1"},
		{"https://host.example/audiobookshelf/", "https://host.example/audiobookshelf/item/li_1"},
	}
	for _, tt := range tests {
		t.Run(tt.serverURL, func(t *testing.T) {
			api := NewMockCoversAPI(t)
			api.EXPECT().Item(mock.Anything, "li_1").Return(promptItem(), nil)

			got, err := NewCovers(api, NewMockImageFetcher(t), tt.serverURL).Prompt(t.Context(), PromptRequest{
				ItemID: "li_1", Instructions: "remove the store label",
			})

			require.NoError(t, err)
			assert.Equal(t, tt.want, got.BookPage)
		})
	}
}

func TestPrompt_BookPageEscapesTheItemID(t *testing.T) {
	api := NewMockCoversAPI(t)
	api.EXPECT().Item(mock.Anything, "li 1/../x").Return(promptItem(), nil)

	got, err := NewCovers(api, NewMockImageFetcher(t), "https://abs.example").Prompt(t.Context(), PromptRequest{
		ItemID: "li 1/../x", Instructions: "remove the store label",
	})

	require.NoError(t, err)
	assert.Equal(t, "https://abs.example/item/li%201%2F..%2Fx", got.BookPage)
}

func TestPrompt_Clean(t *testing.T) {
	api := NewMockCoversAPI(t) // no Library read: a strict mock fails the test on one
	api.EXPECT().Item(mock.Anything, "li_1").Return(promptItem(), nil)

	got, err := NewCovers(api, NewMockImageFetcher(t), "https://abs.example/").Prompt(t.Context(), PromptRequest{
		ItemID: "li_1", Instructions: "  remove the store label at the bottom ", TitleText: true,
	})

	require.NoError(t, err)
	assert.Equal(t, CoverPrompt{
		ItemID: "li_1", Title: "Holly", Prompt: editCoverPrompt("remove the store label at the bottom"),
		AspectRatio: "1:1", BookPage: "https://abs.example/item/li_1", Edit: true,
	}, got)
	assert.True(t, strings.HasSuffix(got.Prompt,
		"\nMake the cover square, extending the background so that nothing is cut off or stretched."), "the square line is the last line")
	assert.NotContains(t, got.Prompt, "The cover is", "the edit line takes the place of the shape line")
	assert.Equal(t, 2, strings.Count(got.Prompt, "\n"), "three lines: the instructions, the keep line and the square line")
}

func TestPrompt_RefusesBadInput(t *testing.T) {
	tests := []struct {
		name    string
		req     PromptRequest
		wantErr error
		want    []string // in the error text
	}{
		{"both", PromptRequest{ItemID: "li_1", Style: "noir", Instructions: "remove the label"}, ErrStyleOrInstructions, nil},
		{"neither", PromptRequest{ItemID: "li_1"}, ErrStyleOrInstructions, nil},
		{"neither, only spaces", PromptRequest{ItemID: "li_1", Style: "  ", Instructions: "\n"}, ErrStyleOrInstructions, nil},
		{"a premise alone", PromptRequest{ItemID: "li_1", Premise: "A town in winter."}, ErrStyleOrInstructions, nil},
		{"style too long", PromptRequest{ItemID: "li_1", Style: strings.Repeat("a", MaxStyleLength+1)}, nil,
			[]string{"style", "301 characters", "at most 300"}},
		{"style with markup", PromptRequest{ItemID: "li_1", Style: "<b>noir</b>"}, nil, []string{"style", `"<"`}},
		{"style with a control character", PromptRequest{ItemID: "li_1", Style: "no\x00ir"}, nil, []string{"style", "control character"}},
		{"premise too long", PromptRequest{ItemID: "li_1", Style: "noir", Premise: strings.Repeat("a", MaxPremiseLength+1)}, nil,
			[]string{"premise", "501 characters", "at most 500"}},
		{"premise with markup", PromptRequest{ItemID: "li_1", Style: "noir", Premise: "A <script> town"}, nil, []string{"premise", `"<"`}},
		{"title too long", PromptRequest{ItemID: "li_1", Style: "noir", Title: strings.Repeat("a", MaxTitleLength+1)}, nil,
			[]string{"title", "201 characters", "at most 200"}},
		{"title with markup", PromptRequest{ItemID: "li_1", Style: "noir", Title: "<b>Holly</b>"}, nil, []string{"title", `"<"`}},
		{"instructions too long", PromptRequest{ItemID: "li_1", Instructions: strings.Repeat("a", MaxEditLength+1)}, nil,
			[]string{"instructions", "301 characters", "at most 300"}},
		{"instructions with markup", PromptRequest{ItemID: "li_1", Instructions: "remove <img src=x>"}, nil, []string{"instructions", `"<"`}},
		{"instructions with an invisible character", PromptRequest{ItemID: "li_1", Instructions: "remove\u200b the label"}, nil,
			[]string{"instructions", "invisible character"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A strict mock: the input is refused before any call.
			_, err := NewCovers(NewMockCoversAPI(t), NewMockImageFetcher(t), "https://abs.example").Prompt(t.Context(), tt.req)

			require.Error(t, err)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			}
			for _, want := range tt.want {
				assert.Contains(t, err.Error(), want)
			}
		})
	}
}

func TestPrompt_AcceptsInputAtTheLimits(t *testing.T) {
	api := NewMockCoversAPI(t)
	api.EXPECT().Item(mock.Anything, "li_1").Return(promptItem(), nil)

	_, err := NewCovers(api, NewMockImageFetcher(t), "https://abs.example").Prompt(t.Context(), PromptRequest{
		ItemID: "li_1", Style: strings.Repeat("я", MaxStyleLength), Premise: strings.Repeat("я", MaxPremiseLength),
		Title: strings.Repeat("я", MaxTitleLength),
	})

	require.NoError(t, err, "the limits count characters, not bytes")
}

func TestPrompt_NamesTheFieldInTheError(t *testing.T) {
	_, err := NewCovers(NewMockCoversAPI(t), NewMockImageFetcher(t), "https://abs.example").Prompt(t.Context(), PromptRequest{
		ItemID: "li_1", Style: "<b>noir</b>",
	})

	require.EqualError(t, err, `style ("<b>noir</b>") contains "<"; use plain text without markup`)
}

func TestPrompt_PodcastIsRefused(t *testing.T) {
	api := NewMockCoversAPI(t)
	api.EXPECT().Item(mock.Anything, "li_1").Return(podcastItem("li_1", "Show", "Host", 3), nil)

	_, err := NewCovers(api, NewMockImageFetcher(t), "https://abs.example").Prompt(t.Context(), PromptRequest{ItemID: "li_1", Style: "noir"})

	require.ErrorIs(t, err, ErrBooksOnly)
}

func TestPrompt_PodcastIsRefusedForACleaningToo(t *testing.T) {
	api := NewMockCoversAPI(t)
	api.EXPECT().Item(mock.Anything, "li_1").Return(podcastItem("li_1", "Show", "Host", 3), nil)

	_, err := NewCovers(api, NewMockImageFetcher(t), "https://abs.example").Prompt(t.Context(), PromptRequest{ItemID: "li_1", Instructions: "remove the label"})

	require.ErrorIs(t, err, ErrBooksOnly)
}

func TestPrompt_ItemErrorFails(t *testing.T) {
	api := NewMockCoversAPI(t)
	api.EXPECT().Item(mock.Anything, "li_1").Return(abs.LibraryItem{}, errAPI)

	_, err := NewCovers(api, NewMockImageFetcher(t), "https://abs.example").Prompt(t.Context(), PromptRequest{ItemID: "li_1", Style: "noir"})

	require.ErrorIs(t, err, errAPI)
}
