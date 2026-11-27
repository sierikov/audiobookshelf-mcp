package tools

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
	"github.com/sierikov/audiobookshelf-mcp/internal/service"
)

// spentNoteText is the note that comes with the spend, word for word.
const spentNoteText = "made by this server since it started; Google bills by tokens, so this is an estimate"

// hollyCoverArt is the result of making two covers of "Holly": the book has a
// current cover, the first new cover was made and the second was refused.
func hollyCoverArt() service.CoverArtResult {
	return service.CoverArtResult{
		Current: []service.CurrentCover{
			{ItemID: "li_1", Title: "Holly", Image: &abs.Image{Data: []byte("current"), MIMEType: "image/jpeg"}},
		},
		Covers: []service.NewCover{
			{
				ItemID: "li_1", Title: "Holly", Style: "moody watercolour", ImageID: "gen_0123456789abcdef",
				Preview: &abs.Image{Data: []byte("made"), MIMEType: "image/png"},
			},
			{ItemID: "li_1", Title: "Holly", Style: "pop art", Problem: "Google would not make this image (IMAGE_SAFETY)"},
		},
		Spent: service.Spend{Images: 1, EstimatedUSD: 0.0359},
	}
}

// generateArgs asks for one cover of li_1, in a style.
func generateArgs() map[string]any {
	return map[string]any{"covers": []any{map[string]any{"itemId": "li_1", "style": "moody watercolour"}}}
}

func TestAllTools_CoverArtOnlyWithAService(t *testing.T) {
	names := func(svcs Services) []string {
		tools := AllTools(svcs)
		out := make([]string, 0, len(tools))
		for _, st := range tools {
			out = append(out, st.Tool.Name)
		}
		return out
	}

	without := names(Services{})
	assert.NotContains(t, without, "generate_covers")
	assert.NotContains(t, without, "clean_cover")
	assert.Contains(t, without, "set_cover", "set_cover is always there: it says why a made cover can't be set")
	assert.Contains(t, without, "get_cover_prompt")

	with := names(Services{CoverArt: NewMockCoverArtService(t)})
	assert.Contains(t, with, "generate_covers")
	assert.Contains(t, with, "clean_cover")
	assert.Len(t, with, len(without)+2)
}

func TestCoverArtTools_Toolset(t *testing.T) {
	tools := CoverArtTools(NewMockCoverArtService(t))

	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		assert.Equal(t, toolsetCovers, tool.Toolset.ID, tool.Tool.Name)
		names = append(names, tool.Tool.Name)
	}
	assert.Equal(t, []string{"generate_covers", "clean_cover"}, names)
}

func TestCoverArtTools_Annotations(t *testing.T) {
	for _, tool := range CoverArtTools(NewMockCoverArtService(t)) {
		a := tool.Tool.Annotations
		require.NotNil(t, a, tool.Tool.Name)
		assert.Equal(t, openWorldCostAnnotations(), a, tool.Tool.Name)
		assert.False(t, a.ReadOnlyHint, "%s changes nothing stored, but is not a read: it costs money", tool.Tool.Name)
		require.NotNil(t, a.DestructiveHint, tool.Tool.Name)
		assert.False(t, *a.DestructiveHint, tool.Tool.Name)
		assert.False(t, a.IdempotentHint, "%s: a second call makes, and charges for, new images", tool.Tool.Name)
		require.NotNil(t, a.OpenWorldHint, tool.Tool.Name)
		assert.True(t, *a.OpenWorldHint, tool.Tool.Name)
	}
}

// artSchemaOf returns the definition of a made-cover tool, which AllTools has
// only with a service.
func artSchemaOf(t *testing.T, name string) *mcp.Tool {
	t.Helper()
	st := findTool(CoverArtTools(NewMockCoverArtService(t)), name)
	require.NotNil(t, st, name)
	return &st.Tool
}

// inputSchemaOf returns the input schema of a tool.
func inputSchemaOf(t *testing.T, tool *mcp.Tool) *jsonschema.Schema {
	t.Helper()
	schema, ok := tool.InputSchema.(*jsonschema.Schema)
	require.True(t, ok, tool.Name)
	return schema
}

