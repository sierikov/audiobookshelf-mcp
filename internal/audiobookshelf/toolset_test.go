package audiobookshelf

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
	require.ErrorContains(t, err, "wrong type")
}

func TestOptionalParam(t *testing.T) {
	args := map[string]any{"name": "hello"}

	v, ok, err := OptionalParam[string](args, "name")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "hello", v)

	_, ok, err = OptionalParam[string](args, "missing")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestOptionalIntParam(t *testing.T) {
	args := map[string]any{"limit": float64(42)}

	v, err := OptionalIntParam(args, "limit", 10)
	require.NoError(t, err)
	assert.Equal(t, 42, v)

	v, err = OptionalIntParam(args, "missing", 10)
	require.NoError(t, err)
	assert.Equal(t, 10, v)

	_, err = OptionalIntParam(map[string]any{"limit": "bad"}, "limit", 10)
	assert.Error(t, err)
}

func TestDefaultToolsets(t *testing.T) {
	defaults := DefaultToolsets()
	assert.True(t, defaults[toolsetLibraries])
	assert.True(t, defaults[toolsetItems])
	assert.True(t, defaults[toolsetPlayback])
	assert.False(t, defaults[toolsetBrowse])
}
