package atomicfile

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func skipPermsOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits do not apply on Windows")
	}
}

func skipSymlinksOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks needs extra privileges on Windows")
	}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create symlinks here: %v", err)
	}
}

// assertSymlink checks that the last element of path is a symlink.
func assertSymlink(t *testing.T, path string) {
	t.Helper()
	fi, err := os.Lstat(path)
	require.NoError(t, err)
	assert.NotZero(t, fi.Mode()&fs.ModeSymlink, "%s is still a link", path)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(path))
	require.NoError(t, err)
	return string(data)
}

func mode(t *testing.T, path string) fs.FileMode {
	t.Helper()
	fi, err := os.Stat(path)
	require.NoError(t, err)
	return fi.Mode().Perm()
}

// names lists the entries of dir.
func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name()
	}
	return out
}

func TestWrite_NewFileGetsPerm(t *testing.T) {
	skipPermsOnWindows(t)
	path := filepath.Join(t.TempDir(), "settings.json")

	require.NoError(t, Write(path, []byte("new"), 0o640))

	assert.Equal(t, "new", readFile(t, path))
	assert.Equal(t, fs.FileMode(0o640), mode(t, path))
}

func TestWrite_ReplacesTheModeOfAnExistingFile(t *testing.T) {
	skipPermsOnWindows(t)
	path := filepath.Join(t.TempDir(), "settings.json")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o600))
	require.NoError(t, os.Chmod(path, 0o644)) //nolint:gosec // G302: a loose mode, to prove Write replaces it

	require.NoError(t, Write(path, []byte("new"), 0o600))

	assert.Equal(t, "new", readFile(t, path))
	assert.Equal(t, fs.FileMode(0o600), mode(t, path))
}

func TestWrite_WritesThroughASymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o700))
	require.NoError(t, os.WriteFile(target, []byte("old"), 0o600))
	link := filepath.Join(dir, "settings.json")
	symlink(t, target, link)

	require.NoError(t, Write(link, []byte("new"), 0o600))

	assertSymlink(t, link)
	assert.Equal(t, "new", readFile(t, target))
	assert.Equal(t, []string{"settings.json"}, names(t, filepath.Dir(target)))
}

func TestWrite_WritesThroughADanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "real"), 0o700))
	link := filepath.Join(dir, "settings.json")
	// A relative target is relative to the link's directory.
	symlink(t, filepath.Join("real", "settings.json"), link)

	require.NoError(t, Write(link, []byte("new"), 0o600))

	assertSymlink(t, link)
	assert.Equal(t, "new", readFile(t, filepath.Join(dir, "real", "settings.json")))
	assert.Equal(t, "new", readFile(t, link))
}

func TestWrite_FollowsAChainOfSymlinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.json")
	require.NoError(t, os.WriteFile(target, []byte("old"), 0o600))
	middle := filepath.Join(dir, "middle.json")
	symlink(t, "target.json", middle)
	link := filepath.Join(dir, "link.json")
	symlink(t, "middle.json", link)

	require.NoError(t, Write(link, []byte("new"), 0o600))

	assert.Equal(t, "new", readFile(t, target))
	assertSymlink(t, middle)
	assertSymlink(t, link)
}

func TestWrite_SymlinkLoopIsAnError(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	symlink(t, "b", a)
	symlink(t, "a", b)

	err := Write(a, []byte("new"), 0o600)

	require.Error(t, err)
	assert.ElementsMatch(t, []string{"a", "b"}, names(t, dir))
}