func TestGenerateCovers_Schema(t *testing.T) {
	tool := artSchemaOf(t, "generate_covers")
	schema := inputSchemaOf(t, tool)

	assert.Equal(t, []string{"covers"}, schema.Required)
	assert.Len(t, schema.Properties, 5)

	covers := schema.Properties["covers"]
	assert.Equal(t, "array", covers.Type)
	assert.Equal(t, 1, *covers.MinItems)
	assert.Equal(t, service.MaxNewCovers, *covers.MaxItems)
	assert.Equal(t, 4, *covers.MaxItems)
	item := covers.Items
	assert.Equal(t, "object", item.Type)
	assert.Equal(t, []string{"itemId", "style"}, item.Required)
	assert.Len(t, item.Properties, 4)
	assert.Equal(t, "string", item.Properties["itemId"].Type)
	assert.Equal(t, "string", item.Properties["style"].Type)
	assert.Equal(t, new(service.MaxStyleLength), item.Properties["style"].MaxLength)
	assert.Equal(t, new(300), item.Properties["style"].MaxLength)
	assert.Equal(t, "string", item.Properties["premise"].Type)
	assert.Equal(t, new(service.MaxPremiseLength), item.Properties["premise"].MaxLength)
	assert.Equal(t, new(500), item.Properties["premise"].MaxLength)
	assert.NotContains(t, item.Required, "premise")
	title := item.Properties["title"]
	assert.Equal(t, "string", title.Type)
	assert.Equal(t, new(service.MaxTitleLength), title.MaxLength)
	assert.Equal(t, new(200), title.MaxLength)
	assert.NotContains(t, item.Required, "title")
	assert.Equal(t, "Title text to show on this cover instead of the book's title in Audiobookshelf, "+
		"such as another translation's title, at most 200 characters; the author stays", title.Description)
	for name, p := range item.Properties {
		assert.NotEmpty(t, p.Description, name)
	}
	assert.Contains(t, item.Properties["premise"].Description, "never plot outcomes")

	assertModelSchema(t, schema.Properties["model"])
	assert.NotContains(t, schema.Required, "model")

	titleText := schema.Properties["titleText"]
	assert.Equal(t, "boolean", titleText.Type)
	assert.JSONEq(t, "true", string(titleText.Default), "the title is on the cover unless titleText is false")

	for _, name := range []string{"referenceImageId", "referenceItemId"} {
		assert.Equal(t, "string", schema.Properties[name].Type, name)
		assert.NotEmpty(t, schema.Properties[name].Description, name)
	}
	assert.Equal(t, openWorldCostAnnotations(), tool.Annotations)

	// The schema, as the server checks a call with it.
	resolved, err := inputSchema(tool)
	require.NoError(t, err)
	withModel := func(model any) map[string]any {
		args := generateArgs()
		args["model"] = model
		return args
	}
	require.NoError(t, resolved.Validate(generateArgs()), "the model is optional")
	require.NoError(t, resolved.Validate(withModel("pro")))
	require.NoError(t, resolved.Validate(withModel("standard")))
	require.Error(t, resolved.Validate(withModel("turbo")))
	require.Error(t, resolved.Validate(withModel("Pro")))
	longTitle := generateArgs()
	longTitle["covers"].([]any)[0].(map[string]any)["title"] = strings.Repeat("a", service.MaxTitleLength+1)
	require.Error(t, resolved.Validate(longTitle))
	longTitle["covers"].([]any)[0].(map[string]any)["title"] = strings.Repeat("a", service.MaxTitleLength)
	require.NoError(t, resolved.Validate(longTitle))
}

// assertModelSchema checks the model property of generate_covers and
// clean_cover: pro or standard, pro by default, and what each costs.
func assertModelSchema(t *testing.T, model *jsonschema.Schema) {
	t.Helper()
	require.NotNil(t, model)
	assert.Equal(t, "string", model.Type)
	assert.Equal(t, []any{"pro", "standard"}, model.Enum)
	assert.JSONEq(t, `"pro"`, string(model.Default))
	assert.Equal(t, "pro: Nano Banana Pro, 2048 px, the best finish and lettering, about $0.16 a cover (the default). "+
		"standard: Nano Banana 2.1, 1024 px, about $0.07 a cover", model.Description)
}

