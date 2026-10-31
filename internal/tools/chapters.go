package tools

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sierikov/audiobookshelf-mcp/internal/service"
)

// ChapterTools returns the tools in the chapters toolset.
func ChapterTools(svc ChapterService) []ServerTool {
	return []ServerTool{
		getChapters(svc),
		searchAudible(svc),
		getAudibleChapters(svc),
		setChapters(svc),
		renameChapters(svc),
	}
}

// maxChapterStart is the latest chapter start accepted, in seconds; anything
// beyond is a mistake, and would not fit a time.Duration.
const maxChapterStart = 1_000_000_000

// writeAnnotations marks a tool that replaces existing chapters: destructive,
// and idempotent because repeating a call leaves the same chapters.
func writeAnnotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{DestructiveHint: new(true), IdempotentHint: true}
}

func itemIDSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "string", Description: "Library item ID of the audiobook"}
}

func regionSchema() *jsonschema.Schema {
	regions := service.AudibleRegions()
	enum := make([]any, 0, len(regions))
	for _, r := range regions {
		enum = append(enum, r)
	}
	return &jsonschema.Schema{Type: "string", Enum: enum, Description: "Audible store region (default us)"}
}

type chapterOutput struct {
	Start     float64 `json:"start"`
	End       float64 `json:"end"`
	StartTime string  `json:"startTime"`
	Title     string  `json:"title"`
}

func toChapterOutputs(cs []service.Chapter) []chapterOutput {
	out := make([]chapterOutput, 0, len(cs))
	for _, c := range cs {
		out = append(out, chapterOutput{
			Start:     c.Start.Seconds(),
			End:       c.End.Seconds(),
			StartTime: FormatClock(c.Start),
			Title:     c.Title,
		})
	}
	return out
}

type fileOutput struct {
	Index     int     `json:"index"`
	Filename  string  `json:"filename"`
	Start     float64 `json:"start"`
	Duration  float64 `json:"duration"`
	StartTime string  `json:"startTime"`
}

type bookChaptersOutput struct {
	ID              string          `json:"id"`
	Title           string          `json:"title"`
	Author          string          `json:"author"`
	ASIN            string          `json:"asin,omitempty"`
	Duration        string          `json:"duration"`
	DurationSeconds float64         `json:"durationSeconds"`
	Chapters        []chapterOutput `json:"chapters"`
	Files           []fileOutput    `json:"files"`
}

func getChapters(svc ChapterService) ServerTool {
	return ServerTool{
		Toolset: metaChapters,
		Tool: mcp.Tool{
			Name:        "get_chapters",
			Description: "Get an audiobook's chapters, total duration and audio files. Times are in seconds. Files are listed for context only; they are rarely chapters.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
			InputSchema: &jsonschema.Schema{
				Type:       "object",
				Properties: map[string]*jsonschema.Schema{"itemId": itemIDSchema()},
				Required:   []string{"itemId"},
			},
		},
		Handler: func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, error) {
			itemID, err := RequiredParam[string](args, "itemId")
			if err != nil {
				return errResult(err.Error())
			}

			b, err := svc.Get(ctx, itemID)
			if err != nil {
				return errResult("failed to get chapters: " + err.Error())
			}

			files := make([]fileOutput, 0, len(b.Files))
			for _, f := range b.Files {
				files = append(files, fileOutput{
					Index:     f.Index,
					Filename:  f.Filename,
					Start:     f.Start.Seconds(),
					Duration:  f.Duration.Seconds(),
					StartTime: FormatClock(f.Start),
				})
			}
			return marshalledResult(bookChaptersOutput{
				ID:              b.ID,
				Title:           b.Title,
				Author:          b.Author,
				ASIN:            b.ASIN,
				Duration:        FormatDuration(b.Duration),
				DurationSeconds: b.Duration.Seconds(),
				Chapters:        toChapterOutputs(b.Chapters),
				Files:           files,
			})
		},
	}
}

type bookRefOutput struct {
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	Author          string  `json:"author"`
	Duration        string  `json:"duration"`
	DurationSeconds float64 `json:"durationSeconds"`
}

type candidateOutput struct {
	ASIN               string   `json:"asin"`
	Title              string   `json:"title"`
	Subtitle           string   `json:"subtitle,omitempty"`
	Author             string   `json:"author"`
	Narrator           string   `json:"narrator,omitempty"`
	Language           string   `json:"language,omitempty"`
	Abridged           bool     `json:"abridged"`
	Runtime            string   `json:"runtime"`
	RuntimeDiffMinutes int      `json:"runtimeDiffMinutes"`
	MatchConfidence    *float64 `json:"matchConfidence,omitempty"`
}

