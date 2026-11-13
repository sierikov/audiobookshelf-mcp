#!/bin/sh
# Package one built binary as an .mcpb bundle for Claude Desktop.
#
# Usage: scripts/mcpb.sh <binary path> <target> <version>
#
# <target> is GoReleaser's {{ .Target }} (e.g. darwin_arm64_v8.0). The bundle
# is written to dist/mcpb/audiobookshelf-mcp-<platform>.mcpb and carries the
# repo-root manifest.json with its version set to <version>.
#
# Called from a GoReleaser build hook. Messages go to stderr only.
set -eu
unset CDPATH

if [ "$#" -ne 3 ]; then
	echo "usage: $0 <binary path> <target> <version>" >&2
	exit 2
fi

bin=$1
target=$2
version=$3

case "$target" in
darwin_arm64*) platform=macos-arm64 ;;
darwin_amd64*) platform=macos-amd64 ;;
linux_amd64*) platform=linux-amd64 ;;
linux_arm64*) platform=linux-arm64 ;;
windows_amd64*) platform=windows-amd64 ;;
*)
	echo "mcpb.sh: unsupported target: $target" >&2
	exit 1
	;;
esac

for tool in jq zip; do
	if ! command -v "$tool" >/dev/null 2>&1; then
		echo "mcpb.sh: $tool is required but not installed" >&2
		exit 1
	fi
done

if [ ! -f "$bin" ]; then
	echo "mcpb.sh: binary not found: $bin" >&2
	exit 1
fi

# Resolve the binary before changing directory, because GoReleaser passes a
# path relative to the project root.
bin=$(cd "$(dirname "$bin")" && pwd)/$(basename "$bin")
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

exe=audiobookshelf-mcp
jq_filter='.version = $v'
if [ "$platform" = windows-amd64 ]; then
	exe=audiobookshelf-mcp.exe
	jq_filter="$jq_filter | .server.entry_point += \".exe\" | .server.mcp_config.command += \".exe\""
fi

stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
trap 'exit 1' HUP INT TERM

jq --arg v "$version" "$jq_filter" manifest.json >"$stage/manifest.json"
if [ "$platform" = windows-amd64 ] &&
	! jq -e '.server.entry_point == "audiobookshelf-mcp.exe" and .server.mcp_config.command == "${__dirname}/audiobookshelf-mcp.exe"' "$stage/manifest.json" >/dev/null; then
	echo "mcpb.sh: the Windows manifest keys server.entry_point and server.mcp_config.command are missing or renamed in manifest.json" >&2
	exit 1
fi
cp icon.png "$stage/icon.png"
cp "$bin" "$stage/$exe"

mkdir -p dist/mcpb
out="$root/dist/mcpb/audiobookshelf-mcp-$platform.mcpb"
rm -f "$out"
(cd "$stage" && zip -q -X "$out" manifest.json icon.png "$exe") >&2

echo "mcpb.sh: wrote dist/mcpb/audiobookshelf-mcp-$platform.mcpb ($version)" >&2