// seenBeforeSavedText is what generate_covers and clean_cover say about who
// sees the images: the model does either way, the user may not, so a pick made
// from the model's description is not a pick made from the images.
const seenBeforeSavedText = "You see these images whether or not the user does, so a pick the user makes from your description is not seeing them: " +
	"unless the user says they can see the images (in Claude Desktop, by opening the tool result), ask them, and if they can't, don't set one."

func TestGenerateCovers_Description(t *testing.T) {
	desc := artSchemaOf(t, "generate_covers").Description

	for _, want := range []string{
		"Make new covers for books with Google's Nano Banana image models. " +
			"This costs money, billed to the user's Gemini key: about $0.16 a cover with model pro (the default) or about $0.07 with standard; at most 4 covers a call. " +
			"Before every call, tell the user how many covers you will make, with which model, what that costs, and the exact title and author text each cover will show, and get their yes.",
		"and get their yes. Use it only after showing what exists with search_covers (other editions and fan art too, when you can search the web) " +
			"and offering the user at least three options to pick from. Every cover is made square, as audiobook covers are. covers lists the covers to make",
		"For a series, propose about three styles in words, make one sample per style for one book, let the user pick, " +
			"then make the rest in the picked style with referenceImageId set to the picked sample, so the series stays consistent. " +
			"A series whose existing covers the user likes can use one of them as the reference with referenceItemId.",
		"premise is one or two sentences on the setting and the opening situation: never plot outcomes, twists, deaths or endings.",
		"The title and author are on the cover as text unless titleText is false; ask the user if unsure. " +
			"title prints other title text on a cover, such as another translation's title.",
		"The images are meant for the user: each book's current cover comes first, then the new ones, and the images note says which is which. " +
			"Set a new cover only after the user has seen it next to the old one and said yes. " +
			"If the user's app doesn't show these images, don't set them; use get_cover_prompt instead. " + seenBeforeSavedText,
		"If stopped is set, Google refused more images (a spending limit or quota): tell the user, and don't call again unless they say so.",
		"Made covers are kept in memory for an hour; to use one, call set_cover with its imageId.",
	} {
		assert.Contains(t, desc, want)
	}
}

func TestCleanCover_Description(t *testing.T) {
	desc := artSchemaOf(t, "clean_cover").Description

	assert.Contains(t, desc, "Clean a book's current cover with Google's Nano Banana image models, "+
		"for example to remove a watermark or a store label, keeping the rest. "+
		"This costs money, billed to the user's Gemini key: about $0.16 with model pro (the default) or about $0.07 with standard; "+
		"tell the user and get their yes first. "+
		"Offer a cleaning as one of at least three options after showing what exists with search_covers, not as the only fix. "+
		"instructions says what to change")
	// A cleaned cover is square, whatever the library says.
	assert.Contains(t, desc, `instructions says what to change, such as "remove the store label at the bottom"; `+
		"the rest of the cover is meant to stay as it is, and the cleaned cover is square, as audiobook covers are, "+
		"with the background extended so that nothing is cut off or stretched.")
	assert.NotContains(t, desc, "library's cover shape")
	// A cleaned cover is set the way a new one is: seen first, then by imageId.
	assert.Contains(t, desc, "set_cover")
	assert.Contains(t, desc, "imageId")
	assert.Contains(t, desc, "get_cover_prompt")
	assert.Contains(t, desc, "If stopped is set")
	// The same sentence as generate_covers, at the end.
	assert.True(t, strings.HasSuffix(desc, "don't call again unless they say so. "+seenBeforeSavedText), desc)
}

