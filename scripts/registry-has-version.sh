#!/bin/sh
# Tell whether the MCP Registry already has a version of a server.
#
# Usage: scripts/registry-has-version.sh <name> <version>
#
# <name> is the server's name as server.json holds it
# (io.github.sierikov/audiobookshelf-mcp), and <version> the version without
# a leading "v". Exits 0 when the registry has that version, 1 when it does
# not, and 2 on any other error (a bad argument, the network, an unexpected
# answer).
#
# It asks the registry's read endpoint,
# GET /v0.1/servers/<name>/versions/<version>, with include_deleted=true: the
# registry refuses to publish a version it has ever had, deleted or not, so a
# deleted version counts as there. $MCP_REGISTRY_URL overrides the registry
# (default https://registry.modelcontextprotocol.io).
#
# Messages go to stderr only.
set -eu
unset CDPATH

if [ "$#" -ne 2 ]; then
	echo "usage: $0 <name> <version>" >&2
	exit 2
fi

name=$1
version=$2
registry=${MCP_REGISTRY_URL:-https://registry.modelcontextprotocol.io}

case "$name" in
?*/?*) ;;
*)
	echo "registry-has-version.sh: name must look like io.github.owner/server: $name" >&2
	exit 2
	;;
esac

case "$version" in
[0-9]*.[0-9]*.[0-9]*) ;;
*)
	echo "registry-has-version.sh: version must look like 1.2.3, without a leading v: $version" >&2
	exit 2
	;;
esac

for tool in curl jq; do
	if ! command -v "$tool" >/dev/null 2>&1; then
		echo "registry-has-version.sh: $tool is required but not installed" >&2
		exit 2
	fi
done

# The name's "/" and a version's "+" (build metadata) go into the path
# URL-encoded.
url=$registry/v0.1/servers/$(printf '%s' "$name" | sed 's|/|%2F|g')/versions/$(printf '%s' "$version" | sed 's|+|%2B|g')?include_deleted=true

tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT
trap 'exit 2' HUP INT TERM

if ! status=$(curl -sS --max-time 30 --retry 3 -o "$tmp" -w '%{http_code}' "$url"); then
	echo "registry-has-version.sh: could not ask $url" >&2
	exit 2
fi

case "$status" in
200)
	if jq -e --arg name "$name" --arg version "$version" \
		'.server.name == $name and .server.version == $version' "$tmp" >/dev/null 2>&1; then
		echo "registry-has-version.sh: the registry has $name $version" >&2
		exit 0
	fi
	echo "registry-has-version.sh: $url answered with another server or version" >&2
	exit 2
	;;
404)
	echo "registry-has-version.sh: the registry does not have $name $version" >&2
	exit 1
	;;
*)
	echo "registry-has-version.sh: $url answered HTTP $status" >&2
	exit 2
	;;
esac
