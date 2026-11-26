package tools

import "github.com/modelcontextprotocol/go-sdk/mcp"

// Every tool sets all four annotations, so no client has to fall back on the
// spec's defaults (destructive and open-world when a hint is missing).

// readAnnotations marks a tool that only reads from the Audiobookshelf server.
// Reading again changes nothing, so it is idempotent.
func readAnnotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(false)}
}

// openWorldReadAnnotations marks a tool that only reads, but asks a service
// outside the Audiobookshelf server, such as Audible or FantLab.
func openWorldReadAnnotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(true)}
}

// additiveWriteAnnotations marks a tool that only adds to what is stored, and
// is idempotent because adding a value twice leaves the same list.
func additiveWriteAnnotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(false)}
}

// destructiveWriteAnnotations marks a tool that removes or replaces stored
// values, and is idempotent because repeating a call leaves the same result.
func destructiveWriteAnnotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{DestructiveHint: new(true), IdempotentHint: true, OpenWorldHint: new(false)}
}

// openWorldDestructiveWriteAnnotations marks a tool that replaces a stored
// value, and has Audiobookshelf fetch the new one from a web address outside
// it. It is idempotent because repeating a call leaves the same result.
func openWorldDestructiveWriteAnnotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{DestructiveHint: new(true), IdempotentHint: true, OpenWorldHint: new(true)}
}
