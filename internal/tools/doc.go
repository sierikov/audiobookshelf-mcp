// Package tools exposes Audiobookshelf as Model Context Protocol (MCP) tools,
// giving LLM assistants access to audiobook and podcast libraries: read-only
// by default, with an opt-in toolset that fixes audiobook chapters.
//
// This package is wiring only. Each tool declares its schema, parses its
// arguments, makes one call to the service layer through a small interface
// ([LibraryService], [ItemService], [PlaybackService], [BrowseService],
// [ChapterService]), and maps the result to its JSON output, formatting
// durations, progress and dates for people along the way. The logic lives in the service package and
// HTTP lives in the abs package; this package imports neither the client nor
// its types.
//
// # Toolsets
//
// Tools are organized into toolsets that can be enabled independently via the
// AUDIOBOOKSHELF_TOOLSETS environment variable:
//
//   - libraries: list libraries, search by title/author/narrator, paginate items
//   - items: detailed item info, currently in-progress items with progress tracking
//   - playback: listening stats, session history, per-item progress
//   - browse: series, authors, and collections (opt-in, disabled by default)
//   - chapters: read a book's chapters, find matching Audible editions and
//     their chapters, and replace or rename chapters (opt-in, disabled by
//     default; the only toolset that writes)
//
// The libraries, items, and playback toolsets are enabled by default. Pass an
// empty list to [NewServer] for the defaults, or "all" to enable everything.
//
// # Security
//
// The default toolsets are strictly read-only, and every read tool carries the
// ReadOnlyHint annotation. Only the opt-in chapters toolset writes: its
// set_chapters and rename_chapters tools replace a book's chapters, are
// annotated destructive so clients ask before running them, and return the
// previous chapters so a change can be undone when those old chapters were
// valid. They need an API token whose user has the Update permission, and
// "all" includes them. No tool deletes items, controls playback, or manages
// users.
package tools
