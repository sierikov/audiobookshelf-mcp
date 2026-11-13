package agents

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testNow is the fixed clock every test uses; testStamp is how it appears in
// backup names.
var testNow = time.Date(2026, 11, 1, 9, 30, 15, 0, time.UTC)

const testStamp = "20261101093015"

// npxEntry is the entry an npm install writes.
func npxEntry() Entry {
	return Entry{Command: "npx", Args: []string{"-y", "@sierikov/audiobookshelf-mcp@0.3.0"}}
}

// fakeEnv returns an Env whose home is a fresh temp directory. LookPath finds
// only bins, Getenv reads vars, and Run fails the test: tests that expect
// commands replace it.
func fakeEnv(t *testing.T, goos string, vars map[string]string, bins ...string) Env {
	t.Helper()
	return Env{
		GOOS:   goos,
		Home:   t.TempDir(),
		Getenv: func(key string) string { return vars[key] },
		LookPath: func(name string) (string, error) {
			if slices.Contains(bins, name) {
				return "/fake/bin/" + name, nil
			}
			return "", exec.ErrNotFound
		},
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			t.Errorf("unexpected command: %s %q", name, args)
			return nil, errors.New("unexpected command")
		},
		Now: func() time.Time { return testNow },
	}
}

func agentByID(t *testing.T, env Env, id string) Agent {
	t.Helper()
	for _, a := range All(env) {
		if a.ID() == id {
			return a
		}
	}
	t.Fatalf("no agent with ID %q", id)
	return nil
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o700))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	mkdir(t, filepath.Dir(path))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(path))
	require.NoError(t, err)
	return string(data)
}

// backups lists the backup files made next to path.
func backups(t *testing.T, path string) []string {
	t.Helper()
	matches, err := filepath.Glob(path + ".bak-*")
	require.NoError(t, err)
	return matches
}

func TestAll_IDsAndOrder(t *testing.T) {
	all := All(fakeEnv(t, "darwin", nil))

	ids := make([]string, len(all))
	for i, a := range all {
		ids[i] = a.ID()
	}

	assert.Equal(t, []string{
		"claude-desktop", "claude-code", "codex", "opencode",
		"hermes", "antigravity", "gemini", "cursor",
	}, ids)
}

func TestAll_Names(t *testing.T) {
	all := All(fakeEnv(t, "darwin", nil))

	names := make([]string, len(all))
	for i, a := range all {
		names[i] = a.Name()
	}

	assert.Equal(t, []string{
		"Claude Desktop", "Claude Code", "Codex", "OpenCode",
		"Hermes Agent", "Antigravity CLI", "Gemini CLI", "Cursor",
	}, names)
}

func TestDetect(t *testing.T) {
	t.Run("nothing installed", func(t *testing.T) {
		for _, a := range All(fakeEnv(t, "darwin", nil)) {
			assert.False(t, a.Detect(), a.ID())
		}
	})

	cases := []struct {
		name string
		id   string
		bin  string // put on PATH
		dir  string // created under the home directory
	}{
		{name: "claude-desktop by directory", id: "claude-desktop", dir: "Library/Application Support/Claude"},
		{name: "claude-code by binary", id: "claude-code", bin: "claude"},
		{name: "codex by binary", id: "codex", bin: "codex"},
		{name: "codex by directory", id: "codex", dir: ".codex"},
		{name: "opencode by binary", id: "opencode", bin: "opencode"},
		{name: "hermes by binary", id: "hermes", bin: "hermes"},
		{name: "hermes by directory", id: "hermes", dir: ".hermes"},
		{name: "antigravity by directory", id: "antigravity", dir: ".gemini/config"},
		{name: "gemini by binary", id: "gemini", bin: "gemini"},
		{name: "cursor by directory", id: "cursor", dir: ".cursor"},
		{name: "agy on PATH is not Antigravity CLI", bin: "agy"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var bins []string
			if tc.bin != "" {
				bins = append(bins, tc.bin)
			}
			env := fakeEnv(t, "darwin", nil, bins...)
			if tc.dir != "" {
				mkdir(t, filepath.Join(env.Home, filepath.FromSlash(tc.dir)))
			}

			for _, a := range All(env) {
				assert.Equal(t, a.ID() == tc.id, a.Detect(), a.ID())
			}
		})
	}
}

