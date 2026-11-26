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

func TestFlexSeries_Shapes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []abs.SeriesRef
	}{
		{"array", `{"series":[{"id":"s1","name":"Holly Gibney","sequence":"1"}]}`,
			[]abs.SeriesRef{{ID: "s1", Name: "Holly Gibney", Sequence: "1"}}},
		{"single object", `{"series":{"id":"s1","name":"Holly Gibney","sequence":"3"}}`,
			[]abs.SeriesRef{{ID: "s1", Name: "Holly Gibney", Sequence: "3"}}},
		{"null", `{"series":null}`, nil},
		{"absent", `{}`, nil},
		{"empty array", `{"series":[]}`, nil},
		{"null sequence", `{"series":[{"id":"s2","name":"X","sequence":null}]}`,
			[]abs.SeriesRef{{ID: "s2", Name: "X"}}},
		{"two entries", `{"series":[{"name":"A","sequence":"1"},{"name":"B","sequence":"1.5"}]}`,
			[]abs.SeriesRef{{Name: "A", Sequence: "1"}, {Name: "B", Sequence: "1.5"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got abs.BookMetadata
			require.NoError(t, json.Unmarshal([]byte(tt.in), &got))
			if len(tt.want) == 0 {
				assert.Empty(t, got.Series)
				return
			}
			assert.Equal(t, abs.FlexSeries(tt.want), got.Series)
		})
	}
}

func TestFlexSeries_RejectsOtherShapes(t *testing.T) {
	var got abs.FlexSeries
	require.Error(t, json.Unmarshal([]byte(`"Holly Gibney"`), &got))
}
