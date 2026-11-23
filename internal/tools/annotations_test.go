package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllTools_DeclareEveryAnnotation(t *testing.T) {
	openWorld := map[string]bool{"search_audible": true, "get_audible_chapters": true, "search_book_metadata": true}

	for _, st := range AllTools(Services{}) {
		name := st.Tool.Name
		a := st.Tool.Annotations
		require.NotNil(t, a, name)
		require.NotNil(t, a.DestructiveHint, name)
		require.NotNil(t, a.OpenWorldHint, name)
		assert.Equal(t, openWorld[name], *a.OpenWorldHint, name)
		assert.True(t, a.IdempotentHint, name)
		if a.ReadOnlyHint {
			assert.False(t, *a.DestructiveHint, name)
		}
	}
}
