package agents

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func parseYAML(t *testing.T, text string) map[string]any {
	t.Helper()
	var doc map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(text), &doc))
	return doc
}

func TestHermes_SetsServerKeepsRest(t *testing.T) {
	t.Run("existing file", func(t *testing.T) {
		env := fakeEnv(t, "darwin", nil, "hermes")
		path := filepath.Join(env.Home, ".hermes", "config.yaml")
		original := `# Hermes settings

model: x # the default model
mcp_servers:
  other:
    command: o
  audiobookshelf:
    command: /old
    env:
      AUDIOBOOKSHELF_TOKEN: t
`
		writeFile(t, path, original)

		res, err := agentByID(t, env, "hermes").Add(t.Context(), npxEntry())

		require.NoError(t, err)
		got := readFile(t, path)
		assert.Contains(t, got, "# Hermes settings\n")
		assert.Contains(t, got, "model: x # the default model\n")
		assert.NotContains(t, got, "AUDIOBOOKSHELF_TOKEN")
		doc := parseYAML(t, got)
		assert.Equal(t, "x", doc["model"])
		assert.Equal(t, map[string]any{
			"other": map[string]any{"command": "o"},
			"audiobookshelf": map[string]any{
				"command": "npx",
				"args":    []any{"-y", "@sierikov/audiobookshelf-mcp@0.3.0"},
				"enabled": true,
			},
		}, doc["mcp_servers"])
		assert.Equal(t, path+".bak-"+testStamp, res.Backup)
		assert.Equal(t, original, readFile(t, res.Backup))
		assert.Equal(t, "Restart Hermes Agent.", res.Restart)
	})

	t.Run("missing file in an existing home", func(t *testing.T) {
		env := fakeEnv(t, "darwin", nil)
		home := filepath.Join(env.Home, ".hermes")
		mkdir(t, home)

		res, err := agentByID(t, env, "hermes").Add(t.Context(), Entry{Command: "/usr/local/bin/audiobookshelf-mcp"})

		require.NoError(t, err)
		assert.Empty(t, res.Backup)
		assert.Equal(t, map[string]any{
			"mcp_servers": map[string]any{
				"audiobookshelf": map[string]any{
					"command": "/usr/local/bin/audiobookshelf-mcp",
					"args":    []any{},
					"enabled": true,
				},
			},
		}, parseYAML(t, readFile(t, filepath.Join(home, "config.yaml"))))
	})

	t.Run("empty mcp_servers", func(t *testing.T) {
		env := fakeEnv(t, "darwin", nil, "hermes")
		path := filepath.Join(env.Home, ".hermes", "config.yaml")
		writeFile(t, path, "model: x\nmcp_servers:\n")

		_, err := agentByID(t, env, "hermes").Add(t.Context(), npxEntry())

		require.NoError(t, err)
		doc := parseYAML(t, readFile(t, path))
		assert.Equal(t, "x", doc["model"])
		assert.Contains(t, doc["mcp_servers"], "audiobookshelf")
	})

	t.Run("comment-only file keeps its comments", func(t *testing.T) {
		for name, original := range map[string]string{
			"trailing newline":    "# Hermes settings\n# model: x\n",
			"no trailing newline": "# Hermes settings\n# model: x",
		} {
			t.Run(name, func(t *testing.T) {
				env := fakeEnv(t, "darwin", nil, "hermes")
				path := filepath.Join(env.Home, ".hermes", "config.yaml")
				writeFile(t, path, original)

				_, err := agentByID(t, env, "hermes").Add(t.Context(), npxEntry())

				require.NoError(t, err)
				got := readFile(t, path)
				assert.True(t, strings.HasPrefix(got, "# Hermes settings\n# model: x\n"), got)
				assert.Equal(t, map[string]any{
					"mcp_servers": map[string]any{
						"audiobookshelf": map[string]any{
							"command": "npx",
							"args":    []any{"-y", "@sierikov/audiobookshelf-mcp@0.3.0"},
							"enabled": true,
						},
					},
				}, parseYAML(t, got))
			})
		}
	})

	t.Run("quotes strings that look like other types", func(t *testing.T) {
		env := fakeEnv(t, "darwin", nil, "hermes")

		_, err := agentByID(t, env, "hermes").Add(t.Context(), Entry{Command: "true", Args: []string{"1", "null"}})

		require.NoError(t, err)
		doc := parseYAML(t, readFile(t, filepath.Join(env.Home, ".hermes", "config.yaml")))
		assert.Equal(t, map[string]any{
			"audiobookshelf": map[string]any{"command": "true", "args": []any{"1", "null"}, "enabled": true},
		}, doc["mcp_servers"])
	})
}

func TestHermes_UsesHermesHome(t *testing.T) {
	t.Run("HERMES_HOME", func(t *testing.T) {
		hermesHome := filepath.Join(t.TempDir(), "hermes-home")
		env := fakeEnv(t, "darwin", map[string]string{"HERMES_HOME": hermesHome}, "hermes")

		_, err := agentByID(t, env, "hermes").Add(t.Context(), npxEntry())

		require.NoError(t, err)
		assert.FileExists(t, filepath.Join(hermesHome, "config.yaml"))
		assert.NoDirExists(t, filepath.Join(env.Home, ".hermes"))
	})

	t.Run("windows LOCALAPPDATA", func(t *testing.T) {
		local := filepath.Join(t.TempDir(), "Local")
		env := fakeEnv(t, "windows", map[string]string{"LOCALAPPDATA": local}, "hermes")

		_, err := agentByID(t, env, "hermes").Add(t.Context(), npxEntry())

		require.NoError(t, err)
		assert.FileExists(t, filepath.Join(local, "hermes", "config.yaml"))
	})
}

func TestHermes_InvalidFileUntouched(t *testing.T) {
	cases := map[string]string{
		"not YAML":                     "model: [x\n",
		"not a mapping":                "- a\n- b\n",
		"mcp_servers is not a mapping": "mcp_servers: [a]\n",
		"more than one document":       "model: x\n---\nmodel: y\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			env := fakeEnv(t, "darwin", nil, "hermes")
			path := filepath.Join(env.Home, ".hermes", "config.yaml")
			writeFile(t, path, content)

			_, err := agentByID(t, env, "hermes").Add(t.Context(), npxEntry())

			require.Error(t, err)
			assert.Contains(t, err.Error(), path)
			assert.Equal(t, content, readFile(t, path))
			assert.Empty(t, backups(t, path))
		})
	}
}
