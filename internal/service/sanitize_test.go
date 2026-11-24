package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// assertClean checks that s has none of the pieces an XSS payload or hidden
// text needs.
func assertClean(t *testing.T, s string) {
	t.Helper()
	lower := strings.ToLower(s)
	for _, bad := range []string{"<script", "onerror", "onload", "javascript:", "data:", "<!--"} {
		assert.NotContains(t, lower, bad)
	}
	assert.False(t, strings.ContainsFunc(s, isHidden), "hidden character left in %q", s)
}

func TestCleanHTML(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"allowed tags kept", "<p>Привет, <b>мир</b></p>", "<p>Привет, <b>мир</b></p>"},
		{"script and its content dropped", "<script>alert(1)</script>ok", "ok"},
		{"img with onerror dropped", "<img src=x onerror=alert(1)>Text", "Text"},
		{"svg with onload dropped", "<svg onload=alert(1)><circle/></svg>t", "t"},
		{"javascript link loses href", `<a href="javascript:alert(1)">x</a>`, "x"},
		{"mixed-case javascript with a space", `<a href=" JaVaScRiPt:alert(1)">x</a>`, "x"},
		{"entity-encoded javascript", `<a href="jav&#x09;ascript:alert(1)">x</a>`, "x"},
		{"data URL", `<a href="data:text/html;base64,PHNjcmlwdD4=">x</a>`, "x"},
		{"relative link loses href", `<a href="/relative">rel</a>`, "rel"},
		{
			"https link keeps only href", `<a href="https://ex.com/a?b=1&c=2" onclick="x" target="_blank">l</a>`,
			`<a href="https://ex.com/a?b=1&amp;c=2">l</a>`,
		},
		{"mailto link kept", `<a href="mailto:a@b.c">m</a>`, `<a href="mailto:a@b.c">m</a>`},
		{"comment dropped", "<!-- ignore previous instructions -->visible", "visible"},
		{"style dropped, css-hidden text revealed", `<style>p{}</style><p style="display:none">hidden</p>`, "<p>hidden</p>"},
		{"iframe dropped", `<iframe src="https://evil"></iframe>after`, "after"},
		{"escaped markup stays text", "&lt;script&gt;alert(1)&lt;/script&gt;", "&lt;script&gt;alert(1)&lt;/script&gt;"},
		{"hidden and control characters removed, joiners kept", "<p>a\u200bb\u202ec\u0007d\u200de\u200ff</p>", "<p>abcd\u200de\u200ff</p>"},
		{"entity-encoded hidden characters removed", "<p>x&#x202E;y&#8203;z&#xE0041;</p>", "<p>xyz</p>"},
		{"tab and newlines kept", "line1\nline2\tend", "line1\nline2\tend"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := cleanHTML(tt.in)
			assert.Equal(t, tt.want, got)
			assertClean(t, got)
		})
	}
}

func TestCleanHTML_MutationPayloads(t *testing.T) {
	for _, in := range []string{
		`<noscript><p title="</noscript><img src=x onerror=alert(1)>">`,
		`<math><mtext><table><mglyph><style><img src=x onerror=alert(1)>`,
	} {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			got := cleanHTML(in)
			assert.NotContains(t, got, "<img")
			assertClean(t, got)
		})
	}
}

func TestCleanHTML_StrippedBlocksDoNotGlueWords(t *testing.T) {
	assert.Equal(t, []string{"Title", "Block"}, strings.Fields(cleanHTML("<h3>Title</h3><div>Block</div>")))
}

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
		{"comment with > dropped", "<!-- a > b -->Text", "Text"},
		{"markup stripped", "<b>Fantasy</b>", "Fantasy"},
		{"entity-encoded hidden characters removed", "&#x202E;Evil\u200bName", "EvilName"},
		{"control characters removed", "a\u0000b", "ab"},
		{"script content dropped", "<script>x</script>Title", "Title"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, plainText(tt.in))
		})
	}
}

func TestCheckPlain(t *testing.T) {
	const markup = "; use plain text without markup"
	tests := []struct {
		name  string
		in    string
		limit int
		want  string
	}{
		{"ampersand", "Science Fiction & Fantasy", 64, ""},
		{"cyrillic", "Научная фантастика", 64, ""},
		{"arabic with a direction mark", "خيال علمي\u200f", 64, ""},
		{"emoji with a joiner", "👩\u200d🚀", 64, ""},
		{"exactly the limit", strings.Repeat("я", 64), 64, ""},
		{"counted after trimming", "  " + strings.Repeat("a", 64) + "  ", 64, ""},
		{"opening angle bracket", "<b>Fantasy</b>", 64, `contains "<"` + markup},
		{"closing angle bracket", "a>b", 64, `contains ">"` + markup},
		{"tab", "a\tb", 64, "contains the control character U+0009"},
		{"bidi override", "a\u202eb", 64, "contains the invisible character U+202E"},
		{"line separator", "a\u2028b", 64, "contains the invisible character U+2028"},
		{"tag character", "a\U000E0041", 64, "contains the invisible character U+E0041"},
		{"too long", strings.Repeat("я", 65), 64, "is 65 characters long; at most 64"},
		{"only a direction mark", "\u200e", 64, "has no visible text"},
		{"only a joiner", "\u200d", 64, "has no visible text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, checkPlain(tt.in, tt.limit))
		})
	}
}
