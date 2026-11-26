package setup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs/abstest"
	"github.com/sierikov/audiobookshelf-mcp/internal/agents"
	"github.com/sierikov/audiobookshelf-mcp/internal/config"
)

const (
	testURL   = "https://abs.example"
	testToken = "secret-token"
)

// testEntry is the entry every test hands to Run.
var testEntry = agents.Entry{Command: "npx", Args: []string{"-y", "@sierikov/audiobookshelf-mcp@0.3.0"}}

// fakeAgent records the entries it is asked to add.
type fakeAgent struct {
	id, name string
	detected bool
	result   agents.Result
	err      error
	added    []agents.Entry
}

func (f *fakeAgent) ID() string   { return f.id }
func (f *fakeAgent) Name() string { return f.name }
func (f *fakeAgent) Detect() bool { return f.detected }

func (f *fakeAgent) Add(_ context.Context, e agents.Entry) (agents.Result, error) {
	f.added = append(f.added, e)
	return f.result, f.err
}

type verifyCall struct{ url, token string }

// fakeVerify stands in for the Audiobookshelf check. The first `failures` calls
// return err; later ones return acct.
type fakeVerify struct {
	acct     Account
	err      error
	failures int
	calls    []verifyCall
}

func (v *fakeVerify) check(_ context.Context, url, token string) (Account, error) {
	v.calls = append(v.calls, verifyCall{url, token})
	if len(v.calls) <= v.failures {
		return Account{}, v.err
	}
	return v.acct, nil
}

// fixture is everything Run needs: two detected fake agents, a fake check
// that accepts any credentials, and a settings file in a temp directory.
type fixture struct {
	deps   Deps
	a, b   *fakeAgent
	verify *fakeVerify
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{
		a:      &fakeAgent{id: "fake-a", name: "Fake A", detected: true},
		b:      &fakeAgent{id: "fake-b", name: "Fake B", detected: true},
		verify: &fakeVerify{acct: Account{Username: "artem", Libraries: 1, CanUpdate: true, CanUpload: true}},
	}
	f.deps = Deps{
		ConfigPath: filepath.Join(t.TempDir(), "audiobookshelf-mcp", "config.json"),
		Agents:     []agents.Agent{f.a, f.b},
		Verify:     f.verify.check,
		Entry:      testEntry,
	}
	return f
}

// run runs setup on scripted input and returns what it printed.
func (f *fixture) run(t *testing.T, input string, opts Options) (string, error) {
	t.Helper()
	var out strings.Builder
	err := Run(t.Context(), strings.NewReader(input), &out, opts, f.deps)
	return out.String(), err
}

// seed writes c as the settings that are already saved.
func (f *fixture) seed(t *testing.T, c config.Config) {
	t.Helper()
	require.NoError(t, config.Save(f.deps.ConfigPath, c))
}

// saved reads the settings file back.
func (f *fixture) saved(t *testing.T) config.Config {
	t.Helper()
	c, err := config.Load(f.deps.ConfigPath)
	require.NoError(t, err)
	return c
}

func (f *fixture) requireNothingSaved(t *testing.T) {
	t.Helper()
	_, err := os.Stat(f.deps.ConfigPath)
	require.ErrorIs(t, err, fs.ErrNotExist)
}

// yes is the set of options that makes setup run without a prompt.
func yes() Options {
	return Options{URL: testURL, Token: testToken, Yes: true}
}

func TestRun_InteractiveHappyPath(t *testing.T) {
	f := newFixture(t)

	out, err := f.run(t, "https://abs.example/\nsecret\ny\n\n", Options{})

	require.NoError(t, err)
	assert.Equal(t, config.Config{URL: "https://abs.example", Token: "secret", Toolsets: editingOn}, f.saved(t))
	assert.Equal(t, []verifyCall{{"https://abs.example", "secret"}}, f.verify.calls)
	assert.Contains(t, out, "Audiobookshelf URL: ")
	assert.Contains(t, out, "API token (Settings → Users → your user → API Token): ")
	assert.Contains(t, out, "Connected to https://abs.example as artem, 1 library.")
	assert.Contains(t, out, "Turn on the tools that edit your library (chapters, genres, tags, descriptions, series, covers)? [y/N]: ")
	assert.Contains(t, out, "Saved settings to "+f.deps.ConfigPath+".")
	assert.Contains(t, out, "  1. Fake A\n  2. Fake B\n")
	assert.Contains(t, out, `Add audiobookshelf to all of these? Press Enter for all, or type numbers (e.g. 1,3), or "none": `)
	assert.Contains(t, out, "✓ Fake A: added.")
	assert.Contains(t, out, "✓ Fake B: added.")
	assert.Equal(t, []agents.Entry{testEntry}, f.a.added)
	assert.Equal(t, []agents.Entry{testEntry}, f.b.added)
	assert.NotContains(t, out, "secret")
}

