// Package agents adds this server to the MCP configuration of the AI agents
// installed on the machine. Each agent has an adapter that knows where its
// configuration lives and how to write to it: by editing its file, or through
// its own CLI.
package agents

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/sierikov/audiobookshelf-mcp/internal/atomicfile"
)

// ServerName is the name this server is registered under in every agent.
const ServerName = "audiobookshelf"

const (
	// newFileMode is the mode of a config file the adapters create. Files that
	// already exist keep their own mode.
	newFileMode fs.FileMode = 0o600
	// newDirMode is the mode of a config directory the adapters create.
	newDirMode fs.FileMode = 0o700
	// backupStamp is the time format in backup names: <file>.bak-<stamp>.
	backupStamp = "20060102150405"
)

// Entry is the command an agent runs to start the server. It never holds the token.
type Entry struct {
	// Command is the executable, such as "npx" or an absolute path.
	Command string
	// Args are the arguments to Command. Empty is fine.
	Args []string
}

// Result reports what Add changed.
type Result struct {
	// Backup is the backup file path; "" when the agent's own CLI made the
	// change, the file was new, or nothing changed.
	Backup string
	// Restart tells the user how to make the agent pick up the change, in one
	// sentence, e.g. "Quit and reopen Claude Desktop.".
	Restart string
}

// Agent is an AI agent whose MCP configuration this package can write to.
type Agent interface {
	// ID is the short name used on the command line, such as "claude-code".
	ID() string
	// Name is the display name, such as "Claude Code".
	Name() string
	// Detect reports whether the agent seems to be installed.
	Detect() bool
	// Add adds this server's entry to the agent's configuration, replacing an
	// entry of the same name. Every other value in the file is kept; formatting
	// and key order may change.
	Add(ctx context.Context, e Entry) (Result, error)
}

// Env is everything adapters read from the machine, so tests can fake it.
type Env struct {
	// GOOS is the operating system, as in runtime.GOOS.
	GOOS string
	// Home is the user's home directory.
	Home string
	// Getenv reads an environment variable.
	Getenv func(string) string
	// LookPath finds an executable on PATH.
	LookPath func(string) (string, error)
	// Run runs a command and returns its combined output.
	Run func(ctx context.Context, name string, args ...string) ([]byte, error)
	// Now returns the current time, used in backup names.
	Now func() time.Time
}

// DefaultEnv returns the Env of the running process.
func DefaultEnv() (Env, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Env{}, fmt.Errorf("finding the home directory: %w", err)
	}
	return Env{
		GOOS:     runtime.GOOS,
		Home:     home,
		Getenv:   os.Getenv,
		LookPath: exec.LookPath,
		Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204: fixed agent binary names, arguments built from our own entry
			// Killing a Windows .cmd shim can leave node holding the output
			// pipe, so stop waiting for it a little after the kill.
			cmd.WaitDelay = 5 * time.Second
			return cmd.CombinedOutput()
		},
		Now: time.Now,
	}, nil
}

// All returns an adapter for every supported agent, in a fixed order.
func All(env Env) []Agent {
	return []Agent{
		claudeDesktop(env),
		claudeCode(env),
		codex(env),
		opencode(env),
		hermes(env),
		antigravity(env),
		gemini(env),
		cursor(env),
	}
}

// about is what every adapter knows about its agent: the ID, the name, the
// restart hint, and how to tell the agent is installed.
type about struct {
	id, name, restart string
	// bin detects the agent when this executable is on PATH; "" skips the check.
	bin string
	// dir detects the agent when this directory exists; "" skips the check.
	dir      string
	lookPath func(string) (string, error)
}

func (a about) ID() string   { return a.id }
func (a about) Name() string { return a.name }

// Detect reports whether bin is on PATH or dir exists.
func (a about) Detect() bool {
	if a.bin != "" {
		if _, err := a.lookPath(a.bin); err == nil {
			return true
		}
	}
	return a.dir != "" && dirExists(a.dir)
}

// fileAgent is an agent configured by editing one file.
type fileAgent struct {
	about
	path string
	// edit returns the new content of the file given the current one, which
	// is nil when the file does not exist. It fails when the current content
	// does not parse.
	edit func(old []byte, e Entry) ([]byte, error)
	now  func() time.Time
}

// Add edits the file, after backing it up when it exists.
func (a fileAgent) Add(_ context.Context, e Entry) (Result, error) {
	bak, err := editFile(a.path, a.now(), func(old []byte) ([]byte, error) { return a.edit(old, e) })
	if err != nil {
		return Result{}, err
	}
	return Result{Backup: bak, Restart: a.restart}, nil
}

// editFile replaces the content of the file at path with edit's result and
// returns the backup's path. A missing file is created, with no backup. When
// edit fails, nothing is written and no backup is made; when edit changes
// nothing, the file is neither written nor backed up, and the backup's path is
// "". A symlink is kept: the file it points to is written, and the backup lands
// next to the link.
func editFile(path string, now time.Time, edit func(old []byte) ([]byte, error)) (string, error) {
	old, err := os.ReadFile(filepath.Clean(path))
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	data, err := edit(old)
	if err != nil {
		return "", fmt.Errorf("editing %s: %w", path, err)
	}
	if exists && bytes.Equal(data, old) {
		return "", nil
	}
	mode := newFileMode
	var bak string
	if exists {
		fi, err := os.Stat(path)
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", path, err)
		}
		mode = fi.Mode().Perm()
		if bak, err = backup(path, old, mode, now); err != nil {
			return "", err
		}
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, newDirMode); err != nil {
		return "", fmt.Errorf("creating %s: %w", dir, err)
	}
	if err := atomicfile.Write(path, data, mode); err != nil {
		return "", fmt.Errorf("writing %s: %w", path, err)
	}
	return bak, nil
}

// backup writes data, the content of the file at path as it was read, to
// <path>.bak-<YYYYMMDDHHMMSS> with the given mode, and returns the copy's
// path. It never overwrites a file.
func backup(path string, data []byte, mode fs.FileMode, now time.Time) (string, error) {
	name := path + ".bak-" + now.Format(backupStamp)
	f, err := os.OpenFile(filepath.Clean(name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return "", fmt.Errorf("backing up %s: %w", path, err)
	}
	_, err = f.Write(data)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(name)
		return "", fmt.Errorf("backing up %s: %w", path, err)
	}
	return name, nil
}

func dirExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// argsOrEmpty returns args, or an empty slice instead of nil, so that encoders
// write an empty list rather than null.
func argsOrEmpty(args []string) []string {
	if args == nil {
		return []string{}
	}
	return args
}
