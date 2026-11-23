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
}

func TestResolveToolsets(t *testing.T) {
	assert.Equal(t, DefaultToolsets(), resolveToolsets(nil))
	assert.Equal(t, map[ToolsetID]bool{toolsetBrowse: true, toolsetItems: true}, resolveToolsets([]string{"browse", " items "}))
	assert.Len(t, resolveToolsets([]string{"all"}), len(AllToolsetMetas()))
	assert.True(t, resolveToolsets([]string{"all"})[toolsetChapters])
	assert.True(t, resolveToolsets([]string{"all"})[toolsetMetadata])
	assert.Equal(t, map[ToolsetID]bool{toolsetChapters: true}, resolveToolsets([]string{"chapters"}))
	assert.Equal(t, map[ToolsetID]bool{toolsetMetadata: true}, resolveToolsets([]string{"metadata"}))
}