func TestRun_ReadsTheTokenWithReadSecret(t *testing.T) {
	f := newFixture(t)
	f.deps.ReadSecret = func() (string, error) { return " hidden-token \n", nil }

	// The input has no token line: the token must come from ReadSecret.
	out, err := f.run(t, "https://abs.example\nn\nnone\n", Options{})

	require.NoError(t, err)
	assert.Equal(t, config.Config{URL: testURL, Token: "hidden-token"}, f.saved(t))
	assert.Equal(t, []verifyCall{{testURL, "hidden-token"}}, f.verify.calls)
	assert.NotContains(t, out, "hidden-token")
}

func TestRun_ReadSecretErrorStopsSetup(t *testing.T) {
	f := newFixture(t)
	errTerminal := errors.New("not a terminal")
	f.deps.ReadSecret = func() (string, error) { return "", errTerminal }

	_, err := f.run(t, "https://abs.example\n", Options{})

	require.ErrorIs(t, err, errTerminal)
	assert.Empty(t, f.verify.calls)
	f.requireNothingSaved(t)
}

func TestRun_KeepsSavedValuesOnEnter(t *testing.T) {
	f := newFixture(t)
	saved := config.Config{URL: testURL, Token: "saved-token", Toolsets: []string{"all"}}
	f.seed(t, saved)

	out, err := f.run(t, "\n\n\n\n", Options{})

	require.NoError(t, err)
	assert.Equal(t, saved, f.saved(t))
	assert.Equal(t, []verifyCall{{testURL, "saved-token"}}, f.verify.calls)
	assert.Contains(t, out, "Audiobookshelf URL [https://abs.example]: ")
	assert.Contains(t, out, "API token (Settings → Users → your user → API Token), Enter keeps the saved one: ")
	assert.Contains(t, out, "? [Y/n]: ")
	assert.Len(t, f.a.added, 1)
	assert.Len(t, f.b.added, 1)
	assert.NotContains(t, out, "saved-token")
}

func TestRun_FlagsReplaceSavedValues(t *testing.T) {
	f := newFixture(t)
	f.seed(t, config.Config{URL: "https://old.example", Token: "old-token", Toolsets: []string{"all"}})

	out, err := f.run(t, "\n\n", Options{URL: " https://new.example// ", Token: "new-token"})

	require.NoError(t, err)
	assert.Equal(t, config.Config{URL: "https://new.example", Token: "new-token", Toolsets: []string{"all"}}, f.saved(t))
	assert.NotContains(t, out, "Audiobookshelf URL")
	assert.NotContains(t, out, "API token")
}

func TestRun_BadURLScheme(t *testing.T) {
	f := newFixture(t)
	opts := yes()
	opts.URL = "abs.example"

	_, err := f.run(t, "", opts)

	require.EqualError(t, err, "the URL must start with http:// or https://")
	assert.Empty(t, f.verify.calls)
	f.requireNothingSaved(t)
}

func TestRun_YesNeedsAURLAndAToken(t *testing.T) {
	t.Run("no URL", func(t *testing.T) {
		f := newFixture(t)

		_, err := f.run(t, "", Options{Token: testToken, Yes: true})

		require.EqualError(t, err, "an Audiobookshelf URL is required")
	})
	t.Run("no token", func(t *testing.T) {
		f := newFixture(t)

		_, err := f.run(t, "", Options{URL: testURL, Yes: true})

		require.EqualError(t, err, "an API token is required")
		assert.Empty(t, f.verify.calls)
	})
}

