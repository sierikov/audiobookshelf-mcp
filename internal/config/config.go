// Package config reads and writes the settings file that holds the
// Audiobookshelf connection details, and layers environment variables on top.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/sierikov/audiobookshelf-mcp/internal/atomicfile"
)

const (
	appDir   = "audiobookshelf-mcp"
	fileName = "config.json"

	dirMode  = 0o700
	fileMode = 0o600

	envURL      = "AUDIOBOOKSHELF_URL"
	envToken    = "AUDIOBOOKSHELF_TOKEN"
	envToolsets = "AUDIOBOOKSHELF_TOOLSETS"
)

// Config is the content of the settings file.
type Config struct {
	// URL is the base URL of the Audiobookshelf server.
	URL string `json:"url"`
	// Token is the Audiobookshelf API token.
	Token string `json:"token"`
	// Toolsets names the toolsets to enable. Empty means the defaults.
	Toolsets []string `json:"toolsets,omitempty"`
}

// Path returns the location of the settings file. On Windows it lives under
// the user config directory (%AppData%); elsewhere under $XDG_CONFIG_HOME,
// falling back to $HOME/.config.
func Path() (string, error) {
	base, err := configDir()
	if err != nil {
		return "", err
	}
	return PathIn(base), nil
}

// PathIn returns where the settings file lives under the config directory
// dir, such as $HOME/.config.
func PathIn(dir string) string {
	return filepath.Join(dir, appDir, fileName)
}

func configDir() (string, error) {
	if runtime.GOOS == "windows" {
		return os.UserConfigDir()
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return xdg, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config"), nil
}

// Load reads the settings file at path. A missing file is not an error: it
// gives the zero Config, as there is nothing saved yet.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("reading %s: %w", path, err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	return c, nil
}

// Save writes c to path, creating the directory when needed. The file is
// private to the user (0600) and replaced atomically, so a reader never sees
// a half-written file. A symlink at path is kept: the file it points to is
// written.
func Save(path string, c Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding settings: %w", err)
	}
	data = append(data, '\n')
	if err := atomicfile.Write(path, data, fileMode); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// WithEnv returns c with the AUDIOBOOKSHELF_URL, AUDIOBOOKSHELF_TOKEN and
// AUDIOBOOKSHELF_TOOLSETS environment variables applied on top. A non-empty
// variable replaces the settings-file value; AUDIOBOOKSHELF_TOOLSETS is a
// comma-separated list whose blank entries are dropped.
func (c Config) WithEnv(getenv func(string) string) Config {
	if v := getenv(envURL); v != "" {
		c.URL = v
	}
	if v := getenv(envToken); v != "" {
		c.Token = v
	}
	if v := getenv(envToolsets); v != "" {
		c.Toolsets = SplitList(v)
	}
	return c
}

// SplitList splits a comma-separated list, such as the value of
// AUDIOBOOKSHELF_TOOLSETS or a command-line flag, into its trimmed, non-empty
// parts. It returns nil when there are none.
func SplitList(s string) []string {
	var out []string
	for item := range strings.SplitSeq(s, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
