package service

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlainText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"block tags do not glue words", "<p>One</p><p>Two</p>", "One Two"},
		{"entities decoded", "Tom &amp; Jerry<br/>end", "Tom & Jerry end"},
		{"non-breaking space only", "<p>&nbsp;</p>", ""},
		{"cyrillic unchanged", "Читает Игорь Князев", "Читает Игорь Князев"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, plainText(tt.in))
		})
	}
}

func TestCheckItemIDs(t *testing.T) {
	t.Run("trims ids", func(t *testing.T) {
		got, err := checkItemIDs([]string{" li_1", "li_2 "})

		require.NoError(t, err)
		assert.Equal(t, []string{"li_1", "li_2"}, got)
	})
	t.Run("no ids", func(t *testing.T) {
		_, err := checkItemIDs([]string{})

		assert.ErrorIs(t, err, ErrNoItems)
	})
	t.Run("too many ids", func(t *testing.T) {
		ids := make([]string, MaxItemsPerCall+1)
		for i := range ids {
			ids[i] = "li_" + strconv.Itoa(i)
		}

		_, err := checkItemIDs(ids)

		require.ErrorIs(t, err, ErrTooManyItems)
		assert.Contains(t, err.Error(), "101, at most 100 per call")
	})
	t.Run("exactly the maximum is fine", func(t *testing.T) {
		ids := make([]string, MaxItemsPerCall)
		for i := range ids {
			ids[i] = "li_" + strconv.Itoa(i)
		}

		got, err := checkItemIDs(ids)

		require.NoError(t, err)
		assert.Len(t, got, MaxItemsPerCall)
	})
	t.Run("empty id", func(t *testing.T) {
		_, err := checkItemIDs([]string{"li_1", ""})

		var ie *ItemError
		require.ErrorAs(t, err, &ie)
		assert.Equal(t, 2, ie.Position)
		assert.Equal(t, "item 2 (): has no id", err.Error())
	})
	t.Run("repeated id after trimming", func(t *testing.T) {
		_, err := checkItemIDs([]string{"li_1", " li_1"})

		var ie *ItemError
		require.ErrorAs(t, err, &ie)
		assert.Equal(t, "li_1", ie.ItemID)
		assert.Equal(t, "item 2 (li_1): is listed twice", err.Error())
	})
	t.Run("ids differing in case are distinct", func(t *testing.T) {
		got, err := checkItemIDs([]string{"li_a", "li_A"})

		require.NoError(t, err)
		assert.Equal(t, []string{"li_a", "li_A"}, got)
	})
}

func TestCheckValues(t *testing.T) {
	t.Run("trims values", func(t *testing.T) {
		got, err := checkValues([]string{" Science Fiction "})

		require.NoError(t, err)
		assert.Equal(t, []string{"Science Fiction"}, got)
	})
	t.Run("no values", func(t *testing.T) {
		_, err := checkValues([]string{})

		assert.ErrorIs(t, err, ErrNoValues)
	})
	t.Run("empty value", func(t *testing.T) {
		_, err := checkValues([]string{"A", " "})

		var ve *ValueError
		require.ErrorAs(t, err, &ve)
		assert.Equal(t, 2, ve.Position)
		assert.Equal(t, "value 2 is empty", err.Error())
	})
	t.Run("case duplicate", func(t *testing.T) {
		_, err := checkValues([]string{"Fiction", "fiction"})

		var ve *ValueError
		require.ErrorAs(t, err, &ve)
		assert.Equal(t, 2, ve.Position)
		assert.Equal(t, `value 2 differs only in case from value 1 ("Fiction")`, err.Error())
	})
	t.Run("exact duplicate is also rejected", func(t *testing.T) {
		_, err := checkValues([]string{"Fiction", "Fiction"})

		assert.EqualError(t, err, `value 2 differs only in case from value 1 ("Fiction")`)
	})
}

func TestAddValues(t *testing.T) {
	tests := []struct {
		name         string
		current, add []string
		want         []string
		conflict     string
	}{
		{"into nil", nil, []string{"A"}, []string{"A"}, ""},
		{"skips present", []string{"A"}, []string{"A", "B"}, []string{"A", "B"}, ""},
		{"case conflict", []string{"fiction"}, []string{"Fiction"}, []string{"fiction", "Fiction"}, "fiction"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, conflict := addValues(tt.current, tt.add)

			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.conflict, conflict)
		})
	}

	t.Run("never nil", func(t *testing.T) {
		got, _ := addValues(nil, nil)

		assert.NotNil(t, got)
		assert.Empty(t, got)
	})
	t.Run("does not change the input", func(t *testing.T) {
		backing := []string{"A", ""}
		current := backing[:1]

		got, _ := addValues(current, []string{"B"})

		assert.Equal(t, []string{"A", "B"}, got)
		assert.Equal(t, []string{"A"}, current)
		assert.Empty(t, backing[1])
	})
}