type audibleSearchOutput struct {
	Book       bookRefOutput     `json:"book"`
	Region     string            `json:"region"`
	Candidates []candidateOutput `json:"candidates"`
}

func toCandidates(ms []service.AudibleMatch) []candidateOutput {
	out := make([]candidateOutput, 0, len(ms))
	for _, m := range ms {
		out = append(out, candidateOutput{
			ASIN:               m.ASIN,
			Title:              m.Title,
			Subtitle:           m.Subtitle,
			Author:             m.Author,
			Narrator:           m.Narrator,
			Language:           m.Language,
			Abridged:           m.Abridged,
			Runtime:            FormatDuration(m.Runtime),
			RuntimeDiffMinutes: int(math.Round(m.RuntimeDiff.Minutes())),
			MatchConfidence:    m.MatchConfidence,
		})
	}
	return out
}

func searchAudible(svc ChapterService) ServerTool {
	return ServerTool{
		Toolset: metaChapters,
		Tool: mcp.Tool{
			Name:        "search_audible",
			Description: "Search Audible for editions of an audiobook, closest runtime first. Pick the unabridged edition in the right language whose runtime matches the book. If none does, retry with another spelling of the author (Audible often uses initials) or another region.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"itemId": itemIDSchema(),
					"region": regionSchema(),
					"title":  {Type: "string", Description: "Title to search for (default: the book's title)"},
					"author": {Type: "string", Description: "Author to search for (default: the book's author)"},
				},
				Required: []string{"itemId"},
			},
		},
		Handler: func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, error) {
			itemID, err := RequiredParam[string](args, "itemId")
			if err != nil {
				return errResult(err.Error())
			}
			region, _, _ := OptionalParam[string](args, "region")
			title, _, _ := OptionalParam[string](args, "title")
			author, _, _ := OptionalParam[string](args, "author")

			res, err := svc.SearchAudible(ctx, itemID, region, title, author)
			if err != nil {
				return errResult("Audible search failed: " + err.Error())
			}

			return marshalledResult(audibleSearchOutput{
				Book: bookRefOutput{
					ID:              res.Book.ID,
					Title:           res.Book.Title,
					Author:          res.Book.Author,
					Duration:        FormatDuration(res.Book.Duration),
					DurationSeconds: res.Book.Duration.Seconds(),
				},
				Region:     res.Region,
				Candidates: toCandidates(res.Matches),
			})
		},
	}
}

type audibleChaptersOutput struct {
	ASIN              string          `json:"asin"`
	Region            string          `json:"region"`
	Runtime           string          `json:"runtime"`
	RuntimeSeconds    float64         `json:"runtimeSeconds"`
	Accurate          bool            `json:"accurate"`
	BrandIntroSeconds float64         `json:"brandIntroSeconds"`
	BrandOutroSeconds float64         `json:"brandOutroSeconds"`
	Chapters          []chapterOutput `json:"chapters"`
}

func getAudibleChapters(svc ChapterService) ServerTool {
	return ServerTool{
		Toolset: metaChapters,
		Tool: mcp.Tool{
			Name:        "get_audible_chapters",
			Description: "Get Audible's chapter list for an edition by ASIN and region. Times are in seconds. Compare with the book before saving: if runtimeSeconds differs from the book's duration, or the chapters sit a constant few seconds off the book's own (often brandIntroSeconds), shift every start by that offset before set_chapters.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"asin":   {Type: "string", Description: "Audible ASIN of the edition"},
					"region": regionSchema(),
				},
				Required: []string{"asin"},
			},
		},
		Handler: func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, error) {
			asin, err := RequiredParam[string](args, "asin")
			if err != nil {
				return errResult(err.Error())
			}
			region, _, _ := OptionalParam[string](args, "region")

			ac, err := svc.AudibleChapters(ctx, asin, region)
			if err != nil {
				return errResult("failed to get Audible chapters: " + err.Error())
			}

			return marshalledResult(audibleChaptersOutput{
				ASIN:              ac.ASIN,
				Region:            ac.Region,
				Runtime:           FormatDuration(ac.Runtime),
				RuntimeSeconds:    ac.Runtime.Seconds(),
				Accurate:          ac.Accurate,
				BrandIntroSeconds: ac.BrandIntro.Seconds(),
				BrandOutroSeconds: ac.BrandOutro.Seconds(),
				Chapters:          toChapterOutputs(ac.Chapters),
			})
		},
	}
}

