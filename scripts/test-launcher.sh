#!/bin/sh
# End-to-end test for the npm packages, the launcher and server-json.sh.
#
# Usage: sh scripts/test-launcher.sh
#
# Builds the packages with scripts/npm-packages.sh, packs them, installs the
# main package and this machine's platform package into a throwaway project,
# and runs the launcher from there: its version, a clean stdout, the exit
# code, and the failures on an unsupported platform and on a supported one
# whose package is missing. It also checks the generated package metadata and
# contents, and stamps a copy of server.json with scripts/server-json.sh.
# Prints "ok: <case>" per case, and exits 1 on the first failure. Needs go,
# jq, node and npm.
set -eu
unset CDPATH

version=0.0.0-test
module=github.com/sierikov/audiobookshelf-mcp

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

tmp=$(mktemp -d)
cleanup() { rm -rf "$tmp"; }
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

ok() { echo "ok: $*"; }
fail() {
	echo "FAIL: $*" >&2
	exit 1
}

# sha256 prints the SHA-256 of a file, as hex.
sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

# 1. Packaging from GoReleaser's real output. dist/artifacts.json is what the
#    release workflow feeds the script, so use a snapshot build as it is.
if [ ! -f dist/artifacts.json ]; then
	HOMEBREW_TAP_GITHUB_TOKEN=${HOMEBREW_TAP_GITHUB_TOKEN:-x} make snapshot >&2
fi
sh scripts/npm-packages.sh "$version" >&2

for entry in darwin-arm64:macos-arm64 darwin-x64:macos-amd64 linux-x64:linux-amd64 linux-arm64:linux-arm64 win32-x64:windows-amd64; do
	node_target=${entry%%:*}
	asset=audiobookshelf-mcp-${entry#*:}
	exe=audiobookshelf-mcp
	case $node_target in
	win32-*)
		asset=$asset.exe
		exe=$exe.exe
		;;
	esac
	want=$(jq -r --arg n "$asset" '.[] | select(.type == "Binary" and .name == $n) | .extra.Checksum' dist/artifacts.json | sed 's/^sha256://')
	[ -n "$want" ] || fail "no checksum for $asset in dist/artifacts.json"
	got=$(sha256 "dist/npm/audiobookshelf-mcp-$node_target/bin/$exe")
	[ "$want" = "$got" ] || fail "audiobookshelf-mcp-$node_target holds the wrong binary"
done
ok "packages carry each platform's binary from dist/artifacts.json"