func TestGenerateCovers_PassesArguments(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		want service.GenerateRequest
	}{
		{"titleText defaults to true", generateArgs(), service.GenerateRequest{
			Covers:    []service.NewCoverRequest{{ItemID: "li_1", Style: "moody watercolour"}},
			TitleText: true,
		}},
		{"titleText false", map[string]any{"covers": generateArgs()["covers"], "titleText": false}, service.GenerateRequest{
			Covers:    []service.NewCoverRequest{{ItemID: "li_1", Style: "moody watercolour"}},
			TitleText: false,
		}},
		{"titleText true", map[string]any{"covers": generateArgs()["covers"], "titleText": true}, service.GenerateRequest{
			Covers:    []service.NewCoverRequest{{ItemID: "li_1", Style: "moody watercolour"}},
			TitleText: true,
		}},
		{"several covers with premises, in order", map[string]any{"covers": []any{
			map[string]any{"itemId": "li_1", "style": "noir", "premise": "A town in winter."},
			map[string]any{"itemId": "li_2", "style": "pop art"},
			map[string]any{"itemId": "li_1", "style": "watercolour", "premise": "A town in spring."},
		}}, service.GenerateRequest{
			Covers: []service.NewCoverRequest{
				{ItemID: "li_1", Style: "noir", Premise: "A town in winter."},
				{ItemID: "li_2", Style: "pop art"},
				{ItemID: "li_1", Style: "watercolour", Premise: "A town in spring."},
			},
			TitleText: true,
		}},
		{"a made cover as the reference", map[string]any{"covers": generateArgs()["covers"], "referenceImageId": "gen_0123456789abcdef"},
			service.GenerateRequest{
				Covers:           []service.NewCoverRequest{{ItemID: "li_1", Style: "moody watercolour"}},
				TitleText:        true,
				ReferenceImageID: "gen_0123456789abcdef",
			}},
		{"a book's cover as the reference", map[string]any{"covers": generateArgs()["covers"], "referenceItemId": "li_9"},
			service.GenerateRequest{
				Covers:          []service.NewCoverRequest{{ItemID: "li_1", Style: "moody watercolour"}},
				TitleText:       true,
				ReferenceItemID: "li_9",
			}},
		{"no model: passed on empty, the service's default", generateArgs(), service.GenerateRequest{
			Covers:    []service.NewCoverRequest{{ItemID: "li_1", Style: "moody watercolour"}},
			Model:     "",
			TitleText: true,
		}},
		{"the pro model", map[string]any{"covers": generateArgs()["covers"], "model": "pro"}, service.GenerateRequest{
			Covers:    []service.NewCoverRequest{{ItemID: "li_1", Style: "moody watercolour"}},
			Model:     service.CoverModelPro,
			TitleText: true,
		}},
		{"the standard model", map[string]any{"covers": generateArgs()["covers"], "model": "standard"}, service.GenerateRequest{
			Covers:    []service.NewCoverRequest{{ItemID: "li_1", Style: "moody watercolour"}},
			Model:     service.CoverModelStandard,
			TitleText: true,
		}},
		{"a title on some covers", map[string]any{"covers": []any{
			map[string]any{"itemId": "li_1", "style": "noir", "title": "Холли"},
			map[string]any{"itemId": "li_1", "style": "pop art"},
		}}, service.GenerateRequest{
			Covers: []service.NewCoverRequest{
				{ItemID: "li_1", Style: "noir", Title: "Холли"},
				{ItemID: "li_1", Style: "pop art"},
			},
			TitleText: true,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			art := NewMockCoverArtService(t)
			art.EXPECT().Generate(mock.Anything, tt.want).Return(hollyCoverArt(), nil)

			result := call(t, CoverArtTools(art), "generate_covers", tt.args)

			require.False(t, result.IsError, resultText(result))
		})
	}
}

func TestGenerateCovers_Result(t *testing.T) {
	art := NewMockCoverArtService(t)
	art.EXPECT().Generate(mock.Anything, mock.Anything).Return(hollyCoverArt(), nil)

	result := call(t, CoverArtTools(art), "generate_covers", generateArgs())

	require.False(t, result.IsError, resultText(result))
	require.Len(t, result.Content, 3, "the text block, then the current cover and the one new cover that was made")
	assertJSON(t, `{"untrusted":true,"source":"gemini",
		"current":[{"itemId":"li_1","title":"Holly","shown":true}],
		"covers":[
			{"index":1,"itemId":"li_1","title":"Holly","style":"moody watercolour","imageId":"gen_0123456789abcdef"},
			{"index":2,"itemId":"li_1","title":"Holly","style":"pop art","problem":"Google would not make this image (IMAGE_SAFETY)"}],
		"spent":{"images":1,"estimatedUSD":"0.04","note":"`+spentNoteText+`"},
		"images":"image 1 is the current cover of Holly; then new cover 1"}`, result)
	// The untrusted marker comes first, and stopped is left out when it is not set.
	assert.Equal(t, []string{"untrusted", "source", "current", "covers", "spent", "images"}, topLevelKeys(t, resultText(result)))
	assert.NotContains(t, resultText(result), "stopped")

	_, isText := result.Content[0].(*mcp.TextContent)
	assert.True(t, isText, "the text block comes first")
	images := imageBlocks(t, result)
	require.Len(t, images, 2)
	for i, want := range []struct{ data, mime string }{{"current", "image/jpeg"}, {"made", "image/png"}} {
		assert.Equal(t, []byte(want.data), images[i].Data, "image %d", i+1)
		assert.Equal(t, want.mime, images[i].MIMEType, "image %d", i+1)
		require.NotNil(t, images[i].Annotations, "image %d", i+1)
		assert.Equal(t, []mcp.Role{"user", "assistant"}, images[i].Annotations.Audience, "image %d is for the user too", i+1)
	}
	// Codex hides the images of a result that has structured content.
	assert.Nil(t, result.StructuredContent)
}

