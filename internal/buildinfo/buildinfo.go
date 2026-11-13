// Package buildinfo reports the version of this build.
package buildinfo

import (
	"runtime/debug"
	"strings"

	"golang.org/x/mod/semver"
)

// devVersion is reported when the build carries no usable version.
const devVersion = "dev"

// version is set at link time with
// -ldflags "-X github.com/sierikov/audiobookshelf-mcp/internal/buildinfo.version=X.Y.Z".
var version string

// Version returns the version of this build without a leading "v": the one
// set at link time, else the module version that go install recorded, else
// "dev".
func Version() string {
	return versionFrom(version, mainVersion())
}

// mainVersion returns the main module version recorded in the binary, or ""
// when the binary carries no build info.
func mainVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	return info.Main.Version
}

// versionFrom picks the version from the link-time value and the main module
// version recorded in the binary. A leading "v" is dropped from either.
func versionFrom(ldflags, main string) string {
	if ldflags != "" {
		return strings.TrimPrefix(ldflags, "v")
	}
	if main == "" || main == "(devel)" {
		return devVersion
	}
	return strings.TrimPrefix(main, "v")
}

// IsRelease reports whether v is a plain release version such as "0.2.0":
// no leading "v", no pre-release or build suffix, all three numbers present.
// Pseudo-versions of local builds and "dev" are not releases.
func IsRelease(v string) bool {
	if strings.HasPrefix(v, "v") {
		return false
	}
	sv := "v" + v
	return semver.IsValid(sv) &&
		semver.Prerelease(sv) == "" &&
		semver.Build(sv) == "" &&
		semver.Canonical(sv) == sv
}
