package agents

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseTOML parses text and returns the mcp_servers table along with the whole
// document.
func parseTOML(t *testing.T, text string) (doc, servers map[string]any) {
	t.Helper()
	require.NoError(t, toml.Unmarshal([]byte(text), &doc))
	servers, ok := doc["mcp_servers"].(map[string]any)
	require.True(t, ok, "mcp_servers is a table")
	return doc, servers
}

func TestCodex_AppendsAndReplacesBlock(t *testing.T) {
	t.Run("replaces the existing block", func(t *testing.T) {
		env := fakeEnv(t, "darwin", nil, "codex")
		path := filepath.Join(env.Home, ".codex", "config.toml")
		original := `model = "o3"   # comment kept

[mcp_servers.codegraph]
command = "codegraph"

[mcp_servers.audiobookshelf]
command = "/old"

[mcp_servers.audiobookshelf.env]
AUDIOBOOKSHELF_TOKEN = "t"

# profiles follow
[profiles.x]
a = 1
`
		writeFile(t, path, original)

		res, err := agentByID(t, env, "codex").Add(t.Context(), npxEntry())

		require.NoError(t, err)
		got := readFile(t, path)
		assert.Contains(t, got, "model = \"o3\"   # comment kept\n")
		assert.Contains(t, got, "\n# profiles follow\n[profiles.x]\n")
		assert.Equal(t, 1, strings.Count(got, "[mcp_servers.audiobookshelf]"))
		assert.NotContains(t, got, "AUDIOBOOKSHELF_TOKEN")
		doc, servers := parseTOML(t, got)
		assert.Equal(t, "o3", doc["model"])
		assert.Equal(t, map[string]any{"command": "codegraph"}, servers["codegraph"])
		assert.Equal(t, map[string]any{
			"command": "npx",
			"args":    []any{"-y", "@sierikov/audiobookshelf-mcp@0.3.0"},
		}, servers["audiobookshelf"])
		assert.Equal(t, map[string]any{"x": map[string]any{"a": int64(1)}}, doc["profiles"])
		assert.Equal(t, path+".bak-"+testStamp, res.Backup)
		assert.Equal(t, original, readFile(t, res.Backup))
		assert.Equal(t, "Restart Codex.", res.Restart)
	})

	t.Run("appends a Windows path that round-trips", func(t *testing.T) {
		env := fakeEnv(t, "windows", nil, "codex")
		path := filepath.Join(env.Home, ".codex", "config.toml")
		writeFile(t, path, "model = \"o3\"")
		exe := `C:\Users\me\AppData\Local\audiobookshelf-mcp\audiobookshelf-mcp.exe`

		_, err := agentByID(t, env, "codex").Add(t.Context(), Entry{Command: exe})

		require.NoError(t, err)
		got := readFile(t, path)
		assert.True(t, strings.HasPrefix(got, "model = \"o3\"\n\n[mcp_servers.audiobookshelf]\n"), got)
		_, servers := parseTOML(t, got)
		assert.Equal(t, map[string]any{"command": exe, "args": []any{}}, servers["audiobookshelf"])
	})

	t.Run("uses CODEX_HOME", func(t *testing.T) {
		codexHome := filepath.Join(t.TempDir(), "codex-home")
		env := fakeEnv(t, "darwin", map[string]string{"CODEX_HOME": codexHome}, "codex")

		res, err := agentByID(t, env, "codex").Add(t.Context(), npxEntry())

		require.NoError(t, err)
		assert.Empty(t, res.Backup)
		_, servers := parseTOML(t, readFile(t, filepath.Join(codexHome, "config.toml")))
		assert.Contains(t, servers, "audiobookshelf")
		assert.NoFileExists(t, filepath.Join(env.Home, ".codex", "config.toml"))
	})

	t.Run("escapes quotes and control characters", func(t *testing.T) {
		env := fakeEnv(t, "darwin", nil, "codex")
		e := Entry{Command: `/opt/my "tools"/abs`, Args: []string{"tab\there", "<&>"}}

		_, err := agentByID(t, env, "codex").Add(t.Context(), e)

		require.NoError(t, err)
		_, servers := parseTOML(t, readFile(t, filepath.Join(env.Home, ".codex", "config.toml")))
		assert.Equal(t, map[string]any{
			"command": e.Command,
			"args":    []any{"tab\there", "<&>"},
		}, servers["audiobookshelf"])
	})

	t.Run("escapes DEL, which TOML forbids in a string", func(t *testing.T) {
		env := fakeEnv(t, "darwin", nil, "codex")
		e := Entry{Command: "/opt/abs\u007fmcp", Args: []string{"a\u007f"}}

		_, err := agentByID(t, env, "codex").Add(t.Context(), e)

		require.NoError(t, err)
		_, servers := parseTOML(t, readFile(t, filepath.Join(env.Home, ".codex", "config.toml")))
		assert.Equal(t, map[string]any{
			"command": e.Command,
			"args":    []any{"a\u007f"},
		}, servers["audiobookshelf"])
	})
}

func TestCodex_InvalidFileUntouched(t *testing.T) {
	cases := map[string]string{
		"not TOML": "model = \n[[",
		// Defined inline, so appending a table would redefine it.
		"entry defined another way": "[mcp_servers]\naudiobookshelf = { command = \"/old\" }\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			env := fakeEnv(t, "darwin", nil, "codex")
			path := filepath.Join(env.Home, ".codex", "config.toml")
			writeFile(t, path, content)

			_, err := agentByID(t, env, "codex").Add(t.Context(), npxEntry())

			require.Error(t, err)
			assert.Contains(t, err.Error(), path)
			assert.Equal(t, content, readFile(t, path))
			assert.Empty(t, backups(t, path))
		})
	}
}

func TestCodex_RefusesWhenAStaleEntrySurvives(t *testing.T) {
	env := fakeEnv(t, "darwin", nil, "codex")
	path := filepath.Join(env.Home, ".codex", "config.toml")
	// The env subtable is not next to our table, so replacing the table would
	// leave it, and the token, behind.
	original := `[mcp_servers.audiobookshelf]
command = "/old"

[profiles.x]
a = 1

[mcp_servers.audiobookshelf.env]
AUDIOBOOKSHELF_TOKEN = "t"
`
	writeFile(t, path, original)

	_, err := agentByID(t, env, "codex").Add(t.Context(), npxEntry())

	require.Error(t, err)
	assert.Contains(t, err.Error(), path)
	assert.Contains(t, err.Error(), "[mcp_servers.audiobookshelf…]")
	assert.Contains(t, err.Error(), "by hand")
	assert.NotContains(t, err.Error(), `"t"`)
	assert.Equal(t, original, readFile(t, path))
	assert.Empty(t, backups(t, path))
}
