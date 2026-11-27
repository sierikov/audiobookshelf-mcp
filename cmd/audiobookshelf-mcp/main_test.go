package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
	"github.com/sierikov/audiobookshelf-mcp/internal/abs/abstest"
	"github.com/sierikov/audiobookshelf-mcp/internal/buildinfo"
	"github.com/sierikov/audiobookshelf-mcp/internal/config"
	"github.com/sierikov/audiobookshelf-mcp/internal/gemini"
	"github.com/sierikov/audiobookshelf-mcp/internal/setup"
	"github.com/sierikov/audiobookshelf-mcp/internal/tools"
	"github.com/sierikov/audiobookshelf-mcp/internal/update"
)

// sandbox points the settings file, the update cache and the home directory
// at fresh temporary directories, so a test never reads or writes the real
// ones on any OS, and switches the update check off. It returns the settings
// file path.
func sandbox(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	// Windows: os.UserHomeDir, os.UserConfigDir and os.UserCacheDir, and the
	// agents' config folders.
	t.Setenv("USERPROFILE", home)
	t.Setenv("AppData", t.TempDir())
	t.Setenv("LocalAppData", t.TempDir())
	t.Setenv("AUDIOBOOKSHELF_URL", "")
	t.Setenv("AUDIOBOOKSHELF_TOKEN", "")
	t.Setenv("AUDIOBOOKSHELF_TOOLSETS", "")
	t.Setenv("AUDIOBOOKSHELF_MCP_GEMINI_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("AUDIOBOOKSHELF_MCP_NO_UPDATE_CHECK", "1")
	path, err := config.Path()
	require.NoError(t, err)
	return path
}

// runCmd runs the command line with the given standard input and returns the
// exit code together with what the command printed to stdout and stderr.
func runCmd(t *testing.T, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(t.Context(), args, strings.NewReader(stdin), &out, &errOut)
	return code, out.String(), errOut.String()
}

// fakeAudiobookshelf is a server that accepts abstest.Token and knows one user
// with two libraries.
func fakeAudiobookshelf(t *testing.T) *abstest.Server {
	t.Helper()
	return abstest.NewServer(t, map[string]any{
		"/api/me": map[string]any{
			"id":          "u1",
			"username":    "artem",
			"permissions": map[string]any{"update": true},
		},
		"/api/libraries": map[string]any{
			"libraries": []map[string]any{{"id": "lib1"}, {"id": "lib2"}},
		},
	})
}

func TestRun_Help(t *testing.T) {
	sandbox(t)
	for _, arg := range []string{"help", "-h", "--help"} {
		t.Run(arg, func(t *testing.T) {
			code, stdout, stderr := runCmd(t, "", arg)

			assert.Equal(t, 0, code)
			assert.Contains(t, stdout, "setup")
			assert.Contains(t, stdout, "version")
			assert.Contains(t, stdout, "help")
			assert.Empty(t, stderr)
		})
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	sandbox(t)

	code, stdout, stderr := runCmd(t, "", "frobnicate")

	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "usage")
	assert.Contains(t, stderr, "frobnicate")
	assert.Empty(t, stdout)
}

func TestRun_Version(t *testing.T) {
	sandbox(t)
	for _, arg := range []string{"version", "--version"} {
		t.Run(arg, func(t *testing.T) {
			code, stdout, stderr := runCmd(t, "", arg)

			assert.Equal(t, 0, code)
			assert.Equal(t, "audiobookshelf-mcp "+buildinfo.Version()+"\n", stdout)
			assert.Empty(t, stderr)
		})
	}
}

func TestRun_ServeUnconfigured(t *testing.T) {
	sandbox(t)

	code, stdout, stderr := runCmd(t, "")

	assert.NotEqual(t, 0, code)
	assert.Empty(t, stdout, "stdout belongs to the MCP channel")
	assert.Equal(t, "error: audiobookshelf-mcp is not configured: "+
		`run "audiobookshelf-mcp setup" (with npx: "npx -y @sierikov/audiobookshelf-mcp setup"), `+
		`or set AUDIOBOOKSHELF_URL and AUDIOBOOKSHELF_TOKEN`+"\n", stderr)
}

func TestRun_ServeWithTokenMissing(t *testing.T) {
	path := sandbox(t)
	require.NoError(t, config.Save(path, config.Config{URL: "https://abs.example"}))

	code, _, stderr := runCmd(t, "")

	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr, `run "audiobookshelf-mcp setup"`)
	assert.Contains(t, stderr, `"npx -y @sierikov/audiobookshelf-mcp setup"`)
}