func TestRemoveValues(t *testing.T) {
	tests := []struct {
		name            string
		current, remove []string
		want            []string
	}{
		{"drops match", []string{"A", "B"}, []string{"A"}, []string{"B"}},
		{"to empty", []string{"A"}, []string{"A"}, []string{}},
		{"from nil", nil, []string{"A"}, []string{}},
		{"exact match only", []string{"Fiction"}, []string{"fiction"}, []string{"Fiction"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := removeValues(tt.current, tt.remove)

			assert.NotNil(t, got)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMatchesFilter(t *testing.T) {
	m := MetadataItem{
		Genres:      []string{"Fantasy", "Adventure"},
		Tags:        []string{"to-read"},
		Language:    "English",
		Description: "text",
	}
	tests := []struct {
		name string
		f    ItemFilter
		want bool
	}{
		{"empty filter", ItemFilter{}, true},
		{"genre", ItemFilter{Genre: "Fantasy"}, true},
		{"genre absent", ItemFilter{Genre: "Horror"}, false},
		{"genre is case sensitive", ItemFilter{Genre: "fantasy"}, false},
		{"tag", ItemFilter{Tag: "to-read"}, true},
		{"tag absent", ItemFilter{Tag: "read"}, false},
		{"language", ItemFilter{Language: "English"}, true},
		{"language is case sensitive", ItemFilter{Language: "english"}, false},
		{"missing genres but has them", ItemFilter{Missing: MissingGenres}, false},
		{"genre and language", ItemFilter{Genre: "Fantasy", Language: "English"}, true},
		{"genre ok, language not", ItemFilter{Genre: "Fantasy", Language: "German"}, false},
		{"language ok, genre not", ItemFilter{Genre: "Horror", Language: "English"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, matchesFilter(m, tt.f))
		})
	}

	t.Run("missing filter selects books lacking the field", func(t *testing.T) {
		bare := MetadataItem{}

		assert.True(t, matchesFilter(bare, ItemFilter{Missing: MissingTags}))
		assert.False(t, matchesFilter(m, ItemFilter{Missing: MissingTags}))
	})
}

func TestIsMissing(t *testing.T) {
	full := MetadataItem{Genres: []string{"G"}, Tags: []string{"T"}, Language: "English", Description: "<p>Text</p>"}
	tests := []struct {
		name string
		m    MetadataItem
		f    MissingField
		want bool
	}{
		{"empty html description", MetadataItem{Description: "<p></p>"}, MissingDescription, true},
		{"description present", full, MissingDescription, false},
		{"blank language", MetadataItem{Language: "  "}, MissingLanguage, true},
		{"language present", full, MissingLanguage, false},
		{"empty genres", MetadataItem{}, MissingGenres, true},
		{"genres present", full, MissingGenres, false},
		{"empty tags", MetadataItem{}, MissingTags, true},
		{"tags present", full, MissingTags, false},
		{"unknown field", MetadataItem{}, MissingField("other"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isMissing(tt.m, tt.f))
		})
	}
}

func TestCountValues(t *testing.T) {
	t.Run("by count then name", func(t *testing.T) {
		got := countValues([][]string{{"B", "A"}, {"A"}, nil})

		assert.Equal(t, []ValueCount{{Name: "A", Count: 2}, {Name: "B", Count: 1}}, got)
	})
	t.Run("ties by name", func(t *testing.T) {
		got := countValues([][]string{{"B"}, {"A"}})

		assert.Equal(t, []ValueCount{{Name: "A", Count: 1}, {Name: "B", Count: 1}}, got)
	})
	t.Run("nothing", func(t *testing.T) {
		assert.Empty(t, countValues(nil))
	})
}

func TestMissingFieldsAndProviders(t *testing.T) {
	assert.Equal(t,
		[]MissingField{MissingGenres, MissingTags, MissingLanguage, MissingDescription},
		MissingFields())
	assert.Equal(t,
		[]string{"fantlab", "google", "audible", "openlibrary", "itunes"},
		MetadataProviders())
}