func TestRun_VerifyFailsThreeTimesInteractive(t *testing.T) {
	f := newFixture(t)
	f.verify.err = errors.New("ABS API 401: unauthorized")
	f.verify.failures = 99
	// Attempt 1 types both values, attempt 2 keeps the URL and types another
	// token, attempt 3 changes both.
	input := "https://abs.example\nfirst-token\n" + "\nsecond-token\n" + "https://other.example\nother-token\n"

	out, err := f.run(t, input, Options{})

	require.EqualError(t, err, "could not connect after 3 attempts")
	assert.Equal(t, []verifyCall{
		{"https://abs.example", "first-token"},
		{"https://abs.example", "second-token"},
		{"https://other.example", "other-token"},
	}, f.verify.calls)
	assert.Equal(t, 3, strings.Count(out, "Could not connect: ABS API 401: unauthorized\n"))
	assert.Contains(t, out, "Audiobookshelf URL [https://abs.example]: ")
	// Nothing is saved, so a failed token is never offered as the saved one.
	assert.NotContains(t, out, "Enter keeps the saved one")
	f.requireNothingSaved(t)
	assert.Empty(t, f.a.added)
}

func TestRun_RetryFallsBackToTheSavedToken(t *testing.T) {
	f := newFixture(t)
	f.seed(t, config.Config{URL: testURL, Token: "saved-token"})
	f.verify.err = errors.New("ABS API 401: unauthorized")
	f.verify.failures = 1
	// Attempt 1 keeps the URL and types a token that fails; attempt 2 presses
	// Enter at both prompts.
	input := "\ntyped-token\n" + "\n\n" + "n\nnone\n"

	out, err := f.run(t, input, Options{})

	require.NoError(t, err)
	assert.Equal(t, []verifyCall{{testURL, "typed-token"}, {testURL, "saved-token"}}, f.verify.calls)
	assert.Equal(t, 2, strings.Count(out, "Enter keeps the saved one: "))
	assert.Equal(t, "saved-token", f.saved(t).Token)
}

func TestRun_VerifyRecoversOnRetry(t *testing.T) {
	f := newFixture(t)
	f.verify.err = errors.New("ABS API 401: unauthorized")
	f.verify.failures = 1

	out, err := f.run(t, "https://abs.example\nwrong\nhttps://abs.example\nright\nn\nnone\n", Options{})

	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(out, "Could not connect:"))
	assert.Contains(t, out, "Connected to https://abs.example as artem, 1 library.")
	assert.Equal(t, config.Config{URL: testURL, Token: "right"}, f.saved(t))
}

func TestRun_YesDoesNotRetryAFailedCheck(t *testing.T) {
	f := newFixture(t)
	errDown := errors.New("connection refused")
	f.verify.err = errDown
	f.verify.failures = 99

	out, err := f.run(t, "", yes())

	require.ErrorIs(t, err, errDown)
	assert.Len(t, f.verify.calls, 1)
	assert.NotContains(t, out, "Audiobookshelf URL")
	f.requireNothingSaved(t)
}

func TestRun_NeverPrintsTheToken(t *testing.T) {
	f := newFixture(t)
	f.verify.err = fmt.Errorf("server rejected %s", testToken)
	f.verify.failures = 99

	for name, opts := range map[string]Options{
		"interactive": {URL: testURL, Token: testToken},
		"yes":         yes(),
	} {
		t.Run(name, func(t *testing.T) {
			// Attempts 2 and 3 press Enter at the URL and token prompts.
			out, err := f.run(t, "\n\n\n\n", opts)

			require.Error(t, err)
			assert.NotContains(t, out, testToken)
			assert.NotContains(t, err.Error(), testToken)
		})
	}
}

func TestRun_ConnectedMessageCountsLibraries(t *testing.T) {
	tests := []struct {
		libraries int
		want      string
	}{
		{0, "Connected to https://abs.example as artem, 0 libraries."},
		{1, "Connected to https://abs.example as artem, 1 library."},
		{2, "Connected to https://abs.example as artem, 2 libraries."},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.libraries), func(t *testing.T) {
			f := newFixture(t)
			f.verify.acct.Libraries = tt.libraries

			out, err := f.run(t, "", yes())

			require.NoError(t, err)
			assert.Contains(t, out, tt.want)
		})
	}
}

