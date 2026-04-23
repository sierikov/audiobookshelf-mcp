// Package main handles the command-line interface for the Audiobookshelf MCP server.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/sierikov/audiobookshelf-mcp/internal/audiobookshelf"
)

func main() {
	url := os.Getenv("AUDIOBOOKSHELF_URL")
	token := os.Getenv("AUDIOBOOKSHELF_TOKEN")

	if url == "" || token == "" {
		log.Fatal("AUDIOBOOKSHELF_URL and AUDIOBOOKSHELF_TOKEN environment variables must be set")
	}

	var toolsets []string
	if ts := os.Getenv("AUDIOBOOKSHELF_TOOLSETS"); ts != "" {
		toolsets = strings.Split(ts, ",")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	cfg := audiobookshelf.Config{
		URL:      url,
		Token:    token,
		Toolsets: toolsets,
	}

	if err := audiobookshelf.Run(ctx, cfg); err != nil {
		stop()
		log.Fatal(err)
	}
	stop()
}
