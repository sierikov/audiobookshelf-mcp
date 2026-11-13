package setup

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sierikov/audiobookshelf-mcp/internal/agents"
	"github.com/sierikov/audiobookshelf-mcp/internal/update"
)

func TestEntryFor_NPMPinsVersion(t *testing.T) {
	exe := "/home/me/.npm/_npx/1/node_modules/@sierikov/audiobookshelf-mcp-linux-x64/bin/audiobookshelf-mcp"

	got := EntryFor(update.MethodNPM, exe, "0.3.0")

	assert.Equal(t, agents.Entry{
		Command: "npx",
		Args:    []string{"-y", "@sierikov/audiobookshelf-mcp@0.3.0"},
	}, got)
}

func TestEntryFor_HomebrewUsesStablePath(t *testing.T) {
	tests := []struct {
		name string
		exe  string
		want string
	}{
		{
			name: "cask on Apple silicon",
			exe:  "/opt/homebrew/Caskroom/audiobookshelf-mcp/0.3.0/audiobookshelf-mcp",
			want: "/opt/homebrew/bin/audiobookshelf-mcp",
		},
		{
			name: "formula on Linux",
			exe:  "/home/linuxbrew/.linuxbrew/Cellar/audiobookshelf-mcp/0.3.0/bin/audiobookshelf-mcp",
			want: "/home/linuxbrew/.linuxbrew/bin/audiobookshelf-mcp",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EntryFor(update.MethodHomebrew, tt.exe, "0.3.0")

			assert.Equal(t, tt.want, got.Command)
			assert.Empty(t, got.Args)
		})
	}
}

func TestEntryFor_HomebrewOutsideTheCellarUsesExe(t *testing.T) {
	exe := "/somewhere/else/audiobookshelf-mcp"

	got := EntryFor(update.MethodHomebrew, exe, "0.3.0")

	assert.Equal(t, exe, got.Command)
	assert.Empty(t, got.Args)
}

func TestEntryFor_GoAndUnknownUseExe(t *testing.T) {
	tests := []struct {
		name   string
		method update.Method
		exe    string
	}{
		{"go install", update.MethodGo, "/home/me/go/bin/audiobookshelf-mcp"},
		{"unknown", update.MethodUnknown, "/usr/local/bin/audiobookshelf-mcp"},
		{"windows download", update.MethodUnknown, `C:\Tools\audiobookshelf-mcp.exe`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EntryFor(tt.method, tt.exe, "0.3.0")

			assert.Equal(t, tt.exe, got.Command)
			assert.Empty(t, got.Args)
		})
	}
}
