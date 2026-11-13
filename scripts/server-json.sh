#!/bin/sh
# Stamp server.json, the MCP Registry entry, for a release.
#
# Usage: scripts/server-json.sh <version>
#
# <version> is the release version without the leading "v" (GoReleaser's
# {{ .Version }}). The file named by $SERVER_JSON (default server.json at the
# repo root; a relative path is relative to the repo root) is rewritten in
# place with:
#
#   - the version;
#   - one mcpb package per platform, pointing at the GitHub release asset and
#     carrying the SHA-256 of the matching bundle in dist/mcpb/, which
#     GoReleaser builds through scripts/mcpb.sh;
#   - the version of the npm package entry the file already holds.
#
# Everything else in the file (name, description, repository, the npm package
# entry apart from its version) is kept. The committed server.json holds the
# placeholder version 0.0.0-dev.
#
# Messages go to stderr only.
set -eu
unset CDPATH

if [ "$#" -ne 1 ]; then
	echo "usage: $0 <version>" >&2
	exit 2
fi

version=$1

case "$version" in
[0-9]*.[0-9]*.[0-9]*) ;;
*)
	echo "server-json.sh: version must look like 1.2.3, without a leading v: $version" >&2
	exit 2
	;;
esac

if ! command -v jq >/dev/null 2>&1; then
	echo "server-json.sh: jq is required but not installed" >&2
	exit 1
fi

# sha256 prints the SHA-256 of a file, as hex.
sha256() {
	if command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | cut -d' ' -f1
	elif command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		echo "server-json.sh: shasum or sha256sum is required but neither is installed" >&2
		return 1
	fi
}

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

target=${SERVER_JSON:-server.json}
bundles=dist/mcpb

if [ ! -f "$target" ]; then
	echo "server-json.sh: not found: $target" >&2
	exit 1
fi

repo=$(jq -r '.repository.url // empty' "$target")
if [ -z "$repo" ]; then
	echo "server-json.sh: $target has no repository.url" >&2
	exit 1
fi
repo=${repo%/}

# Platforms, as they appear in the asset names: audiobookshelf-mcp-<platform>.mcpb.
platforms='macos-arm64 macos-amd64 linux-amd64 linux-arm64 windows-amd64'

# Check all five bundles before writing anything, and name every one that is
# missing.
missing=0
for platform in $platforms; do
	if [ ! -f "$bundles/audiobookshelf-mcp-$platform.mcpb" ]; then
		echo "server-json.sh: bundle not found: $bundles/audiobookshelf-mcp-$platform.mcpb (build with GoReleaser first, make snapshot)" >&2
		missing=1
	fi
done
if [ "$missing" -ne 0 ]; then
	exit 1
fi

# One "<asset name> <sha256>" line per bundle.
sums=
for platform in $platforms; do
	name=audiobookshelf-mcp-$platform.mcpb
	sum=$(sha256 "$bundles/$name")
	if ! printf '%s\n' "$sum" | grep -Eq '^[a-f0-9]{64}$'; then
		echo "server-json.sh: not a SHA-256 for $bundles/$name: '$sum'" >&2
		exit 1
	fi
	sums="$sums$name $sum
"
done

# The npm entry is kept from the file, so a file without one would publish none.
if [ "$(jq '[.packages[] | select(.registryType == "npm")] | length' "$target")" != 1 ]; then
	echo "server-json.sh: $target must hold exactly one npm package entry" >&2
	exit 1
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' HUP INT TERM

jq --arg v "$version" --arg base "$repo/releases/download/v$version" --arg sums "$sums" '
	.version = $v
	| .packages = (
		[$sums | split("\n")[] | select(. != "") | split(" ") | {
			registryType: "mcpb",
			identifier: ($base + "/" + .[0]),
			fileSha256: .[1],
			transport: {type: "stdio"}
		}]
		+ [.packages[] | select(.registryType == "npm") | .version = $v]
	)
' "$target" >"$tmp/server.json"

# Write through cat rather than mv, to keep the file's mode.
cat "$tmp/server.json" >"$target"

echo "server-json.sh: wrote $target ($version, $(jq '.packages | length' "$target") packages)" >&2