func TestGenerateCovers_ResultKeysOfEachEntry(t *testing.T) {
	art := NewMockCoverArtService(t)
	art.EXPECT().Generate(mock.Anything, mock.Anything).Return(hollyCoverArt(), nil)

	result := call(t, CoverArtTools(art), "generate_covers", generateArgs())

	var out struct {
		Current []json.RawMessage `json:"current"`
		Covers  []json.RawMessage `json:"covers"`
		Spent   json.RawMessage   `json:"spent"`
	}
	require.NoError(t, json.Unmarshal([]byte(resultText(result)), &out))
	assert.Equal(t, []string{"itemId", "title", "shown"}, topLevelKeys(t, string(out.Current[0])))
	assert.Equal(t, []string{"index", "itemId", "title", "style", "imageId"}, topLevelKeys(t, string(out.Covers[0])))
	assert.Equal(t, []string{"index", "itemId", "title", "style", "problem"}, topLevelKeys(t, string(out.Covers[1])))
	assert.Equal(t, []string{"images", "estimatedUSD", "note"}, topLevelKeys(t, string(out.Spent)))
}

func TestGenerateCovers_Stopped(t *testing.T) {
	const stopped = "Google refused more images (HTTP 429 RESOURCE_EXHAUSTED): Quota exceeded. This is a spending limit, a quota or used-up prepaid credit; nothing is retried. Covers already made can still be set."
	res := hollyCoverArt()
	res.Stopped = stopped
	res.Covers[1].Problem = "not made: the call stopped"
	art := NewMockCoverArtService(t)
	art.EXPECT().Generate(mock.Anything, mock.Anything).Return(res, nil)

	result := call(t, CoverArtTools(art), "generate_covers", generateArgs())

	assert.False(t, result.IsError, "a stopped call is a result, not an error: the covers made are in it")
	assertJSON(t, `{"untrusted":true,"source":"gemini",
		"current":[{"itemId":"li_1","title":"Holly","shown":true}],
		"covers":[
			{"index":1,"itemId":"li_1","title":"Holly","style":"moody watercolour","imageId":"gen_0123456789abcdef"},
			{"index":2,"itemId":"li_1","title":"Holly","style":"pop art","problem":"not made: the call stopped"}],
		"stopped":"`+stopped+`",
		"spent":{"images":1,"estimatedUSD":"0.04","note":"`+spentNoteText+`"},
		"images":"image 1 is the current cover of Holly; then new cover 1"}`, result)
	assert.Equal(t, []string{"untrusted", "source", "current", "covers", "stopped", "spent", "images"}, topLevelKeys(t, resultText(result)))
	assert.Len(t, imageBlocks(t, result), 2, "the covers made before the stop are shown")
}

