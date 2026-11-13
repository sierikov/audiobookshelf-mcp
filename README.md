# Audiobookshelf MCP Server

[![Go](https://github.com/sierikov/audiobookshelf-mcp/actions/workflows/go.yml/badge.svg)](https://github.com/sierikov/audiobookshelf-mcp/actions/workflows/go.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/sierikov/audiobookshelf-mcp)](https://goreportcard.com/report/github.com/sierikov/audiobookshelf-mcp)
[![MCP Registry](https://img.shields.io/badge/MCP%20Registry-published-light)](https://registry.modelcontextprotocol.io/v0.1/servers?search=io.github.sierikov/audiobookshelf-mcp)

A [Model Context Protocol](https://modelcontextprotocol.io) server for [Audiobookshelf](https://www.audiobookshelf.org/) - gives AI assistants access to your self-hosted audiobook and podcast library.

MCP Registry name: `mcp-name: io.github.sierikov/audiobookshelf-mcp`

## What can it do?

Ask your AI assistant things like:

- "What am I currently listening to?"
- "Search for Dune in my library"
- "What are my listening stats?"
- "What did I listen to recently?"
- "How far am I in Katabasis?"
- "What libraries do I have?"

## Quick start

One command connects the server to your Audiobookshelf and adds it to your AI agents. Pick the way you like to install:

**npx** (needs Node.js 18+):

```bash
npx -y @sierikov/audiobookshelf-mcp setup
```

**Homebrew:**

```bash
brew install sierikov/tap/audiobookshelf-mcp
audiobookshelf-mcp setup
```

Needs Homebrew 6 or newer (the cask uses install steps). Installing by the full name also satisfies Homebrew's tap-trust check. The cask removes the quarantine flag so Gatekeeper does not prompt. Linux support for the cask is not confirmed yet: on Linux, use npx or `go install`.

**go install:**

```bash
go install github.com/sierikov/audiobookshelf-mcp/cmd/audiobookshelf-mcp@latest
audiobookshelf-mcp setup
```

The Go bin directory (`$(go env GOPATH)/bin`, or `$GOBIN` if you set it) must be on your `PATH` for `audiobookshelf-mcp setup` to work.

`setup` asks for:

1. Your Audiobookshelf URL and API token (Settings → Users → your user → **API Token**). The token is hidden as you type. `setup` checks them and prints `Connected to … as …`.
2. Whether to turn on the tools that edit your library (chapters, metadata). The default is no.
3. Which of the agents it found to add the server to. Press Enter for all of them.

It then saves your settings and adds the server to those agents, after backing up each config file it edits. The agents' configs hold only the command to run, never your token. It looks for these agents:

| Agent | ID for `--agents` |
|---|---|
| Claude Desktop | `claude-desktop` |
| Claude Code | `claude-code` |
| Codex | `codex` |
| OpenCode | `opencode` |
| Hermes Agent | `hermes` |
| Antigravity CLI | `antigravity` |
| Gemini CLI | `gemini` |
| Cursor | `cursor` |

Restart your agent, then ask it about your library. If you configured Claude Code by hand before (`claude mcp add`), also run `claude mcp remove audiobookshelf -s local` in that project, because that local entry wins over the one `setup` writes.

`setup` takes flags for the questions it asks, so you can script it:

| Flag | What it does |
|---|---|
| `--url URL` | Your Audiobookshelf URL |
| `--token TOKEN` | Your API token. Other programs on the machine can see it in the process list, and your shell may keep it in its history, so the prompt is safer |
| `--toolsets a,b` | Toolsets to use, as a comma-separated list. It replaces the defaults (`libraries,items,playback`): for example `libraries,items,playback,chapters,metadata`, or `all`. An empty value restores the defaults |
| `--agents id,id` | Agents to add the server to, by ID. `none` adds it to no agent |
| `--yes` | Never ask: use the flags, the saved settings and the defaults |

Run `setup` again to change the token, to add an agent you installed since, or to move agents that use npx to a new version.

## Updating

Upgrade the way you installed:

| Installed with | Upgrade |
|---|---|
| Homebrew | `brew upgrade audiobookshelf-mcp` |
| npx | `npx -y @sierikov/audiobookshelf-mcp@latest setup` |
| go install | `go install github.com/sierikov/audiobookshelf-mcp/cmd/audiobookshelf-mcp@latest` |
| A downloaded binary or the Claude Desktop extension | Download the `.mcpb` or binary for your platform from [GitHub releases](https://github.com/sierikov/audiobookshelf-mcp/releases/latest) |

With npx, `setup` pins each agent to the version it ran with, so running it again with `@latest` is what moves them forward.

At most once a day, `audiobookshelf-mcp version`, `setup` and the running server ask GitHub whether a newer release exists (one request with your version in the User-Agent, nothing else). `version` and `setup` print the answer. The server asks your agent to mention a new release once in a conversation where you use its tools, until you upgrade, with the upgrade command for your install. For a downloaded binary or the extension, the text names the download. Restart the app after upgrading. Set `AUDIOBOOKSHELF_MCP_NO_UPDATE_CHECK=1` to turn the check off.

## Where settings live

`setup` saves your settings to a file only you can read (mode `0600`):

| System | Settings file |
|---|---|
| macOS, Linux | `$XDG_CONFIG_HOME/audiobookshelf-mcp/config.json`, which is `~/.config/audiobookshelf-mcp/config.json` unless `XDG_CONFIG_HOME` is set |
| Windows | `%AppData%\audiobookshelf-mcp\config.json` |

```json
{
  "url": "https://abs.example.com",
  "token": "your-token",
  "toolsets": ["all"]
}
```

If `XDG_CONFIG_HOME` points somewhere other than `~/.config`, apps started outside your shell, such as Claude Desktop or Cursor, may not see it and will look in `~/.config` instead. `setup` warns you when that is the case.

Environment variables override the file. Re-run `setup` to change the token.

| Variable | Required | Default | Description |
|---|---|---|---|
| `AUDIOBOOKSHELF_URL` | Unless saved by `setup` | — | Your Audiobookshelf server URL |
| `AUDIOBOOKSHELF_TOKEN` | Unless saved by `setup` | — | API token from Settings → Users |
| `AUDIOBOOKSHELF_TOOLSETS` | No | `libraries,items,playback` | Comma-separated toolsets to enable: `libraries`, `items`, `playback`, `browse`, `chapters`, `metadata`. Use `all` to enable everything, including the `chapters` and `metadata` write tools. |

## Tools

| Tool | Description | Toolset |
|---|---|---|
| `list_libraries` | List all libraries | libraries |
| `search_library` | Search by title, author, or narrator | libraries |
| `list_library_items` | List items with pagination | libraries |
| `get_item` | Get detailed info about an audiobook or podcast, including its tags | items |
| `get_items_in_progress` | Get currently listening items with progress | items |
| `get_listening_stats` | Total listening time and daily breakdown | playback |
| `get_listening_sessions` | Recent listening sessions | playback |
| `get_media_progress` | Progress for a specific item | playback |
| `list_series` | List all series in a library | browse |
| `get_series` | Get series details with books | browse |
| `get_author` | Get author details with books | browse |
| `list_collections` | List collections in a library | browse |
| `get_chapters` | A book's chapters, duration and audio files | chapters |
| `search_audible` | Find matching Audible editions, closest runtime first | chapters |
| `get_audible_chapters` | Audible's chapter list for an ASIN | chapters |
| `set_chapters` | Replace all of a book's chapters (writes) | chapters |
| `rename_chapters` | Rename chapters, keeping their timestamps (writes) | chapters |
| `get_metadata_overview` | Count every genre, tag and language in a library, and the books missing them | metadata |
| `find_items` | List books with their genres, tags and language, filtered; `detail: full` adds a description preview | metadata |
| `search_book_metadata` | Look a book up in one metadata source (fantlab, google, audible, openlibrary, itunes) | metadata |
| `add_genres` | Add genres to books (writes) | metadata |
| `remove_genres` | Remove genres from books (writes) | metadata |
| `add_tags` | Add tags to books (writes) | metadata |
| `remove_tags` | Remove tags from books (writes) | metadata |
| `update_items` | Set the language and description of books (writes) | metadata |

Default toolsets: `libraries`, `items`, `playback`. The `browse`, `chapters` and `metadata` toolsets are opt-in via `AUDIOBOOKSHELF_TOOLSETS`. `setup` asks whether to turn on the editing toolsets (`chapters`, `metadata`), and `--toolsets` sets the whole list.

### Fixing chapters

With the `chapters` toolset enabled, ask things like *"Fix the chapters for Katabasis"*. The assistant reads the book's current chapters, searches Audible for the edition whose runtime matches your files, fetches its chapters, replaces generic titles ("Chapter 1, 2, 3") with real ones, and shows you the list before saving. When Audible has no match, it can build the list from its own research or from timestamps you give it. Every save returns the previous chapters, so a change can be undone, as long as those old chapters were valid (old chapters that run past the end of the book, for example, cannot be saved back).

### Cleaning up genres, tags and descriptions

With the `metadata` toolset enabled, ask things like *"Make my genres consistent"* or *"Fix the descriptions of the Harry Hole books"*. The assistant surveys the library with `get_metadata_overview`, proposes a mapping from the old genres or tags to the new ones, and shows you the affected books. After you say yes, it adds the new value first and then removes the old one, so no book is left without it. For descriptions it looks up real ones with `search_book_metadata`, shows each change from old to new, and tells you when it wrote a description itself. Every write returns what it changed, so a change can be undone: the genre and tag tools list each book's `added` or `removed` values, and `update_items` returns the `previous` values.

---

## Manual setup

Prefer to configure things yourself, or is your agent not on the list above? Use the extension below (Claude Desktop only) or the three steps after it.

### Claude Desktop extension (one-click)

Download the `.mcpb` bundle for your platform, open it, and Claude Desktop will walk you through setup. Use either the extension or `setup`'s Claude Desktop entry, not both: with both, every tool appears twice.

| Platform | Download |
|---|---|
| macOS (Apple Silicon) | [audiobookshelf-mcp-macos-arm64.mcpb](https://github.com/sierikov/audiobookshelf-mcp/releases/latest/download/audiobookshelf-mcp-macos-arm64.mcpb) |
| macOS (Intel) | [audiobookshelf-mcp-macos-amd64.mcpb](https://github.com/sierikov/audiobookshelf-mcp/releases/latest/download/audiobookshelf-mcp-macos-amd64.mcpb) |
| Windows | [audiobookshelf-mcp-windows-amd64.mcpb](https://github.com/sierikov/audiobookshelf-mcp/releases/latest/download/audiobookshelf-mcp-windows-amd64.mcpb) |
| Linux x64 | [audiobookshelf-mcp-linux-amd64.mcpb](https://github.com/sierikov/audiobookshelf-mcp/releases/latest/download/audiobookshelf-mcp-linux-amd64.mcpb) |
| Linux arm64 | [audiobookshelf-mcp-linux-arm64.mcpb](https://github.com/sierikov/audiobookshelf-mcp/releases/latest/download/audiobookshelf-mcp-linux-arm64.mcpb) |

> The `.mcpb` format is currently only supported by Claude Desktop; other clients use `setup` or the steps below. If your organisation blocks extensions, use `setup` instead.

### 1. Get your API token

Audiobookshelf → Settings → Users → your user → **API Token**

### 2. Install the binary

**go install:**
```bash
go install github.com/sierikov/audiobookshelf-mcp/cmd/audiobookshelf-mcp@latest
```

**Download and install to `/usr/local/bin`:**

<details>
<summary>macOS (Apple Silicon)</summary>

```bash
curl -L https://github.com/sierikov/audiobookshelf-mcp/releases/latest/download/audiobookshelf-mcp-macos-arm64 \
  -o /usr/local/bin/audiobookshelf-mcp && chmod +x /usr/local/bin/audiobookshelf-mcp
```

</details>

<details>
<summary>macOS (Intel)</summary>

```bash
curl -L https://github.com/sierikov/audiobookshelf-mcp/releases/latest/download/audiobookshelf-mcp-macos-amd64 \
  -o /usr/local/bin/audiobookshelf-mcp && chmod +x /usr/local/bin/audiobookshelf-mcp
```

</details>

<details>
<summary>Linux (x64)</summary>

```bash
curl -L https://github.com/sierikov/audiobookshelf-mcp/releases/latest/download/audiobookshelf-mcp-linux-amd64 \
  -o /usr/local/bin/audiobookshelf-mcp && chmod +x /usr/local/bin/audiobookshelf-mcp
```

</details>

<details>
<summary>Linux (arm64)</summary>

```bash
curl -L https://github.com/sierikov/audiobookshelf-mcp/releases/latest/download/audiobookshelf-mcp-linux-arm64 \
  -o /usr/local/bin/audiobookshelf-mcp && chmod +x /usr/local/bin/audiobookshelf-mcp
```

</details>

<details>
<summary>Windows</summary>

Download [`audiobookshelf-mcp-windows-amd64.exe`](https://github.com/sierikov/audiobookshelf-mcp/releases/latest/download/audiobookshelf-mcp-windows-amd64.exe), rename to `audiobookshelf-mcp.exe`, and place it in a directory on your `%PATH%` (e.g. `C:\Program Files\audiobookshelf-mcp\`).

</details>

### 3. Configure your client

<details>
<summary>Claude Code</summary>

```bash
claude mcp add audiobookshelf \
  -e AUDIOBOOKSHELF_URL=https://abs.example.com \
  -e AUDIOBOOKSHELF_TOKEN=your-token \
  -- audiobookshelf-mcp
```

</details>

<details>
<summary>Antigravity CLI (formerly Gemini CLI)</summary>

```bash
antigravity --add-mcp '{"name":"audiobookshelf","command":"audiobookshelf-mcp","env":{"AUDIOBOOKSHELF_URL":"https://abs.example.com","AUDIOBOOKSHELF_TOKEN":"your-token"}}'
```

</details>

<details>
<summary>OpenCode</summary>

Add to `~/.config/opencode/config.json`:

```json
{
  "mcpServers": {
    "audiobookshelf": {
      "type": "stdio",
      "command": "audiobookshelf-mcp",
      "args": [],
      "env": {
        "AUDIOBOOKSHELF_URL": "https://abs.example.com",
        "AUDIOBOOKSHELF_TOKEN": "your-token"
      }
    }
  }
}
```

</details>

---

## Security

The default toolsets are **read-only**. Two opt-in toolsets write, and both need an API token whose user has the **Update** permission in Audiobookshelf. Neither needs admin.

- `chapters`: `set_chapters` and `rename_chapters` replace a book's chapters. They are annotated as destructive so MCP clients ask before running them, and return the previous chapters so a change can be undone when those old chapters were valid.
- `metadata`: `add_genres`, `remove_genres`, `add_tags` and `remove_tags` add and remove genres and tags, and `update_items` sets language and description. The remove tools and `update_items` are annotated as destructive. Every write can be undone: the list tools by calling the opposite tool with each book's `added` or `removed` values, `update_items` by passing the `previous` values it returns. Audiobookshelf can limit what a user sees by tag, so `add_tags` and `remove_tags` can change what a restricted user, such as a kids account, can see.

`all` includes every write tool; list toolsets explicitly to stay read-only. No tool deletes items, controls playback, or manages users. The API token comes from the settings file (readable only by you) or an environment variable, and is never logged or exposed.

## Releasing

A release is a signed tag. The version comes from the tag, so there is nothing to bump in files: `manifest.json` and `server.json` keep the placeholder version `0.0.0-dev`, and the release stamps the real one.

```bash
git tag -s vX.Y.Z -m vX.Y.Z && git push origin vX.Y.Z
```

The Release workflow then publishes, one job after another, the GitHub release (binaries and `.mcpb` bundles) with the Homebrew cask, the npm packages, and the MCP Registry entry. GitHub generates the release notes; edit them on the release afterwards if needed.

Actions → Release → Run workflow is always a dry run: it builds a snapshot, checks `server.json` with the MCP Registry, rehearses the npm publish, and publishes nothing.

If a job fails, fix the cause and use **Re-run failed jobs** on the run: the jobs that succeeded are kept, and the rest reuse their build, which is kept for 90 days.

A pre-release tag such as `v1.2.0-rc.1` becomes a GitHub pre-release and goes to npm under the `next` tag. It gets no Homebrew cask and no MCP Registry entry.

For the first release, npm trusted publishing cannot create a package, so the npm job fails until the packages exist. Publish the six packages by hand once (`@sierikov/audiobookshelf-mcp` and the five `@sierikov/audiobookshelf-mcp-<platform>` ones), set up trusted publishing for each of them on npmjs.com, then re-run the failed jobs.

## License

MIT
