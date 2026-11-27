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

**go install:**

```bash
go install github.com/sierikov/audiobookshelf-mcp/cmd/audiobookshelf-mcp@latest
audiobookshelf-mcp setup
```

The Go bin directory (`$(go env GOPATH)/bin`, or `$GOBIN` if you set it) must be on your `PATH` for `audiobookshelf-mcp setup` to work.

`setup` asks for:

1. Your Audiobookshelf URL and API token (Settings → Users → your user → **API Token**). The token is hidden as you type. `setup` checks them and prints `Connected to … as …`.
2. Whether to turn on the tools that edit your library (chapters, genres, tags, descriptions, series, covers). The default is no. Yes also turns on `browse`, which finds the series and authors those tools work on.
3. If `covers` is on: a Gemini API key, to make new covers (paid, about $0.07 to $0.16 a cover; see [Made covers](#made-covers)). It is hidden as you type. `setup` checks it with Google. Press Enter to skip it, or to keep the saved one, and type `none` to remove the saved one.
4. Which of the agents it found to add the server to. Press Enter for all of them.

It then saves your settings and adds the server to those agents, after backing up each config file it edits. The agents' configs hold only the command to run, never your token or key. It looks for these agents:

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
| `--toolsets a,b` | Toolsets to use, as a comma-separated list. It replaces the defaults (`libraries,items,playback`): for example `libraries,items,playback,browse,chapters,metadata,covers`, or `all`. An empty value restores the defaults |
| `--agents id,id` | Agents to add the server to, by ID. `none` adds it to no agent |
| `--yes` | Never ask: use the flags, the saved settings and the defaults |

Run `setup` again to change the token, to add an agent you installed since, or to move agents that use npx to a new version.

## Updating

Upgrade the way you installed:

| Installed with | Upgrade |
|---|---|
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
  "toolsets": ["all"],
  "geminiApiKey": "your-gemini-key"
}
```

`geminiApiKey` is optional. Leave it out and the tools that make covers are not offered.

If `XDG_CONFIG_HOME` points somewhere other than `~/.config`, apps started outside your shell, such as Claude Desktop or Cursor, may not see it and will look in `~/.config` instead. `setup` warns you when that is the case.

Environment variables override the file. Re-run `setup` to change the token or the Gemini key.

| Variable | Required | Default | Description |
|---|---|---|---|
| `AUDIOBOOKSHELF_URL` | Unless saved by `setup` | — | Your Audiobookshelf server URL |
| `AUDIOBOOKSHELF_TOKEN` | Unless saved by `setup` | — | API token from Settings → Users |
| `AUDIOBOOKSHELF_TOOLSETS` | No | `libraries,items,playback` | Comma-separated toolsets to enable: `libraries`, `items`, `playback`, `browse`, `chapters`, `metadata`, `covers`. Use `all` to enable everything, including the `chapters`, `metadata` and `covers` write tools. |
| `AUDIOBOOKSHELF_MCP_GEMINI_API_KEY` | No | — | Google Gemini API key that pays for made covers. Without one, `generate_covers` and `clean_cover` are not offered. `GEMINI_API_KEY` is never read. |

## Tools

| Tool | Description | Toolset |
|---|---|---|
| `list_libraries` | List all libraries | libraries |
| `search_library` | Search by title, author, or narrator | libraries |
| `list_library_items` | List items with pagination | libraries |
| `get_item` | Get detailed info about an audiobook or podcast, including its tags and, for a book, its authors, narrators and series as lists | items |
| `get_items_in_progress` | Get currently listening items with progress | items |
| `get_listening_stats` | Total listening time and daily breakdown | playback |
| `get_listening_sessions` | Recent listening sessions | playback |
| `get_media_progress` | Progress for a specific item | playback |
| `list_series` | List all series in a library | browse |
| `get_series` | Get series details with its books in order, each with its number | browse |
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
| `update_items` | Set the language, subtitle, description, series, authors and narrators of books (writes) | metadata |
| `search_covers` | Find covers for a book: its current cover and up to 5 candidates, as small images with their links. Also shows cover links found on the web the same way | covers |
| `get_cover_prompt` | Write a prompt for a new cover, or for cleaning one, to paste into the Gemini app or ChatGPT, with links to open them | covers |
| `generate_covers` | Make up to 4 new covers with Google's Nano Banana image models. About $0.16 a cover on Nano Banana Pro, the default, or $0.07 on Nano Banana 2.1; needs a Gemini API key | covers |
| `clean_cover` | Clean a book's cover, such as removing a watermark or a store label, with the same models. About $0.16 on Nano Banana Pro or $0.07 on Nano Banana 2.1; needs a Gemini API key | covers |
| `set_cover` | Set a book's cover from a candidate's link, an image found on the web, or a made cover; final, the old cover file is deleted (writes) | covers |

Default toolsets: `libraries`, `items`, `playback`. The `browse`, `chapters`, `metadata` and `covers` toolsets are opt-in via `AUDIOBOOKSHELF_TOOLSETS`. `covers` finds, makes and sets book covers, and includes writes. With a Gemini API key it can make covers too, which costs money. `setup` asks whether to turn on the editing toolsets (`chapters`, `metadata`, `covers`; a yes adds `browse` too), and `--toolsets` sets the whole list.

### Fixing chapters

With the `chapters` toolset enabled, ask things like *"Fix the chapters for Katabasis"*. The assistant reads the book's current chapters, searches Audible for the edition whose runtime matches your files, fetches its chapters, replaces generic titles ("Chapter 1, 2, 3") with real ones, and shows you the list before saving. When Audible has no match, it can build the list from its own research or from timestamps you give it. Every save returns the previous chapters, so a change can be undone, as long as those old chapters were valid (old chapters that run past the end of the book, for example, cannot be saved back).

### Cleaning up genres, tags and descriptions

With the `metadata` toolset enabled, ask things like *"Make my genres consistent"* or *"Fix the descriptions of the Harry Hole books"*. The assistant surveys the library with `get_metadata_overview`, proposes a mapping from the old genres or tags to the new ones, and shows you the affected books. After you say yes, it adds the new value first and then removes the old one, so no book is left without it. For descriptions it looks up real ones with `search_book_metadata`, shows each change from old to new, and tells you when it wrote a description itself. `update_items` also sets a book's subtitle, and an empty string removes one, such as a junk "[роман]". Every write returns what it changed, so a change can be undone: the genre and tag tools list each book's `added` or `removed` values, and `update_items` returns the `previous` values.

### Series, authors and narrators

With the `metadata` toolset enabled, ask things like *"Put Holly in the Holly Gibney series as book 3"* or *"Set Will Patton as the narrator of these books"*. `update_items` sets a book's series (each with a name and, if the book has one, a number), authors and narrators. Each of these replaces the book's whole list, so the assistant sends every entry the book should keep, and an empty list removes them all; `get_item` returns a book's current lists in the same shape, so the assistant can send them back with its change. Names match regardless of case, so use the exact name of an existing series or author: a misspelt name creates a new one. Audiobookshelf creates a series or an author that doesn't exist yet, and deletes a series left with no books. It also deletes an author left with no books, unless the author has a photo, an ASIN or a description. The assistant shows you each change from old to new before it saves. To undo, pass the `previous` lists the write returns; a series Audiobookshelf deleted comes back as a new one. With the `browse` toolset on too, `get_series` lists a series' books in order, each with its number, so you can check the result. `setup`'s yes turns on `browse` along with `chapters`, `metadata` and `covers`; without `setup`, add it with `--toolsets` or `AUDIOBOOKSHELF_TOOLSETS`.

### Finding and setting covers

With the `covers` toolset enabled, ask things like *"Find a better cover for Holly"*. `search_covers` uses Audiobookshelf's own cover search. The provider defaults to `google`; the others are the ones `search_book_metadata` takes. It returns the book's current cover and up to 5 candidates as small image previews, each candidate also with its link. A preview fits inside 400 px wide and 800 px high: a larger image is scaled down to fit, and a small JPEG, PNG or GIF is shown as it is. A WebP, which is how FantLab serves its covers, is converted to a JPEG, because some apps don't show WebP. Apps that show images from tools, such as Claude Desktop, display them. If your app doesn't, the assistant gives you the links instead, and `set_cover` waits until you have looked at them. A candidate that can't be fetched, isn't an image, is over 2 MiB or has too many pixels (*too large to preview*) comes back with a short problem and no image, and the others still arrive. A book with no cover gets candidates only. So does one whose cover Audiobookshelf can't read: that is reported (`currentProblem`) and does not fail the search.

You pick one by its number, and `set_cover` sets it by its link. The cover sources can return the wrong book or edition. If none of the candidates is right, an assistant that can search the web looks for the cover there, for example on the publisher's or an audiobook store's page, and `set_cover` takes that image's link too. `search_covers` shows those links the same way, next to the current cover: it takes up to 5 of them (`urls`) in place of a source to search. **A cover change is final**: Audiobookshelf deletes the old cover file, so nothing can undo it. The assistant shows you the old and new covers side by side and asks first. `set_cover` needs the Audiobookshelf user's **Update** and **Upload** permissions; `setup` prints a note when `covers` or `all` is on and your user lacks either.

### Made covers

With the `covers` toolset on and a Gemini API key, ask things like *"Make a new cover for Holly, moody watercolour"* or *"Take the store label off this cover"*. `generate_covers` makes up to 4 new covers a call with Google's Nano Banana Pro image model (2048 px), or the cheaper Nano Banana 2.1 (1024 px) when you ask for it. For a series, the assistant makes one sample per style, you pick, and it makes the rest to match. `clean_cover` fixes a cover you already have, such as a watermark or a store label, and keeps the rest. Made and cleaned covers are always square, as audiobook covers are: a portrait cover comes back square, with its background extended. The assistant first shows what exists, from the cover sources and, where it can search the web, other editions and fan art, then offers at least three options, and makes or sets only the one you pick. Both show you the old cover next to the new ones. The title and the author are on a new cover as text unless you say otherwise. The prompt keeps to the setting and the opening, to keep spoilers off the cover.

- **It costs money.** A cover is about $0.16 with Nano Banana Pro and about $0.07 with Nano Banana 2.1. Google has no free tier for images, so the key needs billing turned on. The assistant says how many covers it will make and what that costs, and asks for your yes before every call. Each result says how many covers this server has made since it started and what they cost, as an estimate.
- **The key.** `setup` asks for it after the toolsets question, when `covers` or `all` is on. Or set `AUDIOBOOKSHELF_MCP_GEMINI_API_KEY`. `GEMINI_API_KEY` is never read, so a key you keep for other programs does not switch on paid tools here. Without a key, `generate_covers` and `clean_cover` are not offered; `get_cover_prompt` still is, and writes a prompt for you to paste into the Gemini app or ChatGPT, then upload the result in Audiobookshelf yourself.
- **Made covers live in memory.** For an hour, 24 at most (the oldest go first), and are lost when the server restarts. Nothing is written to disk. `set_cover` takes a made cover by its `imageId`, and the assistant sets one only after you have seen it next to the old one and said yes. That is final like any cover change.
- **A spending limit stops the call.** If Google answers that a spending limit, quota or credit is used up, the whole call stops at once and nothing is retried. The covers already made stay available.

Which apps show you the images: the Claude Desktop chat (inside the folded tool block) and the VS Code extension do. Claude Code in a terminal shows the path of a file that the app saves (this server writes nothing to disk), and Codex and ChatGPT show nothing. Every app checked passes the images to the model, and Hermes passes at most 4 images from one result. If your app shows you no images, the assistant should not set a cover you have not seen: it uses `get_cover_prompt` instead.

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

The default toolsets are **read-only**. Three opt-in toolsets write, and all need an API token whose user has the **Update** permission in Audiobookshelf; `set_cover` also needs the **Upload** permission. None needs admin.

- `chapters`: `set_chapters` and `rename_chapters` replace a book's chapters. They are annotated as destructive so MCP clients ask before running them, and return the previous chapters so a change can be undone when those old chapters were valid.
- `metadata`: `add_genres`, `remove_genres`, `add_tags` and `remove_tags` add and remove genres and tags, and `update_items` sets language, subtitle, description, series, authors and narrators. The remove tools and `update_items` are annotated as destructive. Every write can be undone, except what the text checks below refuse: the list tools by calling the opposite tool with each book's `added` or `removed` values, `update_items` by passing the `previous` values it returns. Audiobookshelf can limit what a user sees by tag, so `add_tags` and `remove_tags` can change what a restricted user, such as a kids account, can see.
- `covers`: `search_covers` only reads, but it downloads the candidate images itself, from addresses the cover source chose or the assistant found on the web. A download carries no Audiobookshelf token or other credentials, takes at most 15 seconds, follows at most 3 redirects (to `http` or `https` addresses only), accepts only a JPEG, PNG, WebP or GIF of at most 2 MiB, and is never written to disk. `set_cover` replaces a book's cover with the image at a web address. That is final, because Audiobookshelf deletes the old cover file, so it is annotated as destructive and tells the assistant to show you the old and new covers side by side and get your yes first. With a Gemini API key, `generate_covers` and `clean_cover` send data to Google. Google receives the book's title, or the other title text the assistant gives for a cover, its author and genres, the style and premise the assistant writes, and, for a series or a cleaning, a reference image or the current cover. A cleaning sends no title, author or genres, only the current cover and what to change. A cover costs about $0.16 with Nano Banana Pro and about $0.07 with Nano Banana 2.1. The tool text tells the assistant to say the count and cost and to get your yes before each call. The key goes only in the `x-goog-api-key` header to Google. It never goes in a URL, an error, a result or the output of `setup`, and never into an agent's config. Made images stay in memory.

`all` includes every write tool; list toolsets explicitly to stay read-only. No tool deletes items, controls playback, or manages users. The API token and the Gemini key come from the settings file (readable only by you) or an environment variable, and are never logged or exposed.

Every call is checked against the tool's input schema before it runs. An unknown argument, a wrong type, or a value out of range is refused with an error that names it, so a misspelled filter or an extra field is never silently ignored.

### Text from other sites

Your own Audiobookshelf is trusted: what the tools read from it is passed on as it is. Text from other sites is not. Four tools return it: `search_book_metadata` (Audible, FantLab, Google Books, iTunes, Open Library, through Audiobookshelf's search), `search_audible`, `get_audible_chapters` and `search_covers` (the same sources). Their results start with `"untrusted": true` and a `source`, and their text is cleaned first: descriptions keep only simple HTML (below), every other field becomes plain text, and invisible characters used to hide text are removed. For `search_covers` the text is each candidate's link: one that is not an `http` or `https` address, has whitespace or is over 2048 bytes is not downloaded, and comes back quoted and cut. The server instructions tell the model to treat that text as data, never as instructions. A caveat: much of a library's text first came from such sites, through Audiobookshelf's own matching, or from the audio files' tags. Once stored it counts as trusted like the rest.

What the write tools store is checked first, because that is where text from other sites can enter the library:

- Descriptions (`update_items`) keep only Audiobookshelf's own allowlist: `p`, `ol`, `ul`, `li`, `strong`, `em`, `del`, `br`, `b`, `i`, and `a` with an `http`, `https` or `mailto` link. Scripts, styles, comments, other attributes and invisible characters are removed before saving, whatever Audiobookshelf version runs. A description is at most 10,000 characters, and one with no text left after cleaning is refused.
- Genres and tags being added, the language, subtitle, series names, authors, narrators and chapter titles are plain text: a value with `<` or `>`, a control or invisible character, or more than 64 characters (200 for chapter titles and subtitles) is refused with an error that names it. A series number is plain text too, at most 16 characters.
- A language, subtitle, description, series, author, narrator or chapter title equal to what the book already has passes as it is, in a list too, so old values never block an edit. Removing accepts any value, so junk already in the library can still be found and removed.
- Undo restores the previous values, except what these checks refuse: a description comes back cleaned, a removed genre or tag the checks refuse can't be added back, and a series Audiobookshelf deleted comes back as a new one.

## Releasing

A release is a signed tag. The version comes from the tag, so there is nothing to bump in files: `manifest.json` and `server.json` keep the placeholder version `0.0.0-dev`, and the release stamps the real one.

```bash
git tag -s vX.Y.Z -m vX.Y.Z && git push origin vX.Y.Z
```

The Release workflow then publishes, one job after another, the GitHub release (binaries and `.mcpb` bundles), the npm packages, and the MCP Registry entry. GitHub generates the release notes; edit them on the release afterwards if needed. The Homebrew cask is switched off for now: to turn it on, set the `HOMEBREW_TAP_GITHUB_TOKEN` secret (write access to `sierikov/homebrew-tap`) and change `skip_upload` in `.goreleaser.yaml` to `auto`.

Actions → Release → Run workflow is always a dry run: it builds a snapshot, checks `server.json` with the MCP Registry, rehearses the npm publish, and publishes nothing.

If a job fails, fix the cause and use **Re-run failed jobs** on the run: the jobs that succeeded are kept, and the rest reuse their build, which is kept for 90 days.

A pre-release tag such as `v1.2.0-rc.1` becomes a GitHub pre-release and goes to npm under the `next` tag. It gets no MCP Registry entry, and no Homebrew cask once that is on.

For the first release, npm trusted publishing cannot create a package, so the npm job fails until the packages exist. Publish the six packages by hand once (`@sierikov/audiobookshelf-mcp` and the five `@sierikov/audiobookshelf-mcp-<platform>` ones), set up trusted publishing for each of them on npmjs.com, then re-run the failed jobs.

## License

MIT
