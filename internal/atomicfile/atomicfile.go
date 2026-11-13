// Package atomicfile replaces files atomically, so a reader sees either the
// old content or the new one, never half of it.
package atomicfile

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// maxLinks is how many symlinks Write follows before it gives up on a loop.
const maxLinks = 40

// Write replaces the file at path with data and sets its mode to perm. It
// writes a temp file in the same directory, syncs it, and renames it over the
// file. A symlink at path is followed, even one whose target does not exist
// yet: the file it points to is written and the link stays a link, and the temp
// file goes in the directory that file really lives in. The temp file is
// removed when any step fails. The directory of the file written must already
// exist.
func Write(path string, data []byte, perm fs.FileMode) error {
	target, err := resolve(path)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Chmod(tmp, perm)
	}
	if err == nil {
		err = os.Rename(tmp, target)
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// resolve follows the symlinks at path and returns the file they lead to,
// which need not exist. The returned path has no symlinks in its directory, so
// the temp file Write creates beside it, and the rename over it, stay in the
// directory the target really lives in.
//
// A relative link target is relative to the directory the link lives in, and
// ".." climbs out of that directory as the file system sees it. If the link's
// directory path goes through a symlink, that is not the directory the path
// names as text, so each hop resolves its own directory before the target is
// joined onto it.
func resolve(path string) (string, error) {
	for range maxLinks {
		path = withRealDir(path)
		fi, err := os.Lstat(path)
		if errors.Is(err, fs.ErrNotExist) {
			return path, nil
		}
		if err != nil {
			return "", err
		}
		if fi.Mode()&fs.ModeSymlink == 0 {
			return path, nil
		}
		target, err := os.Readlink(path)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(target) {
			// The target is joined without cleaning: it may hold a ".." after a
			// linked directory, which withRealDir resolves on the next hop.
			target = filepath.Dir(path) + string(filepath.Separator) + target
		}
		path = target
	}
	return "", &fs.PathError{Op: "resolve", Path: path, Err: errors.New("too many levels of symbolic links")}
}

// withRealDir returns path with the directory part resolved through symlinks
// and "..", and the last element left alone, since it may itself be a link. A
// directory that cannot be resolved, because it does not exist, is left as it
// is, so the caller fails on it with the error that names it.
func withRealDir(path string) string {
	dir, name := filepath.Split(path)
	if dir == "" {
		dir = "."
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return path
	}
	return filepath.Join(realDir, name)
}
