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
| `find_items` | List books with their genres, tags, language and a description preview, filtered | metadata |
| `search_book_metadata` | Look a book up in one metadata source (fantlab, google, audible, openlibrary, itunes) | metadata |
| `add_genres` | Add genres to books (writes) | metadata |
| `remove_genres` | Remove genres from books (writes) | metadata |
| `add_tags` | Add tags to books (writes) | metadata |
| `remove_tags` | Remove tags from books (writes) | metadata |
| `update_items` | Set the language and description of books (writes) | metadata |

Default toolsets: `libraries`, `items`, `playback`. The `browse`, `chapters` and `metadata` toolsets are opt-in via `AUDIOBOOKSHELF_TOOLSETS`.

### Fixing chapters

With the `chapters` toolset enabled, ask things like *"Fix the chapters for Katabasis"*. The assistant reads the book's current chapters, searches Audible for the edition whose runtime matches your files, fetches its chapters, replaces generic titles ("Chapter 1, 2, 3") with real ones, and shows you the list before saving. When Audible has no match, it can build the list from its own research or from timestamps you give it. Every save returns the previous chapters, so a change can be undone, as long as those old chapters were valid (old chapters that run past the end of the book, for example, cannot be saved back).

### Cleaning up genres, tags and descriptions

With the `metadata` toolset enabled, ask things like *"Make my genres consistent"* or *"Fix the descriptions of the Harry Hole books"*. The assistant surveys the library with `get_metadata_overview`, proposes a mapping from the old genres or tags to the new ones, and shows you the affected books. After you say yes, it removes the old value and adds the new one. For descriptions it looks up real ones with `search_book_metadata`, shows each change from old to new, and tells you when it wrote a description itself. Every write returns what it changed, so a change can be undone: the genre and tag tools list each book's `added` or `removed` values, and `update_items` returns the `previous` values.

---

## Install in Claude Desktop (one-click)

Download the `.mcpb` bundle for your platform, open it, and Claude Desktop will walk you through setup.

| Platform | Download |
|---|---|
| macOS (Apple Silicon) | [audiobookshelf-mcp-macos-arm64.mcpb](https://github.com/sierikov/audiobookshelf-mcp/releases/latest/download/audiobookshelf-mcp-macos-arm64.mcpb) |
| macOS (Intel) | [audiobookshelf-mcp-macos-amd64.mcpb](https://github.com/sierikov/audiobookshelf-mcp/releases/latest/download/audiobookshelf-mcp-macos-amd64.mcpb) |
| Windows | [audiobookshelf-mcp-windows-amd64.mcpb](https://github.com/sierikov/audiobookshelf-mcp/releases/latest/download/audiobookshelf-mcp-windows-amd64.mcpb) |
| Linux x64 | [audiobookshelf-mcp-linux-amd64.mcpb](https://github.com/sierikov/audiobookshelf-mcp/releases/latest/download/audiobookshelf-mcp-linux-amd64.mcpb) |
| Linux arm64 | [audiobookshelf-mcp-linux-arm64.mcpb](https://github.com/sierikov/audiobookshelf-mcp/releases/latest/download/audiobookshelf-mcp-linux-arm64.mcpb) |

> The `.mcpb` format is currently only supported by Claude Desktop. Other clients use the manual setup below.

---

## Manual setup (all other clients)

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

## Configuration

| Variable | Required | Default | Description |
|---|---|---|---|
| `AUDIOBOOKSHELF_URL` | Yes | — | Your Audiobookshelf server URL |
| `AUDIOBOOKSHELF_TOKEN` | Yes | — | API token from Settings → Users |
| `AUDIOBOOKSHELF_TOOLSETS` | No | `libraries,items,playback` | Comma-separated toolsets to enable: `libraries`, `items`, `playback`, `browse`, `chapters`, `metadata`. Use `all` to enable everything, including the `chapters` and `metadata` write tools. |

---

## Security

The default toolsets are **read-only**. Two opt-in toolsets write, and both need an API token whose user has the **Update** permission in Audiobookshelf. Neither needs admin.

- `chapters`: `set_chapters` and `rename_chapters` replace a book's chapters. They are annotated as destructive so MCP clients ask before running them, and return the previous chapters so a change can be undone when those old chapters were valid.
- `metadata`: `add_genres`, `remove_genres`, `add_tags` and `remove_tags` add and remove genres and tags, and `update_items` sets language and description. The remove tools and `update_items` are annotated as destructive. Every write can be undone: the list tools by calling the opposite tool with each book's `added` or `removed` values, `update_items` by passing the `previous` values it returns. Audiobookshelf can limit what a user sees by tag, so `add_tags` and `remove_tags` can change what a restricted user, such as a kids account, can see.

`all` includes every write tool; list toolsets explicitly to stay read-only. No tool deletes items, controls playback, or manages users. The API token is passed via environment variable, never logged or exposed.

## License

MIT