// editingOn is what answering y to the toolsets prompt saves: the default
// toolsets, browse and the editing ones, spelled out.
var editingOn = []string{"libraries", "items", "playback", "browse", "chapters", "metadata", "covers"}

func TestRun_ToolsetsPrompt(t *testing.T) {
	tests := []struct {
		name   string
		saved  []string
		answer string
		want   []string
		suffix string
	}{
		{"nothing saved, Enter", nil, "", nil, "[y/N]"},
		{"nothing saved, y", nil, "y", editingOn, "[y/N]"},
		{"nothing saved, Yes in capitals", nil, "YES", editingOn, "[y/N]"},
		{"nothing saved, n", nil, "n", nil, "[y/N]"},
		{"all saved, Enter keeps editing on", []string{"all"}, "", []string{"all"}, "[Y/n]"},
		{"editing list saved, Enter keeps it", editingOn, "", editingOn, "[Y/n]"},
		{"editing list saved, n turns editing off", editingOn, "n", nil, "[Y/n]"},
		{"chapters saved, Enter keeps them", []string{"chapters"}, "", []string{"chapters"}, "[Y/n]"},
		{"metadata saved, Enter keeps them", []string{"metadata"}, "", []string{"metadata"}, "[Y/n]"},
		{"covers saved, Enter keeps them", []string{"covers"}, "", []string{"covers"}, "[Y/n]"},
		{"all saved, n turns editing off", []string{"all"}, "n", nil, "[Y/n]"},
		{"all saved, no turns editing off", []string{"all"}, "no", nil, "[Y/n]"},
		{"read-only saved, Enter keeps them", []string{"browse"}, "", []string{"browse"}, "[y/N]"},
		{"read-only saved, y", []string{"browse"}, "y", editingOn, "[y/N]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.seed(t, config.Config{URL: "https://old.example", Token: "old-token", Toolsets: tt.saved})

			out, err := f.run(t, tt.answer+"\nnone\n", Options{URL: testURL, Token: testToken})

			require.NoError(t, err)
			assert.Contains(t, out, "(chapters, genres, tags, descriptions, series, covers)? "+tt.suffix+": ")
			assert.Equal(t, tt.want, f.saved(t).Toolsets)
		})
	}
}

func TestRun_ToolsetsPromptRejectsAnUnknownAnswer(t *testing.T) {
	f := newFixture(t)

	_, err := f.run(t, "maybe\n", Options{URL: testURL, Token: testToken})

	require.ErrorContains(t, err, `"maybe"`)
	f.requireNothingSaved(t)
}

func TestRun_ToolsetsFlagAndYes(t *testing.T) {
	tests := []struct {
		name  string
		saved []string
		opts  Options
		want  []string
	}{
		{"flag replaces saved", []string{"all"}, Options{ToolsetsSet: true, Toolsets: []string{"chapters"}}, []string{"chapters"}},
		{"empty flag turns editing off", []string{"all"}, Options{ToolsetsSet: true}, nil},
		{"yes keeps saved", []string{"metadata"}, Options{}, []string{"metadata"}},
		{"yes with nothing saved", nil, Options{}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.seed(t, config.Config{URL: testURL, Token: testToken, Toolsets: tt.saved})
			tt.opts.URL, tt.opts.Token, tt.opts.Yes = testURL, testToken, true

			out, err := f.run(t, "", tt.opts)

			require.NoError(t, err)
			assert.Equal(t, tt.want, f.saved(t).Toolsets)
			assert.NotContains(t, out, "[y/N]")
			assert.NotContains(t, out, "[Y/n]")
		})
	}
}

func TestRun_ToolsetsFlagSkipsThePromptWhenInteractive(t *testing.T) {
	f := newFixture(t)

	out, err := f.run(t, "none\n", Options{URL: testURL, Token: testToken, ToolsetsSet: true, Toolsets: []string{"all"}})

	require.NoError(t, err)
	assert.NotContains(t, out, "Turn on the tools")
	assert.Equal(t, []string{"all"}, f.saved(t).Toolsets)
}