# 2. The binaries GoReleaser builds in a snapshot carry the snapshot version,
#    so build ones stamped with the test version and package those instead,
#    from a directory of raw-named release assets.
bins=$tmp/bins
mkdir "$bins"
for target in macos-arm64:darwin:arm64 macos-amd64:darwin:amd64 linux-amd64:linux:amd64 linux-arm64:linux:arm64 windows-amd64:windows:amd64; do
	asset=${target%%:*}
	rest=${target#*:}
	goos=${rest%%:*}
	goarch=${rest#*:}
	ext=
	case $goos in windows) ext=.exe ;; esac
	CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build -trimpath \
		-ldflags "-s -w -X $module/internal/buildinfo.version=$version" \
		-o "$bins/audiobookshelf-mcp-$asset$ext" ./cmd/audiobookshelf-mcp
done
sh scripts/npm-packages.sh "$version" "$bins" >&2

# 3. Pack every package, and install the main one with this platform's into a
#    throwaway project.
tgz=$tmp/tgz
proj=$tmp/proj
mkdir "$tgz" "$proj"
for dir in dist/npm/*; do
	(cd "$dir" && npm pack --silent --pack-destination "$tgz" >/dev/null)
done

here=$(node -p 'process.platform + "-" + process.arch')
printf '{"private": true}\n' >"$proj/package.json"
(cd "$proj" && npm install --no-audit --no-fund --silent \
	"$tgz/sierikov-audiobookshelf-mcp-$version.tgz" \
	"$tgz/sierikov-audiobookshelf-mcp-$here-$version.tgz" >&2)

launcher=$proj/node_modules/@sierikov/audiobookshelf-mcp/cli.js
export AUDIOBOOKSHELF_MCP_NO_UPDATE_CHECK=1

# Case: version, through the bin link that npx uses.
got=$(cd "$proj" && npx --no-install audiobookshelf-mcp version)
[ "$got" = "audiobookshelf-mcp $version" ] || fail "version: got '$got'"
ok "version"

# Case: stdout carries nothing but the MCP stream. jq fails on any line that
# is not JSON, so a banner from the launcher would fail this.
initialize='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"c","version":"0"}}}'
got=$( (
	printf '%s\n' "$initialize"
	sleep 1
) | AUDIOBOOKSHELF_URL=http://x AUDIOBOOKSHELF_TOKEN=x node "$launcher" |
	jq -r 'select(.id == 1).result.serverInfo.version')
[ "$got" = "$version" ] || fail "initialize: serverInfo.version is '$got'"
ok "stdout is clean for MCP"

# Case: the child's exit code comes through.
code=0
node "$launcher" bogus >/dev/null 2>&1 || code=$?
[ "$code" -eq 2 ] || fail "exit code: got $code, want 2"
ok "exit code passes through"

# Case: an unsupported platform fails on stderr alone.
code=0
err=$(node -e 'Object.defineProperty(process, "platform", {value: "freebsd"}); require(process.argv[1])' "$launcher" 2>&1 >"$tmp/stdout") || code=$?
[ "$code" -eq 1 ] || fail "unsupported: exit code $code, want 1"
[ ! -s "$tmp/stdout" ] || fail "unsupported: stdout is not empty"
case $err in
*"no build for freebsd"*) ;;
*) fail "unsupported: stderr is '$err'" ;;
esac
ok "unsupported platform"

# Case: a supported platform whose package is not installed gets its own
# message. A copy of the launcher, with the package.json that lists the
# platform packages, in a directory with no node_modules cannot resolve the
# platform package, so it behaves like --omit=optional.
bare=$tmp/bare
mkdir "$bare"
cp "$launcher" "$(dirname "$launcher")/package.json" "$bare/"
code=0
err=$(cd "$bare" && NODE_PATH= node cli.js 2>&1 >"$tmp/stdout") || code=$?
[ "$code" -eq 1 ] || fail "missing package: exit code $code, want 1"
[ ! -s "$tmp/stdout" ] || fail "missing package: stdout is not empty"
case $err in
*"the package @sierikov/audiobookshelf-mcp-$here is missing. Reinstall without --omit=optional"*) ;;
*) fail "missing package: stderr is '$err'" ;;
esac
case $err in
*"no build for"*) fail "missing package: got the unsupported-platform message: '$err'" ;;
esac
ok "missing platform package"

# Case: metadata of the generated packages.
main=dist/npm/audiobookshelf-mcp/package.json
[ "$(jq -r .mcpName "$main")" = io.github.sierikov/audiobookshelf-mcp ] || fail "mcpName"
jq -e --arg v "$version" '
	[.optionalDependencies[]] as $deps
	| .version == $v and ($deps | length) == 5 and ($deps | all(. == $v))
' "$main" >/dev/null || fail "optionalDependencies are not all $version"
[ "$(jq -r '.optionalDependencies | keys | join(" ")' "$main")" = \
	"@sierikov/audiobookshelf-mcp-darwin-arm64 @sierikov/audiobookshelf-mcp-darwin-x64 @sierikov/audiobookshelf-mcp-linux-arm64 @sierikov/audiobookshelf-mcp-linux-x64 @sierikov/audiobookshelf-mcp-win32-x64" ] ||
	fail "optionalDependencies name the wrong packages"
for dir in dist/npm/audiobookshelf-mcp-*; do
	jq -e --arg v "$version" '
		.version == $v and (.os | length) == 1 and (.cpu | length) == 1
		and .bin == null and .scripts == null and .exports == null
	' "$dir/package.json" >/dev/null || fail "$dir/package.json"
done
[ "$(ls dist/npm | wc -l | tr -d ' ')" -eq 6 ] || fail "dist/npm does not hold six packages"
[ "$(tar -tzf "$tgz/sierikov-audiobookshelf-mcp-$version.tgz" | LC_ALL=C sort | tr '\n' ' ')" = "package/LICENSE package/README.md package/cli.js package/package.json " ] ||
	fail "the main package does not hold exactly LICENSE, README.md, cli.js and package.json"
for tarball in "$tgz"/sierikov-audiobookshelf-mcp-*-"$version".tgz; do
	tar -tzf "$tarball" | grep -qx 'package/LICENSE' || fail "$tarball has no LICENSE"
done
ok "metadata"

# Case: the registry entry. The script rewrites the file named by SERVER_JSON,
# so run it on a copy and leave the committed placeholder as it is.
cp server.json "$tmp/server.json"
SERVER_JSON=$tmp/server.json sh scripts/server-json.sh "$version" >&2 || fail "server-json.sh exited non-zero"
srv=$tmp/server.json
release=https://github.com/sierikov/audiobookshelf-mcp/releases/download/v$version
jq -e --arg v "$version" '
	.version == $v and (.packages | length) == 6
	and ([.packages[] | select(.registryType == "mcpb")] | length) == 5
	and ([.packages[] | select(.registryType == "npm")] | length) == 1
' "$srv" >/dev/null || fail "server.json: wrong version or package count"
jq -e --arg v "$version" '
	.packages[] | select(.registryType == "npm")
	| .identifier == "@sierikov/audiobookshelf-mcp" and .version == $v
	and .runtimeHint == "npx" and .transport == {type: "stdio"}
	and ([.environmentVariables[].name] == ["AUDIOBOOKSHELF_URL", "AUDIOBOOKSHELF_TOKEN", "AUDIOBOOKSHELF_TOOLSETS"])
' "$srv" >/dev/null || fail "server.json: wrong npm package"
for asset in macos-arm64 macos-amd64 linux-amd64 linux-arm64 windows-amd64; do
	file=audiobookshelf-mcp-$asset.mcpb
	[ -f "dist/mcpb/$file" ] || fail "dist/mcpb/$file is missing; run make snapshot"
	want=$(sha256 "dist/mcpb/$file")
	got=$(jq -r --arg id "$release/$file" '[.packages[] | select(.registryType == "mcpb" and .identifier == $id)] | if length == 1 then .[0].fileSha256 else "none" end' "$srv")
	[ "$got" = "$want" ] || fail "server.json: $file has fileSha256 '$got', want '$want'"
done
jq -e '[.packages[] | select(.registryType == "mcpb") | .transport == {type: "stdio"}] | all' "$srv" >/dev/null ||
	fail "server.json: an mcpb package lacks the stdio transport"
ok "server.json"
