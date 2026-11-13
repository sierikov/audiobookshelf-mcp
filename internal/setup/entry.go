package setup

import (
	"path"

	"github.com/sierikov/audiobookshelf-mcp/internal/agents"
	"github.com/sierikov/audiobookshelf-mcp/internal/update"
)

const (
	// binaryName is the executable's name, which Homebrew links into its bin
	// directory.
	binaryName = "audiobookshelf-mcp"
	// npmPackage is the npm package that carries this server.
	npmPackage = "@sierikov/audiobookshelf-mcp"
)

// EntryFor returns the command agents should run to start this server, for
// the binary at exe that was installed with m. version is the version of the
// running build, without a leading "v".
//
// An npm install gets an npx command pinned to that version, so the agent
// keeps running the version setup was run with. Homebrew gets the stable
// bin-directory link, which keeps working after an upgrade replaces the
// versioned directory exe lives in. Anything else runs exe itself.
func EntryFor(m update.Method, exe, version string) agents.Entry {
	switch m {
	case update.MethodNPM:
		return agents.Entry{Command: "npx", Args: []string{"-y", npmPackage + "@" + version}}
	case update.MethodHomebrew:
		if prefix, ok := update.BrewPrefix(exe); ok {
			return agents.Entry{Command: path.Join(prefix, "bin", binaryName)}
		}
	default:
		// go install and unknown installs run exe itself.
	}
	return agents.Entry{Command: exe}
}