func TestRun_WarnsWithoutUpdatePermission(t *testing.T) {
	const note = "Note: artem has no Update permission in Audiobookshelf, so the editing tools will fail until an admin grants it."
	tests := []struct {
		name      string
		toolsets  []string
		canUpdate bool
		warns     bool
	}{
		{"all, no permission", []string{"all"}, false, true},
		{"chapters, no permission", []string{"chapters"}, false, true},
		{"metadata, no permission", []string{"metadata"}, false, true},
		{"covers, no permission", []string{"covers"}, false, true},
		{"all, with permission", []string{"all"}, true, false},
		{"editing off, no permission", nil, false, false},
		{"read-only toolset, no permission", []string{"browse"}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.verify.acct.CanUpdate = tt.canUpdate
			opts := yes()
			opts.ToolsetsSet, opts.Toolsets = true, tt.toolsets

			out, err := f.run(t, "", opts)

			require.NoError(t, err)
			if tt.warns {
				assert.Contains(t, out, note)
			} else {
				assert.NotContains(t, out, "Update permission")
			}
		})
	}
}

func TestRun_WarnsWithoutUploadPermission(t *testing.T) {
	const note = "Note: artem has no Upload permission in Audiobookshelf, so set_cover will fail until an admin grants it."
	tests := []struct {
		name      string
		toolsets  []string
		canUpload bool
		warns     bool
	}{
		{"covers, no permission", []string{"covers"}, false, true},
		{"all, no permission", []string{"all"}, false, true},
		{"covers among others, no permission", []string{"libraries", "chapters", "covers"}, false, true},
		{"covers, with permission", []string{"covers"}, true, false},
		{"all, with permission", []string{"all"}, true, false},
		{"other editing toolsets, no permission", []string{"chapters", "metadata"}, false, false},
		{"editing off, no permission", nil, false, false},
		{"read-only toolset, no permission", []string{"browse"}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.verify.acct.CanUpload = tt.canUpload
			opts := yes()
			opts.ToolsetsSet, opts.Toolsets = true, tt.toolsets

			out, err := f.run(t, "", opts)

			require.NoError(t, err)
			if tt.warns {
				assert.Contains(t, out, note)
				assert.NotContains(t, out, "Update permission", "the user may edit, only uploading is missing")
			} else {
				assert.NotContains(t, out, "Upload permission")
			}
		})
	}
}

func TestRun_YesWithFlags(t *testing.T) {
	f := newFixture(t)

	out, err := f.run(t, "", yes())

	require.NoError(t, err)
	assert.Equal(t, config.Config{URL: testURL, Token: testToken}, f.saved(t))
	assert.Equal(t, []agents.Entry{testEntry}, f.a.added)
	assert.Equal(t, []agents.Entry{testEntry}, f.b.added)
	for _, prompt := range []string{"Audiobookshelf URL", "API token", "[y/N]", "[Y/n]", "Press Enter", "1. Fake A"} {
		assert.NotContains(t, out, prompt)
	}
	assert.Contains(t, out, "Saved settings to ")
}

func TestRun_InteractiveAgentChoice(t *testing.T) {
	tests := []struct {
		answer     string
		wantA      int
		wantB      int
		wantErrMsg string
	}{
		{answer: "", wantA: 1, wantB: 1},
		{answer: "1", wantA: 1},
		{answer: "2", wantB: 1},
		{answer: "2,1", wantA: 1, wantB: 1},
		{answer: " 1 , 2 ", wantA: 1, wantB: 1},
		{answer: "1 2", wantA: 1, wantB: 1},
		{answer: "1,1", wantA: 1},
		{answer: "none"},
		{answer: "NONE"},
		{answer: "3", wantErrMsg: `invalid choice "3"`},
		{answer: "0", wantErrMsg: `invalid choice "0"`},
		{answer: "x", wantErrMsg: `invalid choice "x"`},
		{answer: ",", wantErrMsg: `invalid choice ","`},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.answer), func(t *testing.T) {
			f := newFixture(t)
			// Fake C is not installed, so it is never listed and 3 is out of range.
			f.deps.Agents = append(f.deps.Agents, &fakeAgent{id: "fake-c", name: "Fake C"})

			out, err := f.run(t, tt.answer+"\n", Options{URL: testURL, Token: testToken, ToolsetsSet: true})

			assert.Contains(t, out, "  1. Fake A\n  2. Fake B\nAdd audiobookshelf to all of these?")
			assert.NotContains(t, out, "Fake C")
			if tt.wantErrMsg != "" {
				require.ErrorContains(t, err, tt.wantErrMsg)
			} else {
				require.NoError(t, err)
			}
			assert.Len(t, f.a.added, tt.wantA)
			assert.Len(t, f.b.added, tt.wantB)
		})
	}
}

