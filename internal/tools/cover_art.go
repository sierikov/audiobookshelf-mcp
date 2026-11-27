package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sierikov/audiobookshelf-mcp/internal/service"
)

// CoverArtTools returns the tools that make covers with an image model, which
// cost money: generate_covers and clean_cover. They are in the covers toolset,
// and AllTools offers them only when the server has a Gemini API key.
func CoverArtTools(art CoverArtService) []ServerTool {
	return []ServerTool{
		generateCovers(art),
		cleanCover(art),
	}
}

// spentNote says what the spend in a result counts and how sure it is.
const spentNote = "made by this server since it started; Google bills by tokens, so this is an estimate"

// currentCoverOutput is a book's cover as it was before, which comes first
// among the images when Shown. Problem is there only when the cover could not
// be read, and then Shown is false as for a book without a cover.
type currentCoverOutput struct {
	ItemID  string `json:"itemId"`
	Title   string `json:"title"`
	Shown   bool   `json:"shown"`
	Problem string `json:"problem,omitempty"`
}

// newCoverOutput is a cover that was asked for, numbered from 1 in the order
// asked. ImageID is there for one that was made, to give set_cover, and Problem
// for one that was not. Style is empty for a cleaned cover.
type newCoverOutput struct {
	Index   int    `json:"index"`
	ItemID  string `json:"itemId"`
	Title   string `json:"title"`
	Style   string `json:"style,omitempty"`
	ImageID string `json:"imageId,omitempty"`
	Problem string `json:"problem,omitempty"`
}

// spentOutput is what this server has made since it started and what that
// cost, in US dollars to the cent, as text.
type spentOutput struct {
	Images       int    `json:"images"`
	EstimatedUSD string `json:"estimatedUSD"`
	Note         string `json:"note"`
}

// coverArtOutput is the text block of generate_covers and clean_cover. Like
// search_covers' it starts with the untrusted marker, since what an image model
// makes is not ours to vouch for. Stopped is there only when Google refused
// more images. Images says which image block is which, because the blocks
// follow this one without a label of their own.
type coverArtOutput struct {
	Untrusted bool                 `json:"untrusted"`
	Source    string               `json:"source"`
	Current   []currentCoverOutput `json:"current"`
	Covers    []newCoverOutput     `json:"covers"`
	Stopped   string               `json:"stopped,omitempty"`
	Spent     spentOutput          `json:"spent"`
	Images    string               `json:"images"`
}

// coverArtImagesNote says which image block is which: the current covers that
// are shown come first, numbered in order, then the new covers that have an
// image, by the number they have in the result. currentTitles are the titles of
// the books whose current cover is shown, in order.
func coverArtImagesNote(currentTitles []string, shown []int) string {
	newCovers := func() string { return numbered("new cover", "new covers", shown) }
	switch {
	case len(currentTitles) == 0 && len(shown) == 0:
		return "no images: no current cover is shown and no new cover has an image"
	case len(currentTitles) == 0 && len(shown) == 1:
		return "no current cover is shown; the image is " + newCovers()
	case len(currentTitles) == 0:
		return "no current cover is shown; the images are " + newCovers()
	}
	parts := make([]string, 0, len(currentTitles)+1)
	for i, title := range currentTitles {
		parts = append(parts, fmt.Sprintf("image %d is the current cover of %s", i+1, title))
	}
	if len(shown) == 0 {
		return strings.Join(append(parts, "no new cover has an image"), "; ")
	}
	return strings.Join(append(parts, "then "+newCovers()), "; ")
}

// coverArtResult builds the result of generate_covers and clean_cover: the
// text block, then the current covers that can be shown, one per book, then the
// new covers that were made, so the images follow the order the text gives. It
// sets no structured content: Codex hides the images of a result that has some.
func coverArtResult(res service.CoverArtResult) (*mcp.CallToolResult, error) {
	out := coverArtOutput{
		Untrusted: true,
		Source:    service.SourceGemini,
		Current:   make([]currentCoverOutput, 0, len(res.Current)),
		Covers:    make([]newCoverOutput, 0, len(res.Covers)),
		Stopped:   res.Stopped,
		Spent:     spentOutput{Images: res.Spent.Images, EstimatedUSD: fmt.Sprintf("%.2f", res.Spent.EstimatedUSD), Note: spentNote},
	}
	var images []mcp.Content
	var currentTitles []string
	for _, c := range res.Current {
		out.Current = append(out.Current, currentCoverOutput{ItemID: c.ItemID, Title: c.Title, Shown: c.Image != nil, Problem: c.Problem})
		if c.Image != nil {
			currentTitles = append(currentTitles, c.Title)
			images = append(images, coverImage(c.Image.Data, c.Image.MIMEType))
		}
	}
	var shown []int
	for i, c := range res.Covers {
		out.Covers = append(out.Covers, newCoverOutput{
			Index: i + 1, ItemID: c.ItemID, Title: c.Title, Style: c.Style, ImageID: c.ImageID, Problem: c.Problem,
		})
		if c.Preview != nil {
			shown = append(shown, i+1)
			images = append(images, coverImage(c.Preview.Data, c.Preview.MIMEType))
		}
	}
	out.Images = coverArtImagesNote(currentTitles, shown)

	data, err := marshalJSON(out)
	if err != nil {
		return errResult("failed to marshal result: " + err.Error())
	}
	content := append([]mcp.Content{&mcp.TextContent{Text: string(data)}}, images...)
	return &mcp.CallToolResult{Content: content}, nil
}