func TestFileAgent_ModeOfWrittenFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits do not apply on Windows")
	}
	env := fakeEnv(t, "darwin", nil)
	path := filepath.Join(env.Home, ".cursor", "mcp.json")
	mkdir(t, filepath.Dir(path))
	a := agentByID(t, env, "cursor")

	_, err := a.Add(t.Context(), npxEntry())
	require.NoError(t, err)
	fi, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o600), fi.Mode().Perm(), "a new file is private")

	mode := fs.FileMode(0o640)
	require.NoError(t, os.Chmod(path, mode))
	// A different entry, so the file changes and is backed up.
	res, err := a.Add(t.Context(), Entry{Command: "/usr/local/bin/audiobookshelf-mcp"})
	require.NoError(t, err)
	fi, err = os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, mode, fi.Mode().Perm(), "an existing file keeps its mode")
	fi, err = os.Stat(res.Backup)
	require.NoError(t, err)
	assert.Equal(t, mode, fi.Mode().Perm(), "the backup has the original's mode")
}

func TestFileAgent_WritesThroughSymlink(t *testing.T) {
	env := fakeEnv(t, "darwin", nil, "gemini")
	target := filepath.Join(env.Home, "dotfiles", "gemini-settings.json")
	writeFile(t, target, `{"theme": "dark"}`)
	link := filepath.Join(env.Home, ".gemini", "settings.json")
	mkdir(t, filepath.Dir(link))
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create symlinks here: %v", err)
	}

	res, err := agentByID(t, env, "gemini").Add(t.Context(), npxEntry())

	require.NoError(t, err)
	fi, err := os.Lstat(link)
	require.NoError(t, err)
	assert.NotZero(t, fi.Mode()&fs.ModeSymlink, "the link is still a link")
	assert.JSONEq(t, `{"theme": "dark", "mcpServers": {"audiobookshelf": {
		"command": "npx", "args": ["-y", "@sierikov/audiobookshelf-mcp@0.3.0"]}}}`, readFile(t, target))
	assert.Equal(t, link+".bak-"+testStamp, res.Backup)
	assert.JSONEq(t, `{"theme": "dark"}`, readFile(t, res.Backup))
}

func TestFileAgent_NeverOverwritesABackup(t *testing.T) {
	env := fakeEnv(t, "darwin", nil)
	path := filepath.Join(env.Home, ".cursor", "mcp.json")
	writeFile(t, path, `{"a": 1}`)
	older := path + ".bak-" + testStamp
	writeFile(t, older, "older backup")

	_, err := agentByID(t, env, "cursor").Add(t.Context(), npxEntry())

	require.Error(t, err)
	assert.Contains(t, err.Error(), path)
	assert.Equal(t, "older backup", readFile(t, older))
	assert.JSONEq(t, `{"a": 1}`, readFile(t, path))
}

func TestFileAgent_UnchangedEditMakesNoBackupAndNoWrite(t *testing.T) {
	env := fakeEnv(t, "darwin", nil)
	path := filepath.Join(env.Home, ".cursor", "mcp.json")
	writeFile(t, path, `{"theme": "dark"}`)
	a := agentByID(t, env, "cursor")

	first, err := a.Add(t.Context(), npxEntry())
	require.NoError(t, err)
	require.Equal(t, path+".bak-"+testStamp, first.Backup)
	after := readFile(t, path)
	fi, err := os.Stat(path)
	require.NoError(t, err)

	// The same entry again, in the same second as the first backup.
	second, err := a.Add(t.Context(), npxEntry())

	require.NoError(t, err)
	assert.Empty(t, second.Backup)
	assert.Equal(t, "Restart Cursor.", second.Restart)
	assert.Equal(t, []string{path + ".bak-" + testStamp}, backups(t, path))
	assert.Equal(t, after, readFile(t, path))
	fi2, err := os.Stat(path)
	require.NoError(t, err)
	assert.True(t, os.SameFile(fi, fi2), "the file was not replaced")
}

func TestFileAgent_LeavesNoTempFilesBehind(t *testing.T) {
	env := fakeEnv(t, "darwin", nil)
	dir := filepath.Join(env.Home, ".cursor")
	mkdir(t, dir)

	_, err := agentByID(t, env, "cursor").Add(t.Context(), npxEntry())
	require.NoError(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "mcp.json", entries[0].Name())
}

func TestDefaultEnv(t *testing.T) {
	env, err := DefaultEnv()

	require.NoError(t, err)
	assert.Equal(t, runtime.GOOS, env.GOOS)
	assert.NotEmpty(t, env.Home)
}
