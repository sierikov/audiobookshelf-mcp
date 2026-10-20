package tools

import (
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/sierikov/audiobookshelf-mcp/internal/service"
)

func TestChapterTools_Annotations(t *testing.T) {
	tools := ChapterTools(NewMockChapterService(t))

	for _, name := range []string{"get_chapters", "search_audible", "get_audible_chapters"} {
		tool := findTool(tools, name)
		require.NotNil(t, tool, name)
		assert.True(t, tool.Tool.Annotations.ReadOnlyHint, name)
	}
	for _, name := range []string{"set_chapters", "rename_chapters"} {
		tool := findTool(tools, name)
		require.NotNil(t, tool, name)
		a := tool.Tool.Annotations
		assert.False(t, a.ReadOnlyHint, name)
		require.NotNil(t, a.DestructiveHint, name)
		assert.True(t, *a.DestructiveHint, name)
		assert.True(t, a.IdempotentHint, name)
	}
	for _, tool := range tools {
		assert.Equal(t, toolsetChapters, tool.Toolset.ID, tool.Tool.Name)
	}
	schema, ok := findTool(tools, "search_audible").Tool.InputSchema.(*jsonschema.Schema)
	require.True(t, ok)
	region := schema.Properties["region"]
	assert.Equal(t, []any{"us", "ca", "uk", "au", "fr", "de", "jp", "it", "in", "es"}, region.Enum)
}

func TestGetChapters(t *testing.T) {
	svc := NewMockChapterService(t)
	svc.EXPECT().Get(mock.Anything, "item-1").Return(service.BookChapters{
		ID: "item-1", Title: "Dune", Author: "Frank Herbert", ASIN: "B002V1OF70", Duration: time.Hour,
		Chapters: []service.Chapter{
			{Start: 0, End: 10 * time.Minute, Title: "Opening"},
			{Start: 10 * time.Minute, End: time.Hour, Title: "One"},
		},
		Files: []service.AudioFile{{Index: 1, Filename: "m001.mp3", Start: 0, Duration: 3600500 * time.Millisecond}},
	}, nil)

	result := call(t, ChapterTools(svc), "get_chapters", map[string]any{"itemId": "item-1"})

	assertJSON(t, `{
		"id":"item-1","title":"Dune","author":"Frank Herbert","asin":"B002V1OF70",
		"duration":"1h 0m","durationSeconds":3600,
		"chapters":[
			{"start":0,"end":600,"startTime":"0:00:00","title":"Opening"},
			{"start":600,"end":3600,"startTime":"0:10:00","title":"One"}
		],
		"files":[{"index":1,"filename":"m001.mp3","start":0,"duration":3600.5,"startTime":"0:00:00"}]
	}`, result)
}

func TestGetChapters_EmptyListsAreArrays(t *testing.T) {
	svc := NewMockChapterService(t)
	svc.EXPECT().Get(mock.Anything, "item-1").Return(service.BookChapters{
		ID: "item-1", Title: "Dune", Author: "Frank Herbert", Duration: time.Hour,
	}, nil)

	result := call(t, ChapterTools(svc), "get_chapters", map[string]any{"itemId": "item-1"})

	assertJSON(t, `{
		"id":"item-1","title":"Dune","author":"Frank Herbert","duration":"1h 0m","durationSeconds":3600,
		"chapters":[],"files":[]
	}`, result)
}

func TestGetChapters_MissingItemID(t *testing.T) {
	result := call(t, ChapterTools(NewMockChapterService(t)), "get_chapters", map[string]any{})

	assertToolError(t, "missing required parameter: itemId", result)
}

func TestGetChapters_Error(t *testing.T) {
	svc := NewMockChapterService(t)
	svc.EXPECT().Get(mock.Anything, "item-1").Return(service.BookChapters{}, errBoom)

	result := call(t, ChapterTools(svc), "get_chapters", map[string]any{"itemId": "item-1"})

	assertToolError(t, "failed to get chapters: ABS API 500: boom", result)
}