// coverModelSchema is the model property of generate_covers and clean_cover.
// Leaving it out means pro, as the schema says.
func coverModelSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:        "string",
		Enum:        []any{string(service.CoverModelPro), string(service.CoverModelStandard)},
		Default:     json.RawMessage(`"pro"`),
		Description: "pro: Nano Banana Pro, 2048 px, the best finish and lettering, about $0.16 a cover (the default). standard: Nano Banana 2.1, 1024 px, about $0.07 a cover",
	}
}

// coverTitleSchema is the title property of a new cover, in generate_covers and
// in get_cover_prompt. The schema has the limit when maxLength is set, as
// generate_covers' does for the style and the premise; get_cover_prompt leaves
// it to the service, which names the field that is too long.
func coverTitleSchema(maxLength *int) *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:        "string",
		Description: fmt.Sprintf("Title text to show on this cover instead of the book's title in Audiobookshelf, such as another translation's title, at most %d characters; the author stays", service.MaxTitleLength),
		MaxLength:   maxLength,
	}
}

// generateCoversFailure is the start of every error result of generate_covers.
const generateCoversFailure = "failed to make covers: "

func generateCovers(art CoverArtService) ServerTool {
	return ServerTool{
		Toolset: metaCovers,
		Tool: mcp.Tool{
			Name: "generate_covers",
			Description: `Make new covers for books with Google's Nano Banana image models. This costs money, billed to the user's Gemini key: about $0.16 a cover with model pro (the default) or about $0.07 with standard; at most 4 covers a call. Before every call, tell the user how many covers you will make, with which model, what that costs, and the exact title and author text each cover will show, and get their yes. Use it only after showing what exists with search_covers (other editions and fan art too, when you can search the web) and offering the user at least three options to pick from. Every cover is made square, as audiobook covers are. ` +
				`covers lists the covers to make, each for one book in one style; give the same book more than once to try several styles. ` +
				`For a series, propose about three styles in words, make one sample per style for one book, let the user pick, then make the rest in the picked style with referenceImageId set to the picked sample, so the series stays consistent. A series whose existing covers the user likes can use one of them as the reference with referenceItemId. ` +
				`premise is one or two sentences on the setting and the opening situation: never plot outcomes, twists, deaths or endings. ` +
				`The title and author are on the cover as text unless titleText is false; ask the user if unsure. title prints other title text on a cover, such as another translation's title. ` +
				`The result is a JSON text block followed by images; spent says how many images this server has made since it started and what they cost, as an estimate. ` +
				`The images are meant for the user: each book's current cover comes first, then the new ones, and the images note says which is which. Set a new cover only after the user has seen it next to the old one and said yes. If the user's app doesn't show these images, don't set them; use get_cover_prompt instead. You see these images whether or not the user does, so a pick the user makes from your description is not seeing them: unless the user says they can see the images (in Claude Desktop, by opening the tool result), ask them, and if they can't, don't set one. ` +
				`If stopped is set, Google refused more images (a spending limit or quota): tell the user, and don't call again unless they say so. ` +
				`Made covers are kept in memory for an hour; to use one, call set_cover with its imageId.`,
			Annotations: openWorldCostAnnotations(),
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"covers": {
						Type:        "array",
						Description: fmt.Sprintf("The covers to make, at most %d: one book in several styles, or several books in one style", service.MaxNewCovers),
						MinItems:    new(1),
						MaxItems:    new(service.MaxNewCovers),
						Items: &jsonschema.Schema{
							Type: "object",
							Properties: map[string]*jsonschema.Schema{
								"itemId": itemIDSchema(),
								"style": {
									Type:        "string",
									Description: fmt.Sprintf("Style of the cover, such as \"moody watercolour, muted palette\", at most %d characters", service.MaxStyleLength),
									MaxLength:   new(service.MaxStyleLength),
								},
								"premise": {
									Type:        "string",
									Description: fmt.Sprintf("One or two sentences on the setting and the opening situation, never plot outcomes, twists, deaths or endings, at most %d characters", service.MaxPremiseLength),
									MaxLength:   new(service.MaxPremiseLength),
								},
								"title": coverTitleSchema(new(service.MaxTitleLength)),
							},
							Required: []string{"itemId", "style"},
						},
					},
					"model": coverModelSchema(),
					"titleText": {
						Type:        "boolean",
						Description: "Show the title and the author as text on the covers (default true); false for no text at all",
						Default:     json.RawMessage("true"),
					},
					"referenceImageId": {
						Type:        "string",
						Description: "The imageId of a cover made earlier, in the last hour, whose style every cover here matches. Give this or referenceItemId, not both",
					},
					"referenceItemId": {
						Type:        "string",
						Description: "The itemId of a book whose current cover every cover here matches in style. Give this or referenceImageId, not both",
					},
				},
				Required: []string{"covers"},
			},
		},
		Handler: func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, error) {
			raw, err := RequiredParam[[]any](args, "covers")
			if err != nil {
				return errResult(err.Error())
			}
			covers := make([]service.NewCoverRequest, 0, len(raw))
			for _, entry := range raw {
				obj, _ := entry.(map[string]any)
				strs := optionalStrings(obj, "itemId", "style", "premise", "title")
				covers = append(covers, service.NewCoverRequest{ItemID: strs[0], Style: strs[1], Premise: strs[2], Title: strs[3]})
			}
			refs := optionalStrings(args, "referenceImageId", "referenceItemId")
			model, _ := OptionalParam[string](args, "model")
			titleText, ok := OptionalParam[bool](args, "titleText")
			if !ok {
				titleText = true // the default, which the schema says
			}

			res, err := art.Generate(ctx, service.GenerateRequest{
				Covers: covers, Model: service.CoverModel(model), TitleText: titleText,
				ReferenceImageID: refs[0], ReferenceItemID: refs[1],
			})
			if err != nil {
				return errResult(generateCoversFailure + err.Error())
			}

			return coverArtResult(res)
		},
	}
}

