// Package audiobookshelf implements a Model Context Protocol (MCP) server
// for the Audiobookshelf API, giving LLM assistants read-only access to
// audiobook and podcast libraries.
//
// # Overview
//
// The server exposes Audiobookshelf data as MCP tools, organized into toolsets.
// Each toolset groups related tools and can be enabled or disabled independently
// via the `AUDIOBOOKSHELF_TOOLSETS` environment variable.
//
// # Toolsets
//
// Four toolsets are available:
//
//   - libraries: list libraries, search by title/author/narrator, paginate items
//   - items: detailed item info, currently in-progress items with progress tracking
//   - playback: listening stats, session history, per-item progress
//   - browse: series, authors, and collections (opt-in, disabled by default)
//
// The libraries, items, and playback toolsets are enabled by default.
//
// # Usage
//
// Create a [Config] and call [Run] to start the server on stdio transport:
//
//	cfg := audiobookshelf.Config{
//	    URL:      "https://abs.example.com",
//	    Token:    "your-api-token",
//	    Toolsets: []string{"libraries", "items", "playback", "browse"},
//	}
//	if err := audiobookshelf.Run(ctx, cfg); err != nil {
//	    log.Fatal(err)
//	}
//
// Pass an empty Toolsets slice to use the defaults. Pass []string{"all"} to
// enable every toolset.
//
// # Transport
//
// The server communicates exclusively over stdio using the MCP `stdio` transport.
// There is no HTTP listener LLM clients connect by spawning the process and
// piping stdin/stdout.
//
// # Security
//
// All tools are strictly read-only. No writes, playback control, or user
// management operations are exposed. Every tool definition includes the
// ReadOnlyHint annotation so clients can communicate this constraint to users.
package audiobookshelf