func TestRun_ServeUnconfiguredNamesUnknownSettingsLocation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the settings location always resolves through %AppData% on Windows")
	}
	sandbox(t)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")

	code, _, stderr := runCmd(t, "")

	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr, `run "audiobookshelf-mcp setup"`)
	assert.Contains(t, stderr, `"npx -y @sierikov/audiobookshelf-mcp setup"`)
	assert.Contains(t, stderr, "settings location unknown")
}

func TestRun_ServeBrokenSettingsFile(t *testing.T) {
	path := sandbox(t)
	require.NoError(t, config.Save(path, config.Config{URL: "https://abs.example", Token: "tok"}))
	require.NoError(t, os.WriteFile(path, []byte("{not json"), 0o600))

	code, _, stderr := runCmd(t, "")

	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "error: parsing")
}

// toolNames lists the tools a client sees on the server that tools.NewServer
// builds from svcs, with every toolset on.
func toolNames(t *testing.T, svcs tools.Services) []string {
	t.Helper()
	server := tools.NewServer(svcs, tools.Options{Toolsets: []string{"all"}})
	st, ct := mcp.NewInMemoryTransports()
	_, err := server.Connect(t.Context(), st, nil)
	require.NoError(t, err)
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(t.Context(), ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	var names []string
	for tool, err := range cs.Tools(t.Context(), nil) {
		require.NoError(t, err)
		names = append(names, tool.Name)
	}
	return names
}

func TestServe_RegistersCoverArtOnlyWithAKey(t *testing.T) {
	cfg := config.Config{URL: "https://abs.example", Token: "tok"}
	client := abs.NewClient(cfg.URL, cfg.Token)

	t.Run("without a key", func(t *testing.T) {
		names := toolNames(t, newServices(cfg, client))

		assert.NotContains(t, names, "generate_covers")
		assert.NotContains(t, names, "clean_cover")
		assert.Contains(t, names, "get_cover_prompt")
		assert.Contains(t, names, "set_cover")
	})

	t.Run("with a key", func(t *testing.T) {
		withKey := cfg
		withKey.GeminiKey = "gem-key"

		names := toolNames(t, newServices(withKey, client))

		assert.Contains(t, names, "generate_covers")
		assert.Contains(t, names, "clean_cover")
		assert.Contains(t, names, "get_cover_prompt")
		assert.Contains(t, names, "set_cover")
	})

	t.Run("without a key the service is a nil interface", func(t *testing.T) {
		// A nil *service.CoverArt in the interface would not be nil, and would
		// switch the paid tools on, so this compares the interface itself.
		artIsNil := newServices(cfg, client).CoverArt == nil
		assert.True(t, artIsNil)
	})
}

func TestLoadConfig_GeminiKeyComesFromTheFileOrTheServersOwnVariable(t *testing.T) {
	path := sandbox(t)
	require.NoError(t, config.Save(path, config.Config{URL: "https://abs.example", Token: "tok", GeminiKey: "from-file"}))

	t.Run("the file", func(t *testing.T) {
		cfg, err := loadConfig()

		require.NoError(t, err)
		assert.Equal(t, "from-file", cfg.GeminiKey)
	})
	t.Run("the variable replaces the file", func(t *testing.T) {
		t.Setenv("AUDIOBOOKSHELF_MCP_GEMINI_API_KEY", "from-env")

		cfg, err := loadConfig()

		require.NoError(t, err)
		assert.Equal(t, "from-env", cfg.GeminiKey)
	})
	t.Run("GEMINI_API_KEY is never read", func(t *testing.T) {
		require.NoError(t, config.Save(path, config.Config{URL: "https://abs.example", Token: "tok"}))
		t.Setenv("GEMINI_API_KEY", "somebody-elses")

		cfg, err := loadConfig()

		require.NoError(t, err)
		assert.Empty(t, cfg.GeminiKey)
	})
}

func TestGeminiKeyChecker(t *testing.T) {
	const goodKey = "good-gem-key"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/models/"+string(gemini.ModelPro) && r.Header.Get("x-goog-api-key") == goodKey {
			_, _ = w.Write([]byte(`{"name":"models/` + string(gemini.ModelPro) + `"}`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":400,"message":"API key not valid.","status":"INVALID_ARGUMENT"}}`))
	}))
	t.Cleanup(srv.Close)
	check := geminiKeyChecker(srv.URL)

	require.NoError(t, check(t.Context(), goodKey))

	err := check(t.Context(), "bad-gem-key")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "bad-gem-key")
}