func TestSearchAudible(t *testing.T) {
	svc := NewMockChapterService(t)
	svc.EXPECT().SearchAudible(mock.Anything, "item-1", "de", "", "").Return(service.AudibleSearch{
		Book:   book("item-1", "Dune", "Frank Herbert", time.Hour),
		Region: "de",
		Matches: []service.AudibleMatch{
			{ASIN: "B07X1DUNE1", Title: "Dune", Subtitle: "Der Wüstenplanet", Author: "Frank Herbert",
				Narrator: "Jürgen Holdorf", Language: "German",
				Runtime: 61 * time.Minute, RuntimeDiff: time.Minute, MatchConfidence: new(0.9)},
			{ASIN: "B07X1DUNE2", Title: "Dune", Author: "Frank Herbert", Abridged: true,
				Runtime: 30 * time.Minute, RuntimeDiff: -90 * time.Minute},
		},
	}, nil)

	result := call(t, ChapterTools(svc), "search_audible", map[string]any{"itemId": "item-1", "region": "de"})

	assertJSON(t, `{
		"book":{"id":"item-1","title":"Dune","author":"Frank Herbert","duration":"1h 0m","durationSeconds":3600},
		"region":"de",
		"candidates":[
			{"asin":"B07X1DUNE1","title":"Dune","subtitle":"Der Wüstenplanet","author":"Frank Herbert",
			 "narrator":"Jürgen Holdorf","language":"German","abridged":false,
			 "runtime":"1h 1m","runtimeDiffMinutes":1,"matchConfidence":0.9},
			{"asin":"B07X1DUNE2","title":"Dune","author":"Frank Herbert","abridged":true,
			 "runtime":"30m","runtimeDiffMinutes":-90}
		]
	}`, result)
}

func TestSearchAudible_PassesTitleAndAuthor(t *testing.T) {
	svc := NewMockChapterService(t)
	svc.EXPECT().SearchAudible(mock.Anything, "item-1", "", "Der Wüstenplanet", "Herbert").Return(service.AudibleSearch{
		Book: book("item-1", "Dune", "Frank Herbert", time.Hour), Region: "us",
	}, nil)

	result := call(t, ChapterTools(svc), "search_audible", map[string]any{
		"itemId": "item-1", "title": "Der Wüstenplanet", "author": "Herbert",
	})

	assertJSON(t, `{
		"book":{"id":"item-1","title":"Dune","author":"Frank Herbert","duration":"1h 0m","durationSeconds":3600},
		"region":"us","candidates":[]
	}`, result)
}

func TestSearchAudible_Error(t *testing.T) {
	svc := NewMockChapterService(t)
	svc.EXPECT().SearchAudible(mock.Anything, "item-1", "xx", "", "").Return(service.AudibleSearch{}, errBoom)

	result := call(t, ChapterTools(svc), "search_audible", map[string]any{"itemId": "item-1", "region": "xx"})

	assertToolError(t, "Audible search failed: ABS API 500: boom", result)
}

func TestGetAudibleChapters(t *testing.T) {
	svc := NewMockChapterService(t)
	svc.EXPECT().AudibleChapters(mock.Anything, "B002V1OF70", "").Return(service.AudibleChapters{
		ASIN: "B002V1OF70", Region: "us", Runtime: time.Hour,
		BrandIntro: 2043 * time.Millisecond, BrandOutro: 5061 * time.Millisecond, Accurate: true,
		Chapters: []service.Chapter{{Start: 0, End: time.Hour, Title: "Chapter 1"}},
	}, nil)

	result := call(t, ChapterTools(svc), "get_audible_chapters", map[string]any{"asin": "B002V1OF70"})

	assertJSON(t, `{
		"asin":"B002V1OF70","region":"us","runtime":"1h 0m","runtimeSeconds":3600,"accurate":true,
		"brandIntroSeconds":2.043,"brandOutroSeconds":5.061,
		"chapters":[{"start":0,"end":3600,"startTime":"0:00:00","title":"Chapter 1"}]
	}`, result)
}

func TestGetAudibleChapters_Error(t *testing.T) {
	svc := NewMockChapterService(t)
	svc.EXPECT().AudibleChapters(mock.Anything, "B000000000", "de").Return(service.AudibleChapters{}, errBoom)

	result := call(t, ChapterTools(svc), "get_audible_chapters", map[string]any{"asin": "B000000000", "region": "de"})

	assertToolError(t, "failed to get Audible chapters: ABS API 500: boom", result)
}

// change is a save result with one chapter replaced by two.
func change() service.ChapterChange {
	return service.ChapterChange{
		Updated:  true,
		Previous: []service.Chapter{{Start: 0, End: time.Hour, Title: "Chapter 1"}},
		Saved: []service.Chapter{
			{Start: 0, End: 600500 * time.Millisecond, Title: "Opening"},
			{Start: 600500 * time.Millisecond, End: time.Hour, Title: "One"},
		},
	}
}

const changeJSON = `{
	"updated":true,
	"previous":[{"start":0,"end":3600,"startTime":"0:00:00","title":"Chapter 1"}],
	"chapters":[
		{"start":0,"end":600.5,"startTime":"0:00:00","title":"Opening"},
		{"start":600.5,"end":3600,"startTime":"0:10:00","title":"One"}
	]
}`

