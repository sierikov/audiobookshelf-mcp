package config

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX paths and permission bits do not apply on Windows")
	}
}

func TestPath_UsesXDGConfigHome(t *testing.T) {
	skipOnWindows(t)
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	p, err := Path()

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(xdg, "audiobookshelf-mcp", "config.json"), p)
}

func TestPath_DefaultsToHomeDotConfig(t *testing.T) {
	skipOnWindows(t)
	t.Setenv("XDG_CONFIG_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)

	p, err := Path()

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".config", "audiobookshelf-mcp", "config.json"), p)
}

func TestSaveLoad_RoundTripWithPrivatePermissions(t *testing.T) {
	skipOnWindows(t)
	dir := filepath.Join(t.TempDir(), "sub")
	path := filepath.Join(dir, "config.json")
	in := Config{URL: "https://abs.example", Token: "tok", Toolsets: []string{"all"}}

	require.NoError(t, Save(path, in))

	got, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, in, got)

	fi, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o600), fi.Mode().Perm())
	di, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o700), di.Mode().Perm())
}

func TestSave_TightensExistingFilePermissions(t *testing.T) {
	skipOnWindows(t)
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte("{}"), 0o600))
	require.NoError(t, os.Chmod(path, 0o644)) //nolint:gosec // G302: deliberately loose mode, to prove Save tightens it

	require.NoError(t, Save(path, Config{URL: "u", Token: "t"}))

	fi, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o600), fi.Mode().Perm())
}

func TestSave_LeavesNoTempFilesBehind(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, Save(filepath.Join(dir, "config.json"), Config{URL: "u", Token: "t"}))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "config.json", entries[0].Name())
}

func TestSave_WritesIndentedJSONWithTrailingNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	require.NoError(t, Save(path, Config{URL: "u", Token: "t"}))

	raw, err := os.ReadFile(path) //nolint:gosec // G304: path is under t.TempDir()
	require.NoError(t, err)
	want := `{
  "url": "u",
  "token": "t"
}
`
	assert.Equal(t, want, string(raw))
}

func TestLoad_MissingFile(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "nope.json"))

	require.NoError(t, err)
	assert.Equal(t, Config{}, got)
}

func TestLoad_InvalidJSONNamesThePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte("{"), 0o600))

	_, err := Load(path)

	require.Error(t, err)
	assert.Contains(t, err.Error(), path)
}

func TestWithEnv(t *testing.T) {
	file := Config{URL: "https://file", Token: "file-tok", Toolsets: []string{"all"}}
	env := map[string]string{"AUDIOBOOKSHELF_TOKEN": "env-tok", "AUDIOBOOKSHELF_TOOLSETS": " chapters, metadata ,"}

	got := file.WithEnv(func(k string) string { return env[k] })

	assert.Equal(t, Config{URL: "https://file", Token: "env-tok", Toolsets: []string{"chapters", "metadata"}}, got)
}

func TestWithEnv_URLOverridesFile(t *testing.T) {
	file := Config{URL: "https://file", Token: "file-tok"}
	env := map[string]string{"AUDIOBOOKSHELF_URL": "https://env"}

	got := file.WithEnv(func(k string) string { return env[k] })

	assert.Equal(t, Config{URL: "https://env", Token: "file-tok"}, got)
}

func TestWithEnv_EmptyEnvKeepsFileValues(t *testing.T) {
	file := Config{URL: "https://file", Token: "file-tok", Toolsets: []string{"all"}}

	got := file.WithEnv(func(string) string { return "" })

	assert.Equal(t, file, got)
}

func TestWithEnv_OnlyEmptyToolsetEntriesMeansDefaults(t *testing.T) {
	file := Config{Toolsets: []string{"all"}}
	env := map[string]string{"AUDIOBOOKSHELF_TOOLSETS": " , ,"}

	got := file.WithEnv(func(k string) string { return env[k] })

	assert.Empty(t, got.Toolsets)
}

func TestSplitList(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{" , ,", nil},
		{"all", []string{"all"}},
		{" chapters , ,metadata,", []string{"chapters", "metadata"}},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, SplitList(tt.in))
		})
	}
}
