package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequiredParam(t *testing.T) {
	args := map[string]any{"name": "hello"}

	v, err := RequiredParam[string](args, "name")
	require.NoError(t, err)
	assert.Equal(t, "hello", v)

	_, err = RequiredParam[string](args, "missing")
	require.ErrorContains(t, err, "missing required parameter")

	_, err = RequiredParam[int](args, "name")
	require.ErrorContains(t, err, "missing required parameter: name", "a value of another type reads as absent")
}

func TestOptionalParam(t *testing.T) {
	args := map[string]any{"name": "hello"}

	v, ok := OptionalParam[string](args, "name")
	assert.True(t, ok)
	assert.Equal(t, "hello", v)

	_, ok = OptionalParam[string](args, "missing")
	assert.False(t, ok)
}

func TestOptionalIntParam(t *testing.T) {
	args := map[string]any{"limit": float64(42)}

	assert.Equal(t, 42, OptionalIntParam(args, "limit", 10))
	assert.Equal(t, 10, OptionalIntParam(args, "missing", 10))
	assert.Equal(t, 10, OptionalIntParam(map[string]any{"limit": "bad"}, "limit", 10), "a value of another type reads as absent")
}

func TestDefaultToolsets(t *testing.T) {
	defaults := DefaultToolsets()
	assert.True(t, defaults[toolsetLibraries])
	assert.True(t, defaults[toolsetItems])
	assert.True(t, defaults[toolsetPlayback])
	assert.False(t, defaults[toolsetBrowse])
	assert.False(t, defaults[toolsetChapters])
	assert.False(t, defaults[toolsetMetadata])
	assert.False(t, defaults[toolsetCovers])
}

func TestResolveToolsets(t *testing.T) {
	assert.Equal(t, DefaultToolsets(), resolveToolsets(nil))
	assert.Equal(t, map[ToolsetID]bool{toolsetBrowse: true, toolsetItems: true}, resolveToolsets([]string{"browse", " items "}))
	assert.Len(t, resolveToolsets([]string{"all"}), len(AllToolsetMetas()))
	assert.True(t, resolveToolsets([]string{"all"})[toolsetChapters])
	assert.True(t, resolveToolsets([]string{"all"})[toolsetMetadata])
	assert.True(t, resolveToolsets([]string{"all"})[toolsetCovers])
	assert.Equal(t, map[ToolsetID]bool{toolsetChapters: true}, resolveToolsets([]string{"chapters"}))
	assert.Equal(t, map[ToolsetID]bool{toolsetMetadata: true}, resolveToolsets([]string{"metadata"}))
	assert.Equal(t, map[ToolsetID]bool{toolsetCovers: true}, resolveToolsets([]string{"covers"}))
}

func TestToolsets_CoversOptIn(t *testing.T) {
	assert.False(t, DefaultToolsets()[toolsetCovers], "covers is off unless asked for")
	assert.False(t, resolveToolsets(nil)[toolsetCovers])
	assert.True(t, resolveToolsets([]string{"all"})[toolsetCovers], "all includes covers")
	assert.True(t, resolveToolsets([]string{"covers"})[toolsetCovers])
	assert.Contains(t, AllToolsetMetas(), metaCovers)
	assert.False(t, metaCovers.Default)
	assert.Equal(t, "Find, make and set book covers (includes writes; making covers costs money)", metaCovers.Description)

	defaults := connectSession(t, NewServer(mockServices(t), Options{}))
	listed, err := defaults.ListTools(t.Context(), nil)
	require.NoError(t, err)
	for _, tool := range listed.Tools {
		assert.NotContains(t, []string{"search_covers", "set_cover", "get_cover_prompt", "generate_covers", "clean_cover"}, tool.Name,
			"the defaults leave the cover tools out")
	}

	all := connectSession(t, NewServer(mockServices(t), Options{Toolsets: []string{"all"}}))
	listed, err = all.ListTools(t.Context(), nil)
	require.NoError(t, err)
	names := make([]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
	}
	assert.Contains(t, names, "search_covers")
	assert.Contains(t, names, "set_cover")
	assert.Contains(t, names, "get_cover_prompt")
	assert.Contains(t, names, "generate_covers", "the made-cover tools are in covers, when there is a service")
	assert.Contains(t, names, "clean_cover")
}

func TestToolsets_MadeCoverToolsNeedAService(t *testing.T) {
	svcs := mockServices(t)
	svcs.CoverArt = nil

	srv := connectSession(t, NewServer(svcs, Options{Toolsets: []string{"all"}}))
	listed, err := srv.ListTools(t.Context(), nil)
	require.NoError(t, err)
	names := make([]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
	}
	assert.NotContains(t, names, "generate_covers", "no Gemini key, no paid tools")
	assert.NotContains(t, names, "clean_cover")
	assert.Contains(t, names, "set_cover")
	assert.Contains(t, names, "get_cover_prompt")
}