func TestSetChapters(t *testing.T) {
	svc := NewMockChapterService(t)
	svc.EXPECT().Set(mock.Anything, "item-1", []service.ChapterInput{
		{Start: 0, Title: "Opening"},
		{Start: 600500 * time.Millisecond, Title: "One"},
	}).Return(change(), nil)

	result := call(t, ChapterTools(svc), "set_chapters", map[string]any{
		"itemId": "item-1",
		"chapters": []any{
			map[string]any{"start": 0.0, "title": "Opening"},
			map[string]any{"start": 600.5, "title": "One"},
		},
	})

	assertJSON(t, changeJSON, result)
}

func TestSetChapters_EmptyList(t *testing.T) {
	svc := NewMockChapterService(t)
	svc.EXPECT().Set(mock.Anything, "item-1", []service.ChapterInput{}).Return(service.ChapterChange{
		Updated: true, Previous: []service.Chapter{{Start: 0, End: time.Hour, Title: "Chapter 1"}},
		Saved: []service.Chapter{},
	}, nil)

	result := call(t, ChapterTools(svc), "set_chapters", map[string]any{"itemId": "item-1", "chapters": []any{}})

	assertJSON(t, `{
		"updated":true,
		"previous":[{"start":0,"end":3600,"startTime":"0:00:00","title":"Chapter 1"}],
		"chapters":[]
	}`, result)
}

func TestSetChapters_BadArguments(t *testing.T) {
	tests := []struct {
		name     string
		chapters any
		want     string
	}{
		{"missing", nil, "missing required parameter: chapters"},
		{"not an array", "x", "parameter chapters has wrong type: expected []interface {}, got string"},
		{"start as clock text", []any{map[string]any{"start": "10:00", "title": "A"}}, "chapter 1: start must be a number of seconds"},
		{"negative start", []any{map[string]any{"start": -5.0, "title": "A"}}, "chapter 1: start must be between 0 and 1000000000 seconds"},
		{"huge start", []any{map[string]any{"start": 1e12, "title": "A"}}, "chapter 1: start must be between 0 and 1000000000 seconds"},
		{"missing title", []any{map[string]any{"start": 0.0}}, "chapter 1: title must be a string"},
		{"not an object", []any{0.0}, "chapter 1 must be an object with start and title"},
		{"second chapter bad", []any{map[string]any{"start": 0.0, "title": "A"}, map[string]any{"title": "B"}}, "chapter 2: start must be a number of seconds"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]any{"itemId": "item-1"}
			if tt.chapters != nil {
				args["chapters"] = tt.chapters
			}

			result := call(t, ChapterTools(NewMockChapterService(t)), "set_chapters", args)

			assertToolError(t, tt.want, result)
		})
	}
}

func TestSetChapters_ServiceError(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{&service.ChapterError{Chapter: 2, Reason: "has no title"}, "failed to save chapters: chapter 2 has no title"},
		{errBoom, "failed to save chapters: ABS API 500: boom"},
	}
	for _, tt := range tests {
		svc := NewMockChapterService(t)
		svc.EXPECT().Set(mock.Anything, "item-1", mock.Anything).Return(service.ChapterChange{}, tt.err)

		result := call(t, ChapterTools(svc), "set_chapters", map[string]any{
			"itemId": "item-1", "chapters": []any{map[string]any{"start": 0.0, "title": "A"}},
		})

		assertToolError(t, tt.want, result)
	}
}

func TestRenameChapters(t *testing.T) {
	svc := NewMockChapterService(t)
	svc.EXPECT().Rename(mock.Anything, "item-1", []string{"Opening", "One"}).Return(change(), nil)

	result := call(t, ChapterTools(svc), "rename_chapters", map[string]any{
		"itemId": "item-1", "titles": []any{"Opening", "One"},
	})

	assertJSON(t, changeJSON, result)
}

func TestRenameChapters_BadArguments(t *testing.T) {
	result := call(t, ChapterTools(NewMockChapterService(t)), "rename_chapters", map[string]any{
		"itemId": "item-1", "titles": []any{"A", 3.0},
	})

	assertToolError(t, "title 2 must be a string", result)
}

func TestRenameChapters_Error(t *testing.T) {
	svc := NewMockChapterService(t)
	svc.EXPECT().Rename(mock.Anything, "item-1", []string{"A"}).Return(service.ChapterChange{}, errBoom)

	result := call(t, ChapterTools(svc), "rename_chapters", map[string]any{"itemId": "item-1", "titles": []any{"A"}})

	assertToolError(t, "failed to rename chapters: ABS API 500: boom", result)
}