// cleanCoverFailure is the start of every error result of clean_cover.
const cleanCoverFailure = "failed to clean the cover: "

func cleanCover(art CoverArtService) ServerTool {
	return ServerTool{
		Toolset: metaCovers,
		Tool: mcp.Tool{
			Name: "clean_cover",
			Description: `Clean a book's current cover with Google's Nano Banana image models, for example to remove a watermark or a store label, keeping the rest. This costs money, billed to the user's Gemini key: about $0.16 with model pro (the default) or about $0.07 with standard; tell the user and get their yes first. Offer a cleaning as one of at least three options after showing what exists with search_covers, not as the only fix. ` +
				`instructions says what to change, such as "remove the store label at the bottom"; the rest of the cover is meant to stay as it is, and the cleaned cover is square, as audiobook covers are, with the background extended so that nothing is cut off or stretched. ` +
				`The result is as for generate_covers: a JSON text block followed by the current cover and the cleaned one as images, meant for the user, and the images note says which is which. ` +
				`Set the cleaned cover only after the user has seen it next to the old one and said yes, by calling set_cover with its imageId; if the user's app doesn't show these images, don't set it, and use get_cover_prompt instead. ` +
				`If stopped is set, Google refused more images (a spending limit or quota): tell the user, and don't call again unless they say so. You see these images whether or not the user does, so a pick the user makes from your description is not seeing them: unless the user says they can see the images (in Claude Desktop, by opening the tool result), ask them, and if they can't, don't set one.`,
			Annotations: openWorldCostAnnotations(),
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"itemId": itemIDSchema(),
					"instructions": {
						Type:        "string",
						Description: fmt.Sprintf("What to change on the current cover, such as \"remove the store label at the bottom\", at most %d characters", service.MaxEditLength),
						MaxLength:   new(service.MaxEditLength),
					},
					"model": coverModelSchema(),
				},
				Required: []string{"itemId", "instructions"},
			},
		},
		Handler: func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, error) {
			itemID, err := RequiredParam[string](args, "itemId")
			if err != nil {
				return errResult(err.Error())
			}
			instructions, err := RequiredParam[string](args, "instructions")
			if err != nil {
				return errResult(err.Error())
			}

			model, _ := OptionalParam[string](args, "model")

			res, err := art.Clean(ctx, itemID, instructions, service.CoverModel(model))
			if err != nil {
				return errResult(cleanCoverFailure + err.Error())
			}

			return coverArtResult(res)
		},
	}
}
