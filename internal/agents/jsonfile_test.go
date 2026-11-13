package agents

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONAgent_CreatesFileWhenMissing(t *testing.T) {
	env := fakeEnv(t, "darwin", nil)
	dir := filepath.Join(env.Home, ".cursor")
	mkdir(t, dir)

	res, err := agentByID(t, env, "cursor").Add(t.Context(), Entry{Command: "/opt/homebrew/bin/audiobookshelf-mcp"})

	require.NoError(t, err)
	assert.JSONEq(t, `{"mcpServers": {"audiobookshelf": {
		"type": "stdio", "command": "/opt/homebrew/bin/audiobookshelf-mcp", "args": []}}}`,
		readFile(t, filepath.Join(dir, "mcp.json")))
	assert.Empty(t, res.Backup)
	assert.Equal(t, "Restart Cursor.", res.Restart)
}

func TestJSONAgent_CreatesMissingDirectory(t *testing.T) {
	env := fakeEnv(t, "darwin", nil, "gemini")

	res, err := agentByID(t, env, "gemini").Add(t.Context(), npxEntry())

	require.NoError(t, err)
	assert.JSONEq(t, `{"mcpServers": {"audiobookshelf": {
		"command": "npx", "args": ["-y", "@sierikov/audiobookshelf-mcp@0.3.0"]}}}`,
		readFile(t, filepath.Join(env.Home, ".gemini", "settings.json")))
	assert.Equal(t, "Restart Gemini CLI.", res.Restart)
}

func TestJSONAgent_KeepsOtherKeysAndReplacesOurEntry(t *testing.T) {
	env := fakeEnv(t, "darwin", nil)
	path := filepath.Join(env.Home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	original := `{"preferences":{"x":1},"mcpServers":{"other":{"command":"o"},` +
		`"audiobookshelf":{"command":"/old","env":{"AUDIOBOOKSHELF_TOKEN":"t"}}}}`
	writeFile(t, path, original)

	res, err := agentByID(t, env, "claude-desktop").Add(t.Context(), npxEntry())

	require.NoError(t, err)
	assert.JSONEq(t, `{
		"preferences": {"x": 1},
		"mcpServers": {
			"other": {"command": "o"},
			"audiobookshelf": {"command": "npx", "args": ["-y", "@sierikov/audiobookshelf-mcp@0.3.0"]}
		}
	}`, readFile(t, path))
	assert.Equal(t, path+".bak-"+testStamp, res.Backup)
	assert.Equal(t, original, readFile(t, res.Backup))
	assert.Equal(t, "Quit and reopen Claude Desktop.", res.Restart)
}

func TestJSONAgent_EncodingKeepsNumbersAndText(t *testing.T) {
	env := fakeEnv(t, "darwin", nil)
	path := filepath.Join(env.Home, ".gemini", "config", "mcp_config.json")
	writeFile(t, path, `{"big": 12345678901234567890, "price": 1.50, "url": "https://x.example/?a=1&b=<2>"}`)

	res, err := agentByID(t, env, "antigravity").Add(t.Context(), npxEntry())

	require.NoError(t, err)
	got := readFile(t, path)
	assert.Contains(t, got, `"big": 12345678901234567890,`)
	assert.Contains(t, got, `"price": 1.50,`)
	assert.Contains(t, got, `"url": "https://x.example/?a=1&b=<2>"`)
	assert.Contains(t, got, "{\n  \"big\"", "2-space indent")
	assert.Contains(t, got, "\n    \"audiobookshelf\": {\n", "2-space indent")
	assert.Equal(t, '\n', rune(got[len(got)-1]), "trailing newline")
	assert.Equal(t, "Restart Antigravity CLI.", res.Restart)
}

func TestJSONAgent_EmptyFileIsTreatedAsEmptyObject(t *testing.T) {
	env := fakeEnv(t, "darwin", nil)
	path := filepath.Join(env.Home, ".cursor", "mcp.json")
	writeFile(t, path, "  \n")

	res, err := agentByID(t, env, "cursor").Add(t.Context(), npxEntry())

	require.NoError(t, err)
	assert.JSONEq(t, `{"mcpServers": {"audiobookshelf": {
		"type": "stdio", "command": "npx", "args": ["-y", "@sierikov/audiobookshelf-mcp@0.3.0"]}}}`, readFile(t, path))
	assert.Equal(t, path+".bak-"+testStamp, res.Backup)
}

func TestJSONAgent_ReadsAFileWithAByteOrderMark(t *testing.T) {
	env := fakeEnv(t, "darwin", nil)
	path := filepath.Join(env.Home, ".cursor", "mcp.json")
	original := "\xEF\xBB\xBF" + `{"theme": "dark", "mcpServers": {"other": {"command": "o"}}}`
	writeFile(t, path, original)

	res, err := agentByID(t, env, "cursor").Add(t.Context(), npxEntry())

	require.NoError(t, err)
	got := readFile(t, path)
	assert.False(t, strings.HasPrefix(got, "\xEF\xBB\xBF"), "written without a byte order mark")
	assert.JSONEq(t, `{"theme": "dark", "mcpServers": {
		"other": {"command": "o"},
		"audiobookshelf": {"type": "stdio", "command": "npx", "args": ["-y", "@sierikov/audiobookshelf-mcp@0.3.0"]}}}`, got)
	assert.Equal(t, original, readFile(t, res.Backup), "the backup keeps the original bytes")
}

func TestJSONAgent_InvalidFileUntouched(t *testing.T) {
	cases := map[string]string{
		"not JSON":                    "{ not json",
		"not an object":               `["a"]`,
		"data after the object":       `{"a": 1} {"b": 2}`,
		"mcpServers is not an object": `{"mcpServers": ["a"]}`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			env := fakeEnv(t, "darwin", nil, "gemini")
			path := filepath.Join(env.Home, ".gemini", "settings.json")
			writeFile(t, path, content)

			_, err := agentByID(t, env, "gemini").Add(t.Context(), npxEntry())

			require.Error(t, err)
			assert.Contains(t, err.Error(), path)
			assert.Equal(t, content, readFile(t, path))
			assert.Empty(t, backups(t, path))
		})
	}
}

