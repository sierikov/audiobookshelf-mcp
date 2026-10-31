package abs_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
)

func TestFlexString(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want abs.FlexString
	}{
		{"string", `"2009"`, "2009"},
		{"number", `1961`, "1961"},
		{"number keeps literal text", `2009.0`, "2009.0"},
		{"null", `null`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got abs.FlexString
			require.NoError(t, json.Unmarshal([]byte(tt.in), &got))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFlexString_RejectsOtherShapes(t *testing.T) {
	var got abs.FlexString
	require.Error(t, json.Unmarshal([]byte(`["a"]`), &got))
}

func TestFlexStrings(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want abs.FlexStrings
	}{
		{"array", `["a","b"]`, abs.FlexStrings{"a", "b"}},
		{"comma separated string", `"Genre Fiction, Literary Fiction, "`, abs.FlexStrings{"Genre Fiction", "Literary Fiction"}},
		{"empty string", `""`, nil},
		{"null", `null`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got abs.FlexStrings
			require.NoError(t, json.Unmarshal([]byte(tt.in), &got))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFlexStrings_RejectsOtherShapes(t *testing.T) {
	var got abs.FlexStrings
	require.Error(t, json.Unmarshal([]byte(`42`), &got))
}
