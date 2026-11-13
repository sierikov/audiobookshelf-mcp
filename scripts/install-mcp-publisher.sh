#!/bin/sh
# Install mcp-publisher, the MCP Registry's command-line tool, for the release
# workflow.
#
# Usage: scripts/install-mcp-publisher.sh
#
# Downloads the pinned version's linux/amd64 build (the workflow's runners),
# checks it against the pinned SHA-256, and writes ./mcp-publisher at the
# repo root. Needs curl, sha256sum and tar.
#
# Messages go to stderr only.
set -eu
unset CDPATH

# From registry_1.8.1_checksums.txt in that release; change both lines together.
version=v1.8.1
sha256=a06c9096dcb9727c13555b6be26c7effa707b01f06a4c561ba7a3635443cf2cc

url=https://github.com/modelcontextprotocol/registry/releases/download/$version/mcp-publisher_linux_amd64.tar.gz

if [ "$#" -ne 0 ]; then
	echo "usage: $0" >&2
	exit 2
fi

root=$(cd "$(dirname "$0")/.." && pwd)

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' HUP INT TERM

curl -fsSL --max-time 60 -o "$tmp/mcp-publisher.tar.gz" "$url"
echo "$sha256  $tmp/mcp-publisher.tar.gz" | sha256sum -c - >&2
tar xzf "$tmp/mcp-publisher.tar.gz" -C "$root" mcp-publisher

echo "install-mcp-publisher.sh: wrote $root/mcp-publisher ($version)" >&2