func TestClaudeDesktop_WindowsWrapsNPX(t *testing.T) {
	base := t.TempDir()
	vars := map[string]string{
		"APPDATA":      filepath.Join(base, "Roaming"),
		"LOCALAPPDATA": filepath.Join(base, "Local"),
	}
	env := fakeEnv(t, "windows", vars)
	path := filepath.Join(vars["APPDATA"], "Claude", "claude_desktop_config.json")
	mkdir(t, filepath.Dir(path))
	a := agentByID(t, env, "claude-desktop")

	_, err := a.Add(t.Context(), npxEntry())
	require.NoError(t, err)
	assert.JSONEq(t, `{"mcpServers": {"audiobookshelf": {
		"command": "cmd", "args": ["/c", "npx", "-y", "@sierikov/audiobookshelf-mcp@0.3.0"]}}}`, readFile(t, path))

	exe := `C:\Users\me\AppData\Local\audiobookshelf-mcp\audiobookshelf-mcp.exe`
	_, err = a.Add(t.Context(), Entry{Command: exe})
	require.NoError(t, err)
	assert.JSONEq(t, `{"mcpServers": {"audiobookshelf": {
		"command": "C:\\Users\\me\\AppData\\Local\\audiobookshelf-mcp\\audiobookshelf-mcp.exe", "args": []}}}`,
		readFile(t, path))
}

func TestClaudeDesktop_DoesNotWrapNPXElsewhere(t *testing.T) {
	env := fakeEnv(t, "linux", nil)
	path := filepath.Join(env.Home, ".config", "Claude", "claude_desktop_config.json")
	mkdir(t, filepath.Dir(path))

	_, err := agentByID(t, env, "claude-desktop").Add(t.Context(), npxEntry())

	require.NoError(t, err)
	assert.JSONEq(t, `{"mcpServers": {"audiobookshelf": {
		"command": "npx", "args": ["-y", "@sierikov/audiobookshelf-mcp@0.3.0"]}}}`, readFile(t, path))
}

func TestClaudeDesktop_ConfigDirPerOS(t *testing.T) {
	t.Run("darwin", func(t *testing.T) {
		env := fakeEnv(t, "darwin", nil)
		assert.Equal(t, filepath.Join(env.Home, "Library", "Application Support", "Claude"), claudeDesktopDir(env))
	})

	t.Run("linux with XDG_CONFIG_HOME", func(t *testing.T) {
		xdg := filepath.Join(t.TempDir(), "xdg")
		env := fakeEnv(t, "linux", map[string]string{"XDG_CONFIG_HOME": xdg})
		assert.Equal(t, filepath.Join(xdg, "Claude"), claudeDesktopDir(env))
	})

	t.Run("linux without XDG_CONFIG_HOME", func(t *testing.T) {
		env := fakeEnv(t, "linux", nil)
		assert.Equal(t, filepath.Join(env.Home, ".config", "Claude"), claudeDesktopDir(env))
	})

	windowsVars := func(t *testing.T) map[string]string {
		t.Helper()
		base := t.TempDir()
		return map[string]string{
			"APPDATA":      filepath.Join(base, "Roaming"),
			"LOCALAPPDATA": filepath.Join(base, "Local"),
		}
	}

	t.Run("windows APPDATA", func(t *testing.T) {
		vars := windowsVars(t)
		env := fakeEnv(t, "windows", vars)
		assert.Equal(t, filepath.Join(vars["APPDATA"], "Claude"), claudeDesktopDir(env))
	})

	t.Run("windows MSIX install", func(t *testing.T) {
		vars := windowsVars(t)
		msix := filepath.Join(vars["LOCALAPPDATA"], "Packages", "Claude_pzs8sxrjxfjjc", "LocalCache", "Roaming", "Claude")
		mkdir(t, msix)
		env := fakeEnv(t, "windows", vars)
		assert.Equal(t, msix, claudeDesktopDir(env))
	})

	t.Run("windows without APPDATA", func(t *testing.T) {
		env := fakeEnv(t, "windows", nil)
		assert.Equal(t, filepath.Join(env.Home, "AppData", "Roaming", "Claude"), claudeDesktopDir(env))
	})
}