func TestRun_AgentsFlagPicksByID(t *testing.T) {
	f := newFixture(t)
	opts := yes()
	opts.Agents = []string{"fake-b"}

	out, err := f.run(t, "", opts)

	require.NoError(t, err)
	assert.Empty(t, f.a.added)
	assert.Equal(t, []agents.Entry{testEntry}, f.b.added)
	assert.Contains(t, out, "✓ Fake B: added.")
	assert.NotContains(t, out, "Fake A")
}

func TestRun_AgentsFlagBeatsThePromptWhenInteractive(t *testing.T) {
	f := newFixture(t)

	out, err := f.run(t, "", Options{URL: testURL, Token: testToken, ToolsetsSet: true, Agents: []string{"fake-a", "fake-a"}})

	require.NoError(t, err)
	assert.NotContains(t, out, "Press Enter")
	assert.Len(t, f.a.added, 1)
	assert.Empty(t, f.b.added)
}

func TestRun_AgentsFlagUndetected(t *testing.T) {
	f := newFixture(t)
	f.b.detected = false
	opts := yes()

	opts.Agents = []string{"fake-b"}
	_, err := f.run(t, "", opts)
	require.EqualError(t, err, "Fake B was not found on this machine")

	// One bad ID stops the whole step: the good one is not added either.
	opts.Agents = []string{"fake-a", "fake-b"}
	_, err = f.run(t, "", opts)
	require.EqualError(t, err, "Fake B was not found on this machine")

	assert.Empty(t, f.a.added)
	assert.Empty(t, f.b.added)
}

func TestRun_AgentsNone(t *testing.T) {
	f := newFixture(t)
	opts := yes()
	opts.Agents = []string{"none"}

	out, err := f.run(t, "", opts)

	require.NoError(t, err)
	assert.Empty(t, f.a.added)
	assert.Empty(t, f.b.added)
	assert.NotContains(t, out, "No supported agents found")
	assert.Equal(t, config.Config{URL: testURL, Token: testToken}, f.saved(t))
}

func TestRun_NoAgentsDetectedPrintsManualEntry(t *testing.T) {
	tests := []struct {
		name  string
		entry agents.Entry
		want  string
	}{
		{
			name:  "npx",
			entry: testEntry,
			want:  `No supported agents found. Add this server to your agent by hand: command npx, args ["-y","@sierikov/audiobookshelf-mcp@0.3.0"].`,
		},
		{
			name:  "no args",
			entry: agents.Entry{Command: "/opt/homebrew/bin/audiobookshelf-mcp"},
			want:  `No supported agents found. Add this server to your agent by hand: command /opt/homebrew/bin/audiobookshelf-mcp, args [].`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.a.detected, f.b.detected = false, false
			f.deps.Entry = tt.entry

			// Interactive: the agent prompt must not appear when there is nothing to pick.
			out, err := f.run(t, "n\n", Options{URL: testURL, Token: testToken})

			require.NoError(t, err)
			assert.Contains(t, out, tt.want)
			assert.NotContains(t, out, "Press Enter for all")
			assert.Empty(t, f.a.added)
			assert.Empty(t, f.b.added)
		})
	}
}

func TestRun_OneAgentFailsOthersContinue(t *testing.T) {
	tests := []struct {
		name         string
		aErr, bErr   error
		wantErr      string
		wantOutLines []string
	}{
		{
			name:         "first fails",
			aErr:         errors.New("boom"),
			wantErr:      "setup finished with errors for: Fake A",
			wantOutLines: []string{"✗ Fake A: boom", "✓ Fake B: added."},
		},
		{
			name:         "both fail",
			aErr:         errors.New("boom"),
			bErr:         errors.New("bang"),
			wantErr:      "setup finished with errors for: Fake A, Fake B",
			wantOutLines: []string{"✗ Fake A: boom", "✗ Fake B: bang"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.a.err, f.b.err = tt.aErr, tt.bErr

			out, err := f.run(t, "", yes())

			require.EqualError(t, err, tt.wantErr)
			for _, line := range tt.wantOutLines {
				assert.Contains(t, out, line)
			}
			assert.Len(t, f.a.added, 1)
			assert.Len(t, f.b.added, 1)
			// The settings are saved before the agents are touched.
			assert.Equal(t, testURL, f.saved(t).URL)
		})
	}
}

