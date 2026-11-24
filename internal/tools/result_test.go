package tools

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalledResult_KeepsHTML(t *testing.T) {
	result, err := marshalledResult(map[string]string{"d": "<p>A & B</p>"})

	require.NoError(t, err)
	// JSONEq treats "<" and its \u003c escape as equal, so look at the text itself.
	assert.Contains(t, resultText(result), `"<p>A & B</p>"`)
	assert.NotContains(t, resultText(result), `\u00`)
	assert.False(t, strings.HasSuffix(resultText(result), "\n"), "no trailing newline")
}

func TestWriteFailure_KeepsHTML(t *testing.T) {
	result, err := writeFailure("failed: ", errBoom, []previousOutput[string]{{ItemID: "li_1", Title: "T", Previous: "<p>x</p>"}})

	require.NoError(t, err)
	assert.True(t, result.IsError)
	assert.Equal(t, `failed: ABS API 500: boom; previous values: [{"itemId":"li_1","title":"T","previous":"<p>x</p>"}]`, resultText(result))
}
