package update

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectMethod(t *testing.T) {
	tests := []struct {
		name  string
		exe   string
		goBin string
		want  Method
	}{
		{"brew cask", "/opt/homebrew/Caskroom/audiobookshelf-mcp/0.3.0/audiobookshelf-mcp", "", MethodHomebrew},
		{"brew formula", "/home/linuxbrew/.linuxbrew/Cellar/audiobookshelf-mcp/0.3.0/bin/audiobookshelf-mcp", "", MethodHomebrew},
		{"npm platform package", "/Users/a/.npm/_npx/1a2b/node_modules/@sierikov/audiobookshelf-mcp-darwin-arm64/bin/audiobookshelf-mcp", "", MethodNPM},
		{"npm on windows", `C:\Users\a\AppData\Local\npm-cache\_npx\1a2b\node_modules\@sierikov\audiobookshelf-mcp-win32-x64\bin\audiobookshelf-mcp.exe`, "", MethodNPM},
		{"go install", "/Users/a/go/bin/audiobookshelf-mcp", "/Users/a/go/bin", MethodGo},
		{"go install with trailing slash in GOBIN", "/Users/a/go/bin/audiobookshelf-mcp", "/Users/a/go/bin/", MethodGo},
		{"go install on windows", `C:\Users\a\go\bin\audiobookshelf-mcp.exe`, `C:\Users\a\go\bin`, MethodGo},
		{"repo build", "/Users/a/work/audiobookshelf-mcp/audiobookshelf-mcp", "/Users/a/go/bin", MethodUnknown},
		{"another brew formula is not ours", "/opt/homebrew/Cellar/other-tool/1.0.0/bin/audiobookshelf-mcp", "", MethodUnknown},
		{"unknown GOBIN does not match a bare file name", "audiobookshelf-mcp", "", MethodUnknown},
		{"empty path", "", "", MethodUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, DetectMethod(tt.exe, tt.goBin))
		})
	}
}

func TestBrewPrefix(t *testing.T) {
	tests := []struct {
		name       string
		exe        string
		wantPrefix string
		wantOK     bool
	}{
		{"cask", "/opt/homebrew/Caskroom/audiobookshelf-mcp/0.3.0/audiobookshelf-mcp", "/opt/homebrew", true},
		{"formula", "/home/linuxbrew/.linuxbrew/Cellar/audiobookshelf-mcp/0.3.0/bin/audiobookshelf-mcp", "/home/linuxbrew/.linuxbrew", true},
		{"not brew", "/Users/a/go/bin/audiobookshelf-mcp", "", false},
		{"empty", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefix, ok := BrewPrefix(tt.exe)

			assert.Equal(t, tt.wantPrefix, prefix)
			assert.Equal(t, tt.wantOK, ok)
		})
	}
}

func TestGoBin(t *testing.T) {
	sep := string(filepath.ListSeparator)
	tests := []struct {
		name string
		env  map[string]string
		home string
		want string
	}{
		{"GOBIN wins", map[string]string{"GOBIN": "/custom/bin", "GOPATH": "/gp"}, "/home/a", "/custom/bin"},
		{"first GOPATH entry", map[string]string{"GOPATH": "/gp1" + sep + "/gp2"}, "/home/a", filepath.FromSlash("/gp1/bin")},
		{"home default", map[string]string{}, "/home/a", filepath.FromSlash("/home/a/go/bin")},
		{"empty GOBIN and GOPATH fall through", map[string]string{"GOBIN": "", "GOPATH": ""}, "/home/a", filepath.FromSlash("/home/a/go/bin")},
		{"nothing known", map[string]string{}, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GoBin(func(k string) string { return tt.env[k] }, tt.home)

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestUpgradeCommand(t *testing.T) {
	tests := []struct {
		method Method
		want   string
	}{
		{MethodHomebrew, "brew upgrade audiobookshelf-mcp"},
		{MethodNPM, "npx -y @sierikov/audiobookshelf-mcp@latest setup"},
		{MethodGo, "go install github.com/sierikov/audiobookshelf-mcp/cmd/audiobookshelf-mcp@latest"},
		{MethodUnknown, "download the .mcpb or binary for your platform from https://github.com/sierikov/audiobookshelf-mcp/releases/latest"},
		{Method("something else"), "download the .mcpb or binary for your platform from https://github.com/sierikov/audiobookshelf-mcp/releases/latest"},
	}
	for _, tt := range tests {
		t.Run(string(tt.method), func(t *testing.T) {
			assert.Equal(t, tt.want, UpgradeCommand(tt.method))
		})
	}
}