func TestRun_ReportsRestartHintAndBackup(t *testing.T) {
	f := newFixture(t)
	f.a.result = agents.Result{Backup: "/home/me/a.json.bak-1", Restart: "Quit and reopen Fake A."}
	f.b.result = agents.Result{Restart: "Restart Fake B."}

	out, err := f.run(t, "", yes())

	require.NoError(t, err)
	assert.Contains(t, out, "✓ Fake A: added. Quit and reopen Fake A. (backup: /home/me/a.json.bak-1)\n")
	assert.Contains(t, out, "✓ Fake B: added. Restart Fake B.\n")
}

func TestRun_CorruptSettingsFileIsAnError(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(f.deps.ConfigPath), 0o700))
	require.NoError(t, os.WriteFile(f.deps.ConfigPath, []byte("{not json"), 0o600))

	_, err := f.run(t, "", yes())

	require.ErrorContains(t, err, "parsing "+f.deps.ConfigPath)
	assert.Empty(t, f.verify.calls)
	assert.Empty(t, f.a.added)
}

func TestRun_EndOfInputAborts(t *testing.T) {
	const wantErr = "input ended before setup finished; " +
		"pass --yes to accept the defaults, and --agents to choose which agents to change"
	tests := []struct {
		name  string
		input string
	}{
		{"no input at all", ""},
		{"input ends at the toolsets prompt", "\n\n"},
		{"input ends at the agent prompt", "\n\n\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.seed(t, config.Config{URL: testURL, Token: "saved-token"})

			_, err := f.run(t, tt.input, Options{})

			require.EqualError(t, err, wantErr)
			assert.Empty(t, f.a.added)
			assert.Empty(t, f.b.added)
		})
	}
}

func TestRun_LastLineWithoutNewlineIsAnAnswer(t *testing.T) {
	f := newFixture(t)

	out, err := f.run(t, "https://abs.example\nsecret\nn\n2", Options{})

	require.NoError(t, err)
	assert.Equal(t, config.Config{URL: testURL, Token: "secret"}, f.saved(t))
	assert.Empty(t, f.a.added)
	assert.Equal(t, []agents.Entry{testEntry}, f.b.added)
	assert.Contains(t, out, "✓ Fake B: added.")
}

func TestRun_WarnsAboutANonDefaultXDGConfigHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("XDG_CONFIG_HOME does not decide the settings location on Windows")
	}
	const home = "/home/me"
	note := "Note: settings are under XDG_CONFIG_HOME (/data/xdg). Apps started outside your shell, " +
		"such as Claude Desktop or Cursor, may not see that variable and will look in " +
		filepath.Join(home, ".config", "audiobookshelf-mcp", "config.json") + " instead. " +
		"If one says the server is not configured, unset XDG_CONFIG_HOME and run setup again.\n"
	tests := []struct {
		name  string
		env   map[string]string
		warns bool
	}{
		{"non-default XDG_CONFIG_HOME", map[string]string{"HOME": home, "XDG_CONFIG_HOME": "/data/xdg"}, true},
		{"XDG_CONFIG_HOME unset", map[string]string{"HOME": home}, false},
		{"XDG_CONFIG_HOME is the default", map[string]string{"HOME": home, "XDG_CONFIG_HOME": home + "/.config"}, false},
		{"XDG_CONFIG_HOME is the default with a trailing slash", map[string]string{"HOME": home, "XDG_CONFIG_HOME": home + "/.config/"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.deps.Getenv = func(key string) string { return tt.env[key] }

			out, err := f.run(t, "", yes())

			require.NoError(t, err)
			if tt.warns {
				assert.Contains(t, out, "Saved settings to "+f.deps.ConfigPath+".\n"+note)
			} else {
				assert.NotContains(t, out, "Note: settings are under")
			}
		})
	}

	t.Run("no Getenv", func(t *testing.T) {
		f := newFixture(t)

		out, err := f.run(t, "", yes())

		require.NoError(t, err)
		assert.NotContains(t, out, "Note: settings are under")
	})
}