func TestSetupDeps_ChecksTheGeminiKey(t *testing.T) {
	sandbox(t)

	deps, err := setupDeps(strings.NewReader(""), &bytes.Buffer{})

	require.NoError(t, err)
	assert.NotNil(t, deps.CheckGeminiKey)
}

func TestRun_SetupSavesSettings(t *testing.T) {
	path := sandbox(t)
	srv := fakeAudiobookshelf(t)

	code, stdout, stderr := runCmd(t, "",
		"setup", "--url", srv.URL, "--token", abstest.Token,
		"--toolsets", "chapters, metadata,", "--agents", "none", "--yes")

	require.Equal(t, 0, code, "stderr: %s", stderr)
	assert.Contains(t, stdout, "Connected to "+srv.URL+" as artem, 2 libraries.")
	assert.Contains(t, stdout, "Saved settings to "+path)
	if runtime.GOOS != "windows" {
		// The sandbox's XDG_CONFIG_HOME is not $HOME/.config.
		assert.Contains(t, stdout, "Note: settings are under XDG_CONFIG_HOME (")
	}
	assert.NotContains(t, stdout+stderr, abstest.Token)
	saved, err := config.Load(path)
	require.NoError(t, err)
	assert.Equal(t, config.Config{URL: srv.URL, Token: abstest.Token, Toolsets: []string{"chapters", "metadata"}}, saved)
}

func TestRun_SetupWithoutToolsetsFlagKeepsSavedToolsets(t *testing.T) {
	path := sandbox(t)
	srv := fakeAudiobookshelf(t)
	require.NoError(t, config.Save(path, config.Config{URL: srv.URL, Token: abstest.Token, Toolsets: []string{"all"}}))

	code, _, stderr := runCmd(t, "", "setup", "--agents", "none", "--yes")

	require.Equal(t, 0, code, "stderr: %s", stderr)
	saved, err := config.Load(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"all"}, saved.Toolsets)
}

func TestRun_SetupEmptyToolsetsFlagClearsSavedToolsets(t *testing.T) {
	path := sandbox(t)
	srv := fakeAudiobookshelf(t)
	require.NoError(t, config.Save(path, config.Config{URL: srv.URL, Token: abstest.Token, Toolsets: []string{"all"}}))

	code, _, stderr := runCmd(t, "", "setup", "--toolsets", "", "--agents", "none", "--yes")

	require.Equal(t, 0, code, "stderr: %s", stderr)
	saved, err := config.Load(path)
	require.NoError(t, err)
	assert.Empty(t, saved.Toolsets)
}