type changeOutput struct {
	Updated  bool            `json:"updated"`
	Previous []chapterOutput `json:"previous"`
	Chapters []chapterOutput `json:"chapters"`
}

func toChangeOutput(c service.ChapterChange) changeOutput {
	return changeOutput{
		Updated:  c.Updated,
		Previous: toChapterOutputs(c.Previous),
		Chapters: toChapterOutputs(c.Saved),
	}
}

func setChapters(svc ChapterService) ServerTool {
	return ServerTool{
		Toolset: metaChapters,
		Tool: mcp.Tool{
			Name:        "set_chapters",
			Description: "Replace ALL chapters of an audiobook. Show the user the complete list and get their confirmation before calling. Returns the previous chapters; to undo, call again with them.",
			Annotations: writeAnnotations(),
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"itemId": itemIDSchema(),
					"chapters": {
						Type:        "array",
						Description: "The complete new chapter list, in order. Each chapter ends where the next starts; the last ends with the book. An empty list removes all chapters.",
						Items: &jsonschema.Schema{
							Type: "object",
							Properties: map[string]*jsonschema.Schema{
								"start": {Type: "number", Description: "Start in seconds from the beginning of the book; the first chapter starts at 0"},
								"title": {Type: "string", Description: "Chapter title"},
							},
							Required: []string{"start", "title"},
						},
					},
				},
				Required: []string{"itemId", "chapters"},
			},
		},
		Handler: func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, error) {
			itemID, err := RequiredParam[string](args, "itemId")
			if err != nil {
				return errResult(err.Error())
			}
			raw, err := RequiredParam[[]any](args, "chapters")
			if err != nil {
				return errResult(err.Error())
			}
			chapters, err := parseChapterInputs(raw)
			if err != nil {
				return errResult(err.Error())
			}

			change, err := svc.Set(ctx, itemID, chapters)
			if err != nil {
				return errResult("failed to save chapters: " + err.Error())
			}
			return marshalledResult(toChangeOutput(change))
		},
	}
}

func renameChapters(svc ChapterService) ServerTool {
	return ServerTool{
		Toolset: metaChapters,
		Tool: mcp.Tool{
			Name:        "rename_chapters",
			Description: "Rename an audiobook's chapters, keeping every timestamp. Pass one title per existing chapter, in order. Show the user the new titles and get their confirmation before calling.",
			Annotations: writeAnnotations(),
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"itemId": itemIDSchema(),
					"titles": {
						Type:        "array",
						Description: "New titles, one per existing chapter, in order",
						Items:       &jsonschema.Schema{Type: "string"},
					},
				},
				Required: []string{"itemId", "titles"},
			},
		},
		Handler: func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, error) {
			itemID, err := RequiredParam[string](args, "itemId")
			if err != nil {
				return errResult(err.Error())
			}
			raw, err := RequiredParam[[]any](args, "titles")
			if err != nil {
				return errResult(err.Error())
			}
			titles, err := parseTitles(raw)
			if err != nil {
				return errResult(err.Error())
			}

			change, err := svc.Rename(ctx, itemID, titles)
			if err != nil {
				return errResult("failed to rename chapters: " + err.Error())
			}
			return marshalledResult(toChangeOutput(change))
		},
	}
}

// parseChapterInputs reads the set_chapters list: objects with a start in
// seconds and a title. Chapters are numbered from 1 in errors.
func parseChapterInputs(raw []any) ([]service.ChapterInput, error) {
	out := make([]service.ChapterInput, 0, len(raw))
	for i, item := range raw {
		n := i + 1
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("chapter %d must be an object with start and title", n)
		}
		start, ok := obj["start"].(float64)
		if !ok {
			return nil, fmt.Errorf("chapter %d: start must be a number of seconds", n)
		}
		if !(start >= 0 && start <= maxChapterStart) {
			return nil, fmt.Errorf("chapter %d: start must be between 0 and %d seconds", n, maxChapterStart)
		}
		title, ok := obj["title"].(string)
		if !ok {
			return nil, fmt.Errorf("chapter %d: title must be a string", n)
		}
		out = append(out, service.ChapterInput{Start: time.Duration(math.Round(start * float64(time.Second))), Title: title})
	}
	return out, nil
}

// parseTitles reads the rename_chapters list of titles.
func parseTitles(raw []any) ([]string, error) {
	return parseStrings(raw, "title")
}
