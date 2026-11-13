#!/bin/sh
# Assemble the npm packages of a release, in the esbuild/Biome layout: a main
# package with a small JS launcher, and one package per platform that holds
# the Go binary and is listed in the main package's optionalDependencies.
#
# Usage: scripts/npm-packages.sh <version> [binaries-dir]
#
# <version> is the release version without the leading "v" (GoReleaser's
# {{ .Version }}). The six packages are written to dist/npm/, ready for
# "npm publish" (platform packages first, the main package last). Each gets
# the repo's LICENSE, and the main package also its README.md.
#
# By default each platform's binary is the one GoReleaser built, found
# through dist/artifacts.json. With [binaries-dir] the binaries come from a
# directory of raw release assets instead, named
# audiobookshelf-mcp-{macos-arm64,macos-amd64,linux-amd64,linux-arm64}
# and audiobookshelf-mcp-windows-amd64.exe, as "gh release download" leaves
# them. A relative [binaries-dir] is relative to the caller's directory.
#
# Messages go to stderr only.
set -eu
unset CDPATH

if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
	echo "usage: $0 <version> [binaries-dir]" >&2
	exit 2
fi

version=$1
binaries=${2-}

case "$version" in
[0-9]*.[0-9]*.[0-9]*) ;;
*)
	echo "npm-packages.sh: version must look like 1.2.3, without a leading v: $version" >&2
	exit 2
	;;
esac

if ! command -v jq >/dev/null 2>&1; then
	echo "npm-packages.sh: jq is required but not installed" >&2
	exit 1
fi

# Resolve [binaries-dir] before changing directory: a relative path is
# relative to where the caller is, not to the repo root.
if [ -n "$binaries" ]; then
	if [ ! -d "$binaries" ]; then
		echo "npm-packages.sh: not a directory: $binaries" >&2
		exit 1
	fi
	binaries=$(cd "$binaries" && pwd)
fi

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

tmpl=npm/package.json.tmpl
launcher=npm/cli.js
license=LICENSE
readme=README.md
artifacts=dist/artifacts.json
out=dist/npm
exe=audiobookshelf-mcp

# One platform per line: Node's os and cpu, GoReleaser's goos and goarch, and
# the platform part of the raw asset name.
platforms='darwin arm64 darwin arm64 macos-arm64
darwin x64 darwin amd64 macos-amd64
linux x64 linux amd64 linux-amd64
linux arm64 linux arm64 linux-arm64
win32 x64 windows amd64 windows-amd64'

if [ -z "$binaries" ] && [ ! -f "$artifacts" ]; then
	echo "npm-packages.sh: $artifacts not found; build with GoReleaser first (make snapshot)" >&2
	exit 1
fi

for file in "$license" "$readme"; do
	if [ ! -f "$file" ]; then
		echo "npm-packages.sh: not found: $file" >&2
		exit 1
	fi
done

# find_binary <goos> <goarch> <asset platform> prints where that platform's
# binary is. With the artifacts list, it prints nothing unless exactly one
# binary matches.
find_binary() {
	if [ -n "$binaries" ]; then
		case "$1" in
		windows) echo "$binaries/audiobookshelf-mcp-$3.exe" ;;
		*) echo "$binaries/audiobookshelf-mcp-$3" ;;
		esac
		return
	fi
	jq -r --arg os "$1" --arg arch "$2" '
		[.[] | select(.type == "Binary" and .goos == $os and .goarch == $arch) | .path]
		| unique
		| if length == 1 then .[0] else empty end
	' "$artifacts"
}

# Check all five before writing anything, and name every one that is missing.
missing=0
while read -r _ _ goos goarch asset; do
	bin=$(find_binary "$goos" "$goarch" "$asset")
	if [ -z "$bin" ]; then
		echo "npm-packages.sh: no single $goos/$goarch binary in $artifacts" >&2
		missing=1
	elif [ ! -f "$bin" ]; then
		echo "npm-packages.sh: binary not found: $bin" >&2
		missing=1
	fi
done <<EOF
$platforms
EOF
if [ "$missing" -ne 0 ]; then
	exit 1
fi

# The platform suffixes, as the main package lists them in optionalDependencies.
suffixes=
while read -r node_os node_cpu _; do
	suffixes="$suffixes $node_os-$node_cpu"
done <<EOF
$platforms
EOF

main_name=$(jq -r .main.name "$tmpl")
main_dir=$out/${main_name#*/}

rm -rf "$out"
mkdir -p "$main_dir"

jq --arg v "$version" --arg suffixes "$suffixes" '
	.main as $m
	| {name: $m.name, version: $v} + $m + .common
	| .optionalDependencies = (
		[($suffixes | split(" "))[] | select(. != "") | {key: ($m.name + "-" + .), value: $v}]
		| from_entries
	)
' "$tmpl" >"$main_dir/package.json"
cp "$launcher" "$main_dir/cli.js"
chmod 755 "$main_dir/cli.js"
cp "$license" "$readme" "$main_dir/"
echo "npm-packages.sh: wrote $main_dir ($version)" >&2

while read -r node_os node_cpu goos goarch asset; do
	bin=$(find_binary "$goos" "$goarch" "$asset")
	name=$exe
	case "$node_os" in
	win32) name=$exe.exe ;;
	esac
	dir=$main_dir-$node_os-$node_cpu

	mkdir -p "$dir/bin"
	jq --arg v "$version" --arg os "$node_os" --arg cpu "$node_cpu" --arg exe "$name" '
		.main.name as $main
		| {name: ($main + "-" + $os + "-" + $cpu), version: $v}
		+ .platform
		+ {os: [$os], cpu: [$cpu], files: ["bin/" + $exe]}
		+ .common
	' "$tmpl" >"$dir/package.json"
	cp "$bin" "$dir/bin/$name"
	cp "$license" "$dir/"
	case "$node_os" in
	win32) ;;
	*) chmod 755 "$dir/bin/$name" ;;
	esac
	echo "npm-packages.sh: wrote $dir ($version)" >&2
done <<EOF
$platforms
EOF