func TestRun_SetupReadsTokenFromPipedInput(t *testing.T) {
	path := sandbox(t)
	srv := fakeAudiobookshelf(t)

	code, stdout, stderr := runCmd(t, abstest.Token+"\nn\n", "setup", "--url", srv.URL, "--agents", "none")

	require.Equal(t, 0, code, "stderr: %s", stderr)
	assert.NotContains(t, stdout+stderr, abstest.Token)
	saved, err := config.Load(path)
	require.NoError(t, err)
	assert.Equal(t, abstest.Token, saved.Token)
}

func TestRun_SetupConnectionFailureExitsOne(t *testing.T) {
	path := sandbox(t)
	srv := fakeAudiobookshelf(t)

	code, stdout, stderr := runCmd(t, "", "setup", "--url", srv.URL, "--token", "wrong-token", "--agents", "none", "--yes")

	assert.Equal(t, 1, code)
	assert.True(t, strings.HasPrefix(stderr, "error: could not connect"), "stderr: %s", stderr)
	assert.NotContains(t, stdout+stderr, "wrong-token")
	assert.NoFileExists(t, path)
}

func TestRun_SetupUsageErrors(t *testing.T) {
	sandbox(t)
	tests := map[string][]string{
		"unknown flag":        {"setup", "--nope"},
		"stray argument":      {"setup", "extra"},
		"flag without value":  {"setup", "--url"},
		"bad boolean literal": {"setup", "--yes=maybe"},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			code, stdout, stderr := runCmd(t, "", args...)

			assert.Equal(t, 2, code)
			assert.Empty(t, stdout)
			assert.Contains(t, stderr, "usage: audiobookshelf-mcp setup")
		})
	}
}

func TestRun_SetupHelp(t *testing.T) {
	sandbox(t)

	code, stdout, stderr := runCmd(t, "", "setup", "--help")

	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "usage: audiobookshelf-mcp setup")
	for _, flagName := range []string{"-url", "-token", "-toolsets", "-agents", "-yes"} {
		assert.Contains(t, stdout, flagName)
	}
	assert.Contains(t, stdout, "libraries,items,playback,browse,chapters,metadata,covers", "the --toolsets example is what answering y saves")
	assert.Contains(t, stdout, "Gemini API key", "setup asks for the key when covers are on")
	assert.Empty(t, stderr)
}

func TestParseSetupFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want setup.Options
	}{
		{name: "no flags", args: nil, want: setup.Options{}},
		{
			name: "everything",
			args: []string{"--url", "https://abs.example", "--token", "tok", "--toolsets", "all", "--agents", "cursor,codex", "--yes"},
			want: setup.Options{
				URL: "https://abs.example", Token: "tok",
				Toolsets: []string{"all"}, ToolsetsSet: true,
				Agents: []string{"cursor", "codex"}, Yes: true,
			},
		},
		{
			name: "single dash",
			args: []string{"-url", "https://abs.example", "-yes"},
			want: setup.Options{URL: "https://abs.example", Yes: true},
		},
		{name: "empty toolsets is still set", args: []string{"--toolsets", ""}, want: setup.Options{ToolsetsSet: true}},
		{
			name: "lists are trimmed and stripped of empties",
			args: []string{"--toolsets", " chapters , ,metadata,", "--agents", " cursor,, codex "},
			want: setup.Options{
				Toolsets: []string{"chapters", "metadata"}, ToolsetsSet: true,
				Agents: []string{"cursor", "codex"},
			},
		},
		{name: "agents none", args: []string{"--agents", "none"}, want: setup.Options{Agents: []string{"none"}}},
		{name: "empty agents is the same as absent", args: []string{"--agents", ""}, want: setup.Options{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSetupFlags(newSetupFlagSet(), tt.args)

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseSetupFlags_RejectsPositionalArguments(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"--yes", "tok123"}, "setup takes only flags (got 1 extra argument(s))"},
		{[]string{"--url", "x", "tok123", "--yes"}, "setup takes only flags (got 2 extra argument(s))"},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			_, err := parseSetupFlags(newSetupFlagSet(), tt.args)

			require.EqualError(t, err, tt.want)
		})
	}
}

