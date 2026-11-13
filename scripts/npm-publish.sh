#!/bin/sh
# Publish the npm packages that scripts/npm-packages.sh wrote to dist/npm/.
#
# Usage: scripts/npm-publish.sh <version> <tag> [--dry-run]
#
# <version> is the release version without the leading "v", as the packages
# hold it. <tag> is the npm dist-tag: latest for a release, next for a
# pre-release, and dry-run for a rehearsal (npm wants a tag other than latest
# for a pre-release version, and a snapshot version is one). With --dry-run,
# npm goes through every step but the upload.
#
# The platform packages go first and the main package last, because the main
# package lists them as optional dependencies. A package whose
# <name>@<version> is already on npm is skipped, so a re-run after a partial
# publish carries on where it stopped.
#
# Messages go to stderr only.
set -eu
unset CDPATH

if [ "$#" -lt 2 ] || [ "$#" -gt 3 ] || { [ "$#" -eq 3 ] && [ "$3" != --dry-run ]; }; then
	echo "usage: $0 <version> <tag> [--dry-run]" >&2
	exit 2
fi

version=$1
tag=$2
dry_run=${3-}

case "$version" in
[0-9]*.[0-9]*.[0-9]*) ;;
*)
	echo "npm-publish.sh: version must look like 1.2.3, without a leading v: $version" >&2
	exit 2
	;;
esac

if [ -z "$tag" ]; then
	echo "npm-publish.sh: the npm tag is empty" >&2
	exit 2
fi

for tool in jq npm; do
	if ! command -v "$tool" >/dev/null 2>&1; then
		echo "npm-publish.sh: $tool is required but not installed" >&2
		exit 1
	fi
done

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

main=dist/npm/audiobookshelf-mcp

# Check every package before publishing any, so a stale or missing dist/npm/
# publishes nothing.
for dir in "$main"-* "$main"; do
	if [ ! -f "$dir/package.json" ]; then
		echo "npm-publish.sh: no package in $dir; run scripts/npm-packages.sh first" >&2
		exit 1
	fi
	have=$(jq -r .version "$dir/package.json")
	if [ "$have" != "$version" ]; then
		echo "npm-publish.sh: $dir holds version $have, not $version" >&2
		exit 1
	fi
done

# publish <dir> publishes the package in <dir>, unless npm already has its
# version.
publish() {
	name=$(jq -r .name "$1/package.json")
	if [ "$(npm view "$name@$version" version 2>/dev/null)" = "$version" ]; then
		echo "npm-publish.sh: $name@$version is already on npm, skipping" >&2
		return 0
	fi
	(cd "$1" && npm publish --access public --tag "$tag" ${dry_run:+"$dry_run"} >&2)
}

for dir in "$main"-*; do
	publish "$dir"
done
publish "$main"