func TestWrite_LeavesNoTempFilesBehind(t *testing.T) {
	t.Run("after success", func(t *testing.T) {
		dir := t.TempDir()

		require.NoError(t, Write(filepath.Join(dir, "settings.json"), []byte("new"), 0o600))

		assert.Equal(t, []string{"settings.json"}, names(t, dir))
	})
	t.Run("after a failed rename", func(t *testing.T) {
		dir := t.TempDir()
		// A directory cannot be replaced by a file, so the rename fails.
		target := filepath.Join(dir, "settings.json")
		require.NoError(t, os.Mkdir(target, 0o700))

		err := Write(target, []byte("new"), 0o600)

		require.Error(t, err)
		assert.Equal(t, []string{"settings.json"}, names(t, dir))
		fi, statErr := os.Stat(target)
		require.NoError(t, statErr)
		assert.True(t, fi.IsDir(), "the directory is untouched")
	})
}

func TestWrite_MissingDirectoryIsAnError(t *testing.T) {
	err := Write(filepath.Join(t.TempDir(), "missing", "settings.json"), []byte("new"), 0o600)

	require.Error(t, err)
}

func TestWrite_RelativeTargetUnderSymlinkedDirectory(t *testing.T) {
	skipSymlinksOnWindows(t)
	base := t.TempDir()
	realSub := filepath.Join(base, "real", "sub")
	realOther := filepath.Join(base, "real", "other")
	require.NoError(t, os.MkdirAll(realSub, 0o700))
	require.NoError(t, os.MkdirAll(realOther, 0o700))
	target := filepath.Join(realOther, "target.json")
	require.NoError(t, os.WriteFile(target, []byte("old"), 0o600))
	// linkdir is a symlinked directory, like ~/.config pointing into a dotfiles repo.
	symlink(t, filepath.Join("real", "sub"), filepath.Join(base, "linkdir"))
	// The ".." climbs out of real/sub, where the link lives, not out of linkdir's parent.
	symlink(t, filepath.Join("..", "other", "target.json"), filepath.Join(realSub, "config.json"))

	require.NoError(t, Write(filepath.Join(base, "linkdir", "config.json"), []byte("new"), 0o600))

	assert.Equal(t, "new", readFile(t, target))
	assertSymlink(t, filepath.Join(base, "linkdir", "config.json"))
	assertSymlink(t, filepath.Join(realSub, "config.json"))
	assert.ElementsMatch(t, []string{"linkdir", "real"}, names(t, base), "nothing is created next to linkdir")
	assert.Equal(t, []string{"config.json"}, names(t, realSub), "no temp file in the link's directory")
	assert.Equal(t, []string{"target.json"}, names(t, realOther), "no temp file beside the target")
}

func TestWrite_RelativeChainUnderSymlinkedDirectory(t *testing.T) {
	skipSymlinksOnWindows(t)
	base := t.TempDir()
	realSub := filepath.Join(base, "real", "sub")
	realOther := filepath.Join(base, "real", "other")
	decoy := filepath.Join(base, "other")
	for _, dir := range []string{realSub, realOther, decoy} {
		require.NoError(t, os.MkdirAll(dir, 0o700))
	}
	final := filepath.Join(realOther, "final.json")
	require.NoError(t, os.WriteFile(final, []byte("old"), 0o600))
	symlink(t, filepath.Join("real", "sub"), filepath.Join(base, "linkdir"))
	// The first hop leaves real/sub through "..". The second hop is a bare name,
	// so it is relative to real/other, where the first hop landed. A text join
	// of "linkdir/../other" would have taken it to the decoy instead.
	symlink(t, filepath.Join("..", "other", "mid.json"), filepath.Join(realSub, "config.json"))
	symlink(t, "final.json", filepath.Join(realOther, "mid.json"))

	require.NoError(t, Write(filepath.Join(base, "linkdir", "config.json"), []byte("new"), 0o600))

	assert.Equal(t, "new", readFile(t, final))
	assert.Empty(t, names(t, decoy), "the decoy directory stays empty")
	assertSymlink(t, filepath.Join(base, "linkdir"))
	assertSymlink(t, filepath.Join(realSub, "config.json"))
	assertSymlink(t, filepath.Join(realOther, "mid.json"))
	assert.ElementsMatch(t, []string{"final.json", "mid.json"}, names(t, realOther), "no temp file beside the target")
}