func TestRun_UnknownAgentIDFailsBeforeConnecting(t *testing.T) {
	for name, opts := range map[string]Options{
		"yes":         {Agents: []string{"bogus"}, Yes: true, URL: testURL, Token: testToken},
		"interactive": {Agents: []string{"fake-a", "bogus"}},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)

			out, err := f.run(t, "", opts)

			require.EqualError(t, err, `unknown agent "bogus"; known: fake-a, fake-b`)
			assert.Empty(t, out, "nothing is asked")
			assert.Empty(t, f.verify.calls)
			f.requireNothingSaved(t)
		})
	}
}

func TestVerifyWith_ShortensLongErrors(t *testing.T) {
	page := strings.Repeat("not found. ", 128)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusNotFound)
		// Like a proxy that echoes the credentials back, with the token across
		// the 300-byte cut: "checking the token: ABS API 404: " is 33 bytes and
		// "Bearer " 7, so the token starts at byte 295.
		_, _ = io.WriteString(w, page[:255]+"Bearer "+testToken+page[255:])
	}))
	t.Cleanup(srv.Close)

	_, err := VerifyWith(t.Context(), srv.URL, testToken)

	require.Error(t, err)
	msg := err.Error()
	assert.LessOrEqual(t, len(msg), 304)
	assert.True(t, strings.HasPrefix(msg, "checking the token: ABS API 404: not found. "), msg)
	assert.True(t, strings.HasSuffix(msg, "…"), msg)
	assert.True(t, utf8.ValidString(msg))
	assert.NotContains(t, msg, testToken[:5], "no part of the token survives the cut")
}

func TestVerifyWith_ReadsUpload(t *testing.T) {
	tests := []struct {
		name        string
		permissions map[string]any
		want        Account
	}{
		{
			"update but no upload",
			map[string]any{"update": true, "upload": false},
			Account{Username: "artem", Libraries: 1, CanUpdate: true},
		},
		{
			"upload but no update",
			map[string]any{"update": false, "upload": true},
			Account{Username: "artem", Libraries: 1, CanUpload: true},
		},
		{
			"both",
			map[string]any{"update": true, "upload": true},
			Account{Username: "artem", Libraries: 1, CanUpdate: true, CanUpload: true},
		},
		{"neither", map[string]any{}, Account{Username: "artem", Libraries: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := abstest.NewServer(t, map[string]any{
				"/api/me":        map[string]any{"id": "u1", "username": "artem", "permissions": tt.permissions},
				"/api/libraries": map[string]any{"libraries": []map[string]any{{"id": "lib1"}}},
			})

			got, err := VerifyWith(t.Context(), srv.URL, abstest.Token)

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestVerifyWith(t *testing.T) {
	srv := abstest.NewServer(t, map[string]any{
		"/api/me": map[string]any{
			"id":          "u1",
			"username":    "artem",
			"permissions": map[string]any{"update": true},
		},
		"/api/libraries": map[string]any{
			"libraries": []map[string]any{{"id": "lib1"}, {"id": "lib2"}},
		},
	})

	t.Run("valid token", func(t *testing.T) {
		got, err := VerifyWith(t.Context(), srv.URL, abstest.Token)

		require.NoError(t, err)
		assert.Equal(t, Account{Username: "artem", Libraries: 2, CanUpdate: true}, got)
	})
	t.Run("rejected token", func(t *testing.T) {
		_, err := VerifyWith(t.Context(), srv.URL, "wrong-token")

		require.ErrorContains(t, err, "401")
		assert.NotContains(t, err.Error(), "wrong-token")
	})
	t.Run("nothing listening", func(t *testing.T) {
		dead := abstest.NewServer(t, nil)
		addr := dead.URL
		dead.Close()

		_, err := VerifyWith(t.Context(), addr, abstest.Token)

		require.Error(t, err)
	})
}