func TestGenerateCovers_CurrentCoverThatIsNotShown(t *testing.T) {
	res := hollyCoverArt()
	res.Current = []service.CurrentCover{{ItemID: "li_1", Title: "Holly", Problem: "the current cover could not be read (HTTP 500)"}}
	art := NewMockCoverArtService(t)
	art.EXPECT().Generate(mock.Anything, mock.Anything).Return(res, nil)

	result := call(t, CoverArtTools(art), "generate_covers", generateArgs())

	assertJSON(t, `{"untrusted":true,"source":"gemini",
		"current":[{"itemId":"li_1","title":"Holly","shown":false,"problem":"the current cover could not be read (HTTP 500)"}],
		"covers":[
			{"index":1,"itemId":"li_1","title":"Holly","style":"moody watercolour","imageId":"gen_0123456789abcdef"},
			{"index":2,"itemId":"li_1","title":"Holly","style":"pop art","problem":"Google would not make this image (IMAGE_SAFETY)"}],
		"spent":{"images":1,"estimatedUSD":"0.04","note":"`+spentNoteText+`"},
		"images":"no current cover is shown; the image is new cover 1"}`, result)
	images := imageBlocks(t, result)
	require.Len(t, images, 1, "the new cover only: there is no current cover to show")
	assert.Equal(t, []byte("made"), images[0].Data)
}

func TestGenerateCovers_BookWithoutCover(t *testing.T) {
	res := hollyCoverArt()
	res.Current = []service.CurrentCover{{ItemID: "li_1", Title: "Holly"}} // no cover, no problem
	art := NewMockCoverArtService(t)
	art.EXPECT().Generate(mock.Anything, mock.Anything).Return(res, nil)

	result := call(t, CoverArtTools(art), "generate_covers", generateArgs())

	assert.Contains(t, resultText(result), `"current":[{"itemId":"li_1","title":"Holly","shown":false}]`, "no problem for a book without a cover")
}

func TestGenerateCovers_ImagesNote(t *testing.T) {
	cover := &abs.Image{Data: []byte("x"), MIMEType: "image/jpeg"}
	shownCurrent := func(id, title string) service.CurrentCover {
		return service.CurrentCover{ItemID: id, Title: title, Image: cover}
	}
	noCurrent := func(id, title string) service.CurrentCover { return service.CurrentCover{ItemID: id, Title: title} }
	made := func(id string) service.NewCover {
		return service.NewCover{ItemID: id, Title: "t", ImageID: "gen_" + id, Preview: cover}
	}
	refused := func(id string) service.NewCover {
		return service.NewCover{ItemID: id, Title: "t", Problem: "Google would not make this image"}
	}
	tests := []struct {
		name    string
		current []service.CurrentCover
		covers  []service.NewCover
		want    string
		images  int
	}{
		{"one current and one new", []service.CurrentCover{shownCurrent("a", "Holly")}, []service.NewCover{made("a")},
			"image 1 is the current cover of Holly; then new cover 1", 2},
		{"two currents and three new", []service.CurrentCover{shownCurrent("a", "Holly"), shownCurrent("b", "If It Bleeds")},
			[]service.NewCover{made("a"), made("b"), made("b")},
			"image 1 is the current cover of Holly; image 2 is the current cover of If It Bleeds; then new covers 1, 2 and 3 in that order", 5},
		{"new covers are named by their place, refused ones counted", []service.CurrentCover{shownCurrent("a", "Holly")},
			[]service.NewCover{refused("a"), made("a"), made("a")},
			"image 1 is the current cover of Holly; then new covers 2 and 3 in that order", 3},
		{"a book without a current cover is not counted", []service.CurrentCover{noCurrent("a", "Holly"), shownCurrent("b", "If It Bleeds")},
			[]service.NewCover{made("a"), made("b")},
			"image 1 is the current cover of If It Bleeds; then new covers 1 and 2 in that order", 3},
		{"current only", []service.CurrentCover{shownCurrent("a", "Holly")}, []service.NewCover{refused("a")},
			"image 1 is the current cover of Holly; no new cover has an image", 1},
		{"new only", []service.CurrentCover{noCurrent("a", "Holly")}, []service.NewCover{refused("a"), made("a")},
			"no current cover is shown; the image is new cover 2", 1},
		{"several new only", []service.CurrentCover{noCurrent("a", "Holly")}, []service.NewCover{made("a"), made("a")},
			"no current cover is shown; the images are new covers 1 and 2 in that order", 2},
		{"no images", []service.CurrentCover{noCurrent("a", "Holly")}, []service.NewCover{refused("a")},
			"no images: no current cover is shown and no new cover has an image", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			art := NewMockCoverArtService(t)
			art.EXPECT().Generate(mock.Anything, mock.Anything).Return(service.CoverArtResult{Current: tt.current, Covers: tt.covers}, nil)

			result := call(t, CoverArtTools(art), "generate_covers", generateArgs())

			require.False(t, result.IsError, resultText(result))
			var out struct {
				Images string `json:"images"`
			}
			require.NoError(t, json.Unmarshal([]byte(resultText(result)), &out))
			assert.Equal(t, tt.want, out.Images)
			assert.Len(t, imageBlocks(t, result), tt.images)
			assert.Len(t, result.Content, 1+tt.images, "the text block and the images, nothing else")
		})
	}
}

