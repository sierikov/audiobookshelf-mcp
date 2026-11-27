#!/bin/sh
# Wait until npm serves a version of a package.
#
# Usage: scripts/npm-wait-for-version.sh <package> <version> [seconds]
#
# <package> is the npm name (@sierikov/audiobookshelf-mcp), <version> the
# version without a leading "v", and [seconds] how long to wait at most
# (default 600). Exits 0 once npm serves that version, 1 when time runs out,
# and 2 on a bad argument or a missing tool.
#
# npm accepts a publish at once but serves the new version only a minute or
# two later, and the MCP Registry refuses to list an npm package version that
# npm doesn't serve yet. The release workflow runs this between the two.
#
# It asks GET <registry>/<package>/<version> every 15 seconds; anything but
# HTTP 200 counts as not yet. $NPM_REGISTRY_URL overrides the registry
# (default https://registry.npmjs.org).
#
# Messages go to stderr only.
set -eu
unset CDPATH

if [ "$#" -lt 2 ] || [ "$#" -gt 3 ]; then
	echo "usage: $0 <package> <version> [seconds]" >&2
	exit 2
fi

package=$1
version=$2
seconds=${3:-600}
registry=${NPM_REGISTRY_URL:-https://registry.npmjs.org}
interval=15

case "$package" in
'' | *' '*)
	echo "npm-wait-for-version.sh: not a package name: $package" >&2
	exit 2
	;;
esac

case "$version" in
[0-9]*.[0-9]*.[0-9]*) ;;
*)
	echo "npm-wait-for-version.sh: version must look like 1.2.3, without a leading v: $version" >&2
	exit 2
	;;
esac

case "$seconds" in
'' | *[!0-9]*)
	echo "npm-wait-for-version.sh: seconds must be a whole number: $seconds" >&2
	exit 2
	;;
esac

if ! command -v curl >/dev/null 2>&1; then
	echo "npm-wait-for-version.sh: curl is required but not installed" >&2
	exit 2
fi

# A scoped name's "/" and a version's "+" (build metadata) go into the path
# URL-encoded.
url=$registry/$(printf '%s' "$package" | sed 's|/|%2F|g')/$(printf '%s' "$version" | sed 's|+|%2B|g')

waited=0
while :; do
	status=$(curl -sS --max-time 30 -o /dev/null -w '%{http_code}' "$url" 2>/dev/null) || status=error
	if [ "$status" = 200 ]; then
		echo "npm-wait-for-version.sh: npm serves $package@$version (after ${waited}s)" >&2
		exit 0
	fi
	if [ "$waited" -ge "$seconds" ]; then
		echo "npm-wait-for-version.sh: npm still doesn't serve $package@$version after ${waited}s (last answer: $status)" >&2
		exit 1
	fi
	echo "npm-wait-for-version.sh: npm doesn't serve $package@$version yet ($status); checking again in ${interval}s" >&2
	sleep "$interval"
	waited=$((waited + interval))
done