func TestParseSetupFlags_RejectsUnknownToolsets(t *testing.T) {
	const known = "known: libraries, items, playback, browse, chapters, metadata, covers, all"
	tests := []struct {
		toolsets string
		want     string
	}{
		{"chapters,metdata", `unknown toolset "metdata"; ` + known},
		{"covers,bogus", `unknown toolset "bogus"; ` + known},
		{"nope", `unknown toolset "nope"; ` + known},
		{"All", `unknown toolset "All"; ` + known},
	}
	for _, tt := range tests {
		t.Run(tt.toolsets, func(t *testing.T) {
			_, err := parseSetupFlags(newSetupFlagSet(), []string{"--toolsets", tt.toolsets})

			require.EqualError(t, err, tt.want)
		})
	}
}

func TestRun_SetupUnknownToolsetIsAUsageError(t *testing.T) {
	path := sandbox(t)

	code, stdout, stderr := runCmd(t, "", "setup", "--toolsets", "chapters,metdata", "--yes")

	assert.Equal(t, 2, code)
	assert.Empty(t, stdout)
	assert.True(t, strings.HasPrefix(stderr, "error: unknown toolset \"metdata\"; "+
		"known: libraries, items, playback, browse, chapters, metadata, covers, all\n\n"), "stderr: %s", stderr)
	assert.Contains(t, stderr, "usage: audiobookshelf-mcp setup")
	assert.NoFileExists(t, path)
}

func TestRun_SetupNeverEchoesAStrayArgument(t *testing.T) {
	sandbox(t)

	code, stdout, stderr := runCmd(t, "", "setup", "--url", "x", "secret")

	assert.Equal(t, 2, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "setup takes only flags (got 1 extra argument(s))")
	assert.NotContains(t, stderr, "secret")
}

// fixedNow is the clock of the update tests.
var fixedNow = time.Date(2026, time.November, 1, 12, 0, 0, 0, time.UTC)

// currentRelease is the version the update tests pretend to run, and
// newerRelease the release their fake lookups find.
const (
	currentRelease = "0.2.0"
	newerRelease   = "0.3.0"
)

// fakeChecker returns an update checker for release 0.2.0 with a cache file in
// a temporary directory, a fixed clock and the given lookup, so no test
// reaches the network.
func fakeChecker(t *testing.T, fetch func(context.Context) (string, error)) *update.Checker {
	t.Helper()
	return &update.Checker{
		Current:   currentRelease,
		CachePath: filepath.Join(t.TempDir(), "update-check.json"),
		Now:       func() time.Time { return fixedNow },
		Fetch:     fetch,
	}
}

// fetching returns a lookup that finds the given release.
func fetching(release string) func(context.Context) (string, error) {
	return func(context.Context) (string, error) { return release, nil }
}

// installMethodHere is how the running test binary counts as installed, which
// decides the upgrade command in the notices.
func installMethodHere(t *testing.T) update.Method {
	t.Helper()
	_, method, err := detectInstall()
	require.NoError(t, err)
	return method
}

func TestReportUpdate_FetchedNewerReleasePrintsNotice(t *testing.T) {
	checker := fakeChecker(t, fetching(newerRelease))
	var out bytes.Buffer

	reportUpdate(t.Context(), &out, checker)

	assert.Equal(t, update.CLINotice(newerRelease, currentRelease, installMethodHere(t))+"\n", out.String())
}

