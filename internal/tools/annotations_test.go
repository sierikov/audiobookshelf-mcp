package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllTools_DeclareEveryAnnotation(t *testing.T) {
	openWorld := map[string]bool{
		"search_audible": true, "get_audible_chapters": true, "search_book_metadata": true,
		"search_covers": true, "set_cover": true, "generate_covers": true, "clean_cover": true,
	}
	// A call to either of these makes new images, and charges for them, each time.
	notIdempotent := map[string]bool{"generate_covers": true, "clean_cover": true}

	tools := AllTools(Services{CoverArt: NewMockCoverArtService(t)})
	for _, name := range []string{"generate_covers", "clean_cover"} {
		require.NotNil(t, findTool(tools, name), "%s is checked too", name)
	}
	for _, st := range tools {
		name := st.Tool.Name
		a := st.Tool.Annotations
		require.NotNil(t, a, name)
		require.NotNil(t, a.DestructiveHint, name)
		require.NotNil(t, a.OpenWorldHint, name)
		assert.Equal(t, openWorld[name], *a.OpenWorldHint, name)
		assert.Equal(t, !notIdempotent[name], a.IdempotentHint, name)
		if a.ReadOnlyHint {
			assert.False(t, *a.DestructiveHint, name)
		}
	}
}