func TestGenerateCovers_SpentIsFormattedToTheCent(t *testing.T) {
	for usd, want := range map[float64]string{0: "0.00", 0.0359: "0.04", 0.0749: "0.07", 1.234: "1.23", 12: "12.00"} {
		res := hollyCoverArt()
		res.Spent.EstimatedUSD = usd
		art := NewMockCoverArtService(t)
		art.EXPECT().Generate(mock.Anything, mock.Anything).Return(res, nil)

		result := call(t, CoverArtTools(art), "generate_covers", generateArgs())

		assert.Contains(t, resultText(result), fmt.Sprintf(`"estimatedUSD":%q`, want), "%v", usd)
	}
}

func TestGenerateCovers_Error(t *testing.T) {
	art := NewMockCoverArtService(t)
	art.EXPECT().Generate(mock.Anything, mock.Anything).Return(service.CoverArtResult{}, fmt.Errorf("%w, not 5", service.ErrNewCoverCount))

	result := call(t, CoverArtTools(art), "generate_covers", generateArgs())

	assertToolError(t, "failed to make covers: "+service.ErrNewCoverCount.Error()+", not 5", result)
	assert.Len(t, result.Content, 1)
}

func TestGenerateCovers_MissingCovers(t *testing.T) {
	result := call(t, CoverArtTools(NewMockCoverArtService(t)), "generate_covers", map[string]any{})

	assertToolError(t, "missing required parameter: covers", result)
}

func TestCleanCover_Schema(t *testing.T) {
	tool := artSchemaOf(t, "clean_cover")
	schema := inputSchemaOf(t, tool)

	assert.Equal(t, []string{"itemId", "instructions"}, schema.Required)
	assert.Len(t, schema.Properties, 3)
	assert.Equal(t, "string", schema.Properties["itemId"].Type)
	instructions := schema.Properties["instructions"]
	assert.Equal(t, "string", instructions.Type)
	assert.Equal(t, new(service.MaxEditLength), instructions.MaxLength)
	assert.Equal(t, new(300), instructions.MaxLength)
	assert.NotEmpty(t, instructions.Description)
	assertModelSchema(t, schema.Properties["model"])
	assert.NotContains(t, schema.Required, "model")
	assert.Equal(t, openWorldCostAnnotations(), tool.Annotations)

	resolved, err := inputSchema(tool)
	require.NoError(t, err)
	args := map[string]any{"itemId": "li_1", "instructions": "remove the label"}
	require.NoError(t, resolved.Validate(args), "the model is optional")
	args["model"] = "standard"
	require.NoError(t, resolved.Validate(args))
	args["model"] = "turbo"
	require.Error(t, resolved.Validate(args))
}

func TestCleanCover_PassesTheModelThrough(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		want service.CoverModel
	}{
		{"no model: passed on empty, the service's default", map[string]any{"itemId": "li_1", "instructions": "remove the label"}, ""},
		{"pro", map[string]any{"itemId": "li_1", "instructions": "remove the label", "model": "pro"}, service.CoverModelPro},
		{"standard", map[string]any{"itemId": "li_1", "instructions": "remove the label", "model": "standard"}, service.CoverModelStandard},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			art := NewMockCoverArtService(t)
			art.EXPECT().Clean(mock.Anything, "li_1", "remove the label", tt.want).Return(service.CoverArtResult{}, nil)

			result := call(t, CoverArtTools(art), "clean_cover", tt.args)

			require.False(t, result.IsError, resultText(result))
		})
	}
}

