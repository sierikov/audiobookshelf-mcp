package update

import (
	"path"
	"path/filepath"
	"strings"
)

// Method is the way this binary was installed, which decides the command that
// upgrades it.
type Method string

// The install methods DetectMethod tells apart.
const (
	// MethodHomebrew is a Homebrew formula or cask.
	MethodHomebrew Method = "homebrew"
	// MethodNPM is the npm package, usually run through npx.
	MethodNPM Method = "npm"
	// MethodGo is go install.
	MethodGo Method = "go"
	// MethodUnknown is anything else: a downloaded release, a repo build.
	MethodUnknown Method = "unknown"
)

const (
	// brewPackageDir is the directory Homebrew keeps this package in, under
	// each of brewRoots.
	brewPackageDir = "audiobookshelf-mcp/"
	// npmPackageMarker starts the path of the npm package and of its
	// per-platform packages (@sierikov/audiobookshelf-mcp-darwin-arm64 and so on).
	npmPackageMarker = "/node_modules/@sierikov/audiobookshelf-mcp"
)

// brewRoots are the Homebrew directories that hold installed packages: the
// Cellar for formulae, the Caskroom for casks.
var brewRoots = []string{"/Cellar/", "/Caskroom/"}

// DetectMethod works out how the binary at exe was installed. exe must
// already be resolved through symlinks, because Homebrew and npm both link
// into their own bin directories. goBin is the directory go install writes to
// (see GoBin); empty means unknown. Windows paths are recognised on any
// system.
func DetectMethod(exe, goBin string) Method {
	p := toSlash(exe)
	for _, root := range brewRoots {
		if strings.Contains(p, root+brewPackageDir) {
			return MethodHomebrew
		}
	}
	if strings.Contains(p, npmPackageMarker) {
		return MethodNPM
	}
	if goBin != "" && path.Dir(p) == path.Clean(toSlash(goBin)) {
		return MethodGo
	}
	return MethodUnknown
}

// BrewPrefix returns the Homebrew prefix of an executable installed by
// Homebrew: the part of the path before /Cellar/ or /Caskroom/. The second
// result is false when exe is not under either.
func BrewPrefix(exe string) (string, bool) {
	best := -1
	for _, root := range brewRoots {
		if i := strings.Index(exe, root); i >= 0 && (best < 0 || i < best) {
			best = i
		}
	}
	if best < 0 {
		return "", false
	}
	return exe[:best], true
}

// GoBin returns the directory go install writes binaries to: $GOBIN, else the
// bin directory of the first $GOPATH entry, else $HOME/go/bin. It returns ""
// when none of those is known.
func GoBin(getenv func(string) string, home string) string {
	if dir := getenv("GOBIN"); dir != "" {
		return dir
	}
	if paths := filepath.SplitList(getenv("GOPATH")); len(paths) > 0 && paths[0] != "" {
		return filepath.Join(paths[0], "bin")
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, "go", "bin")
}

// UpgradeCommand returns the instruction that upgrades an install made with m.
// It is phrased to follow "Upgrade: " and to fit in parentheses after "how to
// upgrade".
func UpgradeCommand(m Method) string {
	switch m {
	case MethodHomebrew:
		return "brew upgrade audiobookshelf-mcp"
	case MethodNPM:
		return "npx -y @sierikov/audiobookshelf-mcp@latest setup"
	case MethodGo:
		return "go install github.com/sierikov/audiobookshelf-mcp/cmd/audiobookshelf-mcp@latest"
	default:
		return "download the .mcpb or binary for your platform from https://github.com/sierikov/audiobookshelf-mcp/releases/latest"
	}
}

// toSlash turns Windows separators into slashes, so one set of rules matches
// paths from any system.
func toSlash(p string) string {
	return strings.ReplaceAll(p, `\`, "/")
}