func TestReportUpdate_CachedNewerReleasePrintsNoticeWithoutFetching(t *testing.T) {
	checker := fakeChecker(t, fetching(newerRelease))
	require.NoError(t, checker.Refresh(t.Context())) // fills the cache
	checker.Fetch = func(context.Context) (string, error) {
		assert.Fail(t, "looked the release up although the cache is fresh")
		return "", errors.New("unexpected lookup")
	}
	var out bytes.Buffer

	reportUpdate(t.Context(), &out, checker)

	assert.Equal(t, update.CLINotice(newerRelease, currentRelease, installMethodHere(t))+"\n", out.String())
}

func TestReportUpdate_FailedFetchPrintsNothing(t *testing.T) {
	checker := fakeChecker(t, func(context.Context) (string, error) { return "", errors.New("offline") })
	var out bytes.Buffer

	reportUpdate(t.Context(), &out, checker)

	assert.Empty(t, out.String())
}

func TestReportUpdate_FailedRefreshStillReportsWhatTheCacheKnows(t *testing.T) {
	checker := fakeChecker(t, fetching(newerRelease))
	require.NoError(t, checker.Refresh(t.Context()))
	checker.Now = func() time.Time { return fixedNow.Add(25 * time.Hour) } // the cache is stale now
	checker.Fetch = func(context.Context) (string, error) { return "", errors.New("offline") }
	var out bytes.Buffer

	reportUpdate(t.Context(), &out, checker)

	assert.Equal(t, update.CLINotice(newerRelease, currentRelease, installMethodHere(t))+"\n", out.String())
}

func TestReportUpdate_NothingNewerPrintsNothing(t *testing.T) {
	for _, latest := range []string{currentRelease, "0.1.0"} {
		t.Run(latest, func(t *testing.T) {
			checker := fakeChecker(t, fetching(latest))
			var out bytes.Buffer

			reportUpdate(t.Context(), &out, checker)

			assert.Empty(t, out.String())
		})
	}
}

func TestNewChecker(t *testing.T) {
	sandbox(t)
	const pseudoVersion = "0.1.1-0.20261031223530-f456434fef54+dirty"
	tests := []struct {
		name    string
		version string
		env     map[string]string
		want    bool
	}{
		{name: "release", version: currentRelease, want: true},
		{name: "opted out", version: currentRelease, env: map[string]string{update.OptOutEnv: "1"}},
		{name: "dev build", version: "dev"},
		{name: "build from a commit", version: pseudoVersion},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker, ok := newChecker(tt.version, func(key string) string { return tt.env[key] })

			require.Equal(t, tt.want, ok)
			if !ok {
				return
			}
			assert.Equal(t, tt.version, checker.Current)
			assert.Equal(t, "update-check.json", filepath.Base(checker.CachePath))
		})
	}
}

func TestUpdateInstructions(t *testing.T) {
	t.Run("cached newer release", func(t *testing.T) {
		checker := fakeChecker(t, fetching(newerRelease))
		require.NoError(t, checker.Refresh(t.Context())) // fills the cache
		checker.Fetch = func(context.Context) (string, error) {
			assert.Fail(t, "looked the release up, but the instructions only read the cache")
			return "", errors.New("unexpected lookup")
		}

		got := updateInstructions(checker)

		assert.Equal(t, update.AgentNotice(newerRelease, currentRelease, installMethodHere(t)), got)
	})
	t.Run("empty cache", func(t *testing.T) {
		checker := fakeChecker(t, func(context.Context) (string, error) {
			assert.Fail(t, "looked the release up, but the instructions only read the cache")
			return "", errors.New("unexpected lookup")
		})

		assert.Empty(t, updateInstructions(checker))
	})
	t.Run("cached release is not newer", func(t *testing.T) {
		checker := fakeChecker(t, fetching(currentRelease))
		require.NoError(t, checker.Refresh(t.Context()))

		assert.Empty(t, updateInstructions(checker))
	})
	t.Run("update check off", func(t *testing.T) {
		assert.Empty(t, updateInstructions(nil))
	})
}