func TestCleanCover_Result(t *testing.T) {
	art := NewMockCoverArtService(t)
	art.EXPECT().Clean(mock.Anything, "li_1", "remove the store label at the bottom", service.CoverModel("")).Return(service.CoverArtResult{
		Current: []service.CurrentCover{{ItemID: "li_1", Title: "Holly", Image: &abs.Image{Data: []byte("current"), MIMEType: "image/jpeg"}}},
		Covers: []service.NewCover{{
			ItemID: "li_1", Title: "Holly", ImageID: "gen_0123456789abcdef",
			Preview: &abs.Image{Data: []byte("cleaned"), MIMEType: "image/png"},
		}},
		Spent: service.Spend{Images: 3, EstimatedUSD: 0.1077},
	}, nil)

	result := call(t, CoverArtTools(art), "clean_cover", map[string]any{"itemId": "li_1", "instructions": "remove the store label at the bottom"})

	require.False(t, result.IsError, resultText(result))
	assertJSON(t, `{"untrusted":true,"source":"gemini",
		"current":[{"itemId":"li_1","title":"Holly","shown":true}],
		"covers":[{"index":1,"itemId":"li_1","title":"Holly","imageId":"gen_0123456789abcdef"}],
		"spent":{"images":3,"estimatedUSD":"0.11","note":"`+spentNoteText+`"},
		"images":"image 1 is the current cover of Holly; then new cover 1"}`, result)
	assert.NotContains(t, resultText(result), `"style"`, "a cleaned cover has no style")
	images := imageBlocks(t, result)
	require.Len(t, images, 2)
	assert.Equal(t, []byte("current"), images[0].Data, "the current cover first")
	assert.Equal(t, []byte("cleaned"), images[1].Data)
	assert.Nil(t, result.StructuredContent)
}

func TestCleanCover_Stopped(t *testing.T) {
	art := NewMockCoverArtService(t)
	art.EXPECT().Clean(mock.Anything, "li_1", "remove the label", service.CoverModel("")).Return(service.CoverArtResult{
		Current: []service.CurrentCover{{ItemID: "li_1", Title: "Holly", Image: &abs.Image{Data: []byte("current"), MIMEType: "image/jpeg"}}},
		Covers:  []service.NewCover{{ItemID: "li_1", Title: "Holly", Problem: "not made: the call stopped"}},
		Stopped: "Google refused more images (HTTP 402).",
	}, nil)

	result := call(t, CoverArtTools(art), "clean_cover", map[string]any{"itemId": "li_1", "instructions": "remove the label"})

	assert.False(t, result.IsError, resultText(result))
	assert.Contains(t, resultText(result), `"stopped":"Google refused more images (HTTP 402)."`)
	assert.Contains(t, resultText(result), `"estimatedUSD":"0.00"`)
	assert.Len(t, imageBlocks(t, result), 1, "the current cover only")
}

func TestCleanCover_Error(t *testing.T) {
	art := NewMockCoverArtService(t)
	art.EXPECT().Clean(mock.Anything, "li_1", "remove the label", service.CoverModel("")).
		Return(service.CoverArtResult{}, fmt.Errorf("item li_1: %w", service.ErrNoCoverToClean))

	result := call(t, CoverArtTools(art), "clean_cover", map[string]any{"itemId": "li_1", "instructions": "remove the label"})

	assertToolError(t, "failed to clean the cover: item li_1: "+service.ErrNoCoverToClean.Error(), result)
	assert.Len(t, result.Content, 1)
}

func TestCleanCover_MissingParams(t *testing.T) {
	tools := CoverArtTools(NewMockCoverArtService(t))

	assertToolError(t, "missing required parameter: itemId", call(t, tools, "clean_cover", map[string]any{"instructions": "remove the label"}))
	assertToolError(t, "missing required parameter: instructions", call(t, tools, "clean_cover", map[string]any{"itemId": "li_1"}))
}

// TestCoverArt_DescriptionsSayTheCost keeps the texts that guard the user's
// money: both tools say they cost money and ask for the user's yes first.
func TestCoverArt_DescriptionsSayTheCost(t *testing.T) {
	for _, tool := range CoverArtTools(NewMockCoverArtService(t)) {
		desc := tool.Tool.Description
		assert.Contains(t, desc, "This costs money", tool.Tool.Name)
		assert.Contains(t, desc, "get their yes", tool.Tool.Name)
	}
}
