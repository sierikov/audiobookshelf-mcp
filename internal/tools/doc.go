// Package tools exposes Audiobookshelf as Model Context Protocol (MCP) tools,
// giving LLM assistants access to audiobook and podcast libraries: read-only
// by default, with three opt-in toolsets that write. One fixes audiobook
// chapters; one cleans up genres, tags, language, descriptions, series,
// authors and narrators; one finds and sets book covers.
//
// This package is wiring only. Each tool declares its schema, parses its
// arguments, makes one call to the service layer through a small interface
// ([LibraryService], [ItemService], [PlaybackService], [BrowseService],
// [ChapterService], [MetadataService], [CoversService]), and maps the result
// to its JSON output, formatting durations, progress and dates for people
// along the way.
// The logic lives in the service package and HTTP lives in the abs package;
// this package imports neither the client nor its types.
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
//     default; writes)
//   - metadata: survey genres, tags, language and descriptions across a
//     library, find books by them, look a book up in a metadata source, and
//     add or remove genres and tags or set language, description, series,
//     authors and narrators (opt-in, disabled by default; writes)
//   - covers: find cover images for a book on a metadata source, shown as
//     images next to the current cover, and set a book's cover from one of
//     them (opt-in, disabled by default; writes)
//
// The libraries, items, and playback toolsets are enabled by default. Leave
// [Options].Toolsets empty for the defaults, or pass "all" to enable everything.
//
// # Security
//
// The default toolsets are strictly read-only, and every read tool carries the
// ReadOnlyHint annotation. Three opt-in toolsets write. The chapters and
// metadata toolsets need an API token whose user has the Update permission,
// and the covers toolset one whose user has both the Update and the Upload
// permission, because Audiobookshelf asks for both to set a cover; none needs
// admin.
//
// The chapters toolset's set_chapters and rename_chapters tools replace a
// book's chapters, are annotated destructive so clients ask before running
// them, and return the previous chapters so a change can be undone when those
// old chapters were valid.
//
// The metadata toolset adds and removes genres and tags, and sets language,
// description, series, authors and narrators. Its remove tools and
// update_items are annotated destructive.
// Every write can be undone, except what the text checks below refuse: the
// list tools by calling the opposite tool with each book's added or removed
// values, update_items by passing the previous values it returns. Audiobookshelf can limit what a user sees by tag, so
// add_tags and remove_tags can change what a restricted user, such as a kids
// account, can see.
//
// The covers toolset's set_cover replaces a book's cover with the image at a
// web address, and that is final: Audiobookshelf deletes the old cover file, so
// the tool is annotated destructive and its description tells the model to
// show the user the old and new covers side by side and get a yes first.
// The address can be a candidate's or, when the cover sources return the wrong
// book, that of an image the model found on the web itself, which
// search_covers shows next to the current cover as it shows candidates.
// search_covers shows the covers as images, which it downloads itself with a
// client that carries no Audiobookshelf token or other credentials: each
// download is limited in size and time, follows only http and https
// redirects, and is never written to disk. Both tools are annotated
// open-world, because they reach web addresses a provider or the model chose.
//
// "all" includes every write tool. No tool deletes items, controls playback,
// or manages users.
//
// The user's Audiobookshelf is trusted; text from other sites is not. The
// results of search_book_metadata, search_audible and get_audible_chapters
// start with "untrusted": true and their source, and their text is cleaned
// in the service layer. So does search_covers, whose candidates are addresses
// the provider or the model chose: one that is not an http or https address, has
// whitespace or is over 2048 bytes is not downloaded, and comes back quoted
// and cut; what the downloaded images show is third-party content too. On the
// way in, descriptions are reduced to Audiobookshelf's own HTML allowlist, and
// genres, tags, language, series, author and narrator names, series numbers
// and chapter titles must be plain text within a length limit. A value equal
// to the stored one passes as it is, and removing accepts any value; undo
// restores previous values except what those checks refuse.
package tools
