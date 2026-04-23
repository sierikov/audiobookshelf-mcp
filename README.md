# Audiobookshelf MCP Server

[![Go](https://github.com/sierikov/audiobookshelf-mcp/actions/workflows/go.yml/badge.svg)](https://github.com/sierikov/audiobookshelf-mcp/actions/workflows/go.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/sierikov/audiobookshelf-mcp)](https://goreportcard.com/report/github.com/sierikov/audiobookshelf-mcp)

A Model Context Protocol (MCP) server for [Audiobookshelf](https://www.audiobookshelf.org/) — gives AI assistants
read-only access to your audiobook and podcast library.

## What can it do?

Ask your AI assistant things like:

- "What am I currently listening to?"
- "Search for Dune in my library"
- "What are my listening stats?"
- "What did I listen to recently?"
- "How far am I in Katabasis?"
- "What libraries do I have?"

## Tools

| Tool                     | Description                                     | Toolset   |
|--------------------------|-------------------------------------------------|-----------|
| `list_libraries`         | List all libraries                              | libraries |
| `search_library`         | Search by title, author, or narrator            | libraries |
| `list_library_items`     | List items with pagination                      | libraries |
| `get_item`               | Get detailed info about an audiobook or podcast | items     |
| `get_items_in_progress`  | Get currently listening items with progress     | items     |
| `get_listening_stats`    | Total listening time and daily breakdown        | playback  |
| `get_listening_sessions` | Recent listening sessions                       | playback  |
| `get_media_progress`     | Progress for a specific item                    | playback  |
| `list_series`            | List all series in a library                    | browse    |
| `get_series`             | Get series details with books                   | browse    |
| `get_author`             | Get author details with books                   | browse    |
| `list_collections`       | List collections in a library                   | browse    |

**Default toolsets:** `libraries`, `items`, `playback`. The `browse` toolset is opt-in.

## Setup

### 1. Get your API token

Audiobookshelf web UI &rarr; Settings &rarr; Users &rarr; your user &rarr; **API Token**

### 2. Install

**From source:**

```bash
go install github.com/sierikov/audiobookshelf-mcp/cmd/audiobookshelf-mcp@latest
```

**From release binary:**

Download from [Releases](https://github.com/sierikov/audiobookshelf-mcp/releases) and place in your `$PATH`.

<details>
<summary>Linux (amd64)</summary>

```bash
curl -L https://github.com/sierikov/audiobookshelf-mcp/releases/latest/download/audiobookshelf-mcp-linux-amd64 -o audiobookshelf-mcp
chmod +x audiobookshelf-mcp
```

</details>

<details>
<summary>Linux (arm64)</summary>

```bash
curl -L https://github.com/sierikov/audiobookshelf-mcp/releases/latest/download/audiobookshelf-mcp-linux-arm64 -o audiobookshelf-mcp
chmod +x audiobookshelf-mcp
```

</details>

<details>
<summary>macOS (Apple Silicon / arm64)</summary>

```bash
curl -L https://github.com/sierikov/audiobookshelf-mcp/releases/latest/download/audiobookshelf-mcp-macos-arm64 -o audiobookshelf-mcp
chmod +x audiobookshelf-mcp
```

</details>

<details>
<summary>macOS (Intel / amd64)</summary>

```bash
curl -L https://github.com/sierikov/audiobookshelf-mcp/releases/latest/download/audiobookshelf-mcp-macos-amd64 -o audiobookshelf-mcp
chmod +x audiobookshelf-mcp
```

</details>

<details>
<summary>Windows (amd64)</summary>

Download [`audiobookshelf-mcp-windows-amd64.exe`](https://github.com/sierikov/audiobookshelf-mcp/releases/latest/download/audiobookshelf-mcp-windows-amd64.exe), rename it to `audiobookshelf-mcp.exe`, and add it to a directory in your `%PATH%`.

</details>

### 3. Configure your AI client

<details>
<summary>Claude Desktop</summary>

Edit `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS) or
`%APPDATA%\Claude\claude_desktop_config.json` (Windows):

```json
{
  "mcpServers": {
    "audiobookshelf": {
      "command": "audiobookshelf-mcp",
      "env": {
        "AUDIOBOOKSHELF_URL": "https://abs.example.com",
        "AUDIOBOOKSHELF_TOKEN": "your-api-token"
      }
    }
  }
}
```

</details>

<details>
<summary>Claude Code</summary>

```bash
claude mcp add audiobookshelf -- env AUDIOBOOKSHELF_URL=https://abs.example.com AUDIOBOOKSHELF_TOKEN=your-api-token audiobookshelf-mcp
```

Or add to `.mcp.json` in your project:

```json
{
  "mcpServers": {
    "audiobookshelf": {
      "command": "audiobookshelf-mcp",
      "env": {
        "AUDIOBOOKSHELF_URL": "https://abs.example.com",
        "AUDIOBOOKSHELF_TOKEN": "your-api-token"
      }
    }
  }
}
```

</details>

<details>
<summary>VS Code / Cursor / Windsurf</summary>

Add to your settings JSON (`mcp` section):

```json
{
  "mcp": {
    "servers": {
      "audiobookshelf": {
        "command": "audiobookshelf-mcp",
        "env": {
          "AUDIOBOOKSHELF_URL": "https://abs.example.com",
          "AUDIOBOOKSHELF_TOKEN": "your-api-token"
        }
      }
    }
  }
}
```

</details>

<details>
<summary>OpenCode</summary>

Add to your OpenCode config:

```json
{
  "mcp": {
    "audiobookshelf": {
      "type": "local",
      "command": "audiobookshelf-mcp",
      "env": {
        "AUDIOBOOKSHELF_URL": "https://abs.example.com",
        "AUDIOBOOKSHELF_TOKEN": "your-api-token"
      }
    }
  }
}
```

</details>



## Configuration

| Environment Variable      | Required | Description                                                              |
|---------------------------|----------|--------------------------------------------------------------------------|
| `AUDIOBOOKSHELF_URL`      | Yes      | Your Audiobookshelf server URL                                           |
| `AUDIOBOOKSHELF_TOKEN`    | Yes      | API token (Bearer token)                                                 |
| `AUDIOBOOKSHELF_TOOLSETS` | No       | Comma-separated toolsets to enable (default: `libraries,items,playback`) |

To enable all toolsets including browse:

```
AUDIOBOOKSHELF_TOOLSETS=libraries,items,playback,browse
```

## Build from source

```bash
git clone https://github.com/sierikov/audiobookshelf-mcp.git
cd audiobookshelf-mcp
go build -o audiobookshelf-mcp ./cmd/audiobookshelf-mcp
```

Cross-compile:

```bash
GOOS=linux   GOARCH=amd64 go build -o audiobookshelf-mcp-linux-amd64       ./cmd/audiobookshelf-mcp
GOOS=linux   GOARCH=arm64 go build -o audiobookshelf-mcp-linux-arm64       ./cmd/audiobookshelf-mcp
GOOS=darwin  GOARCH=amd64 go build -o audiobookshelf-mcp-macos-amd64       ./cmd/audiobookshelf-mcp
GOOS=darwin  GOARCH=arm64 go build -o audiobookshelf-mcp-macos-arm64       ./cmd/audiobookshelf-mcp
GOOS=windows GOARCH=amd64 go build -o audiobookshelf-mcp-windows-amd64.exe ./cmd/audiobookshelf-mcp
```

## Security

- All tools are **read-only** — no writes, no playback control, no user management
- The API token is passed via environment variable, never logged or exposed
- All tool definitions include `readOnlyHint: true` annotation

## License

MIT
