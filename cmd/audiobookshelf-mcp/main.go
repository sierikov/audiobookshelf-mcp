// Package main handles the command-line interface for the Audiobookshelf MCP server.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
	"github.com/sierikov/audiobookshelf-mcp/internal/service"
	"github.com/sierikov/audiobookshelf-mcp/internal/tools"
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

	client := abs.NewClient(url, token)
	server := tools.NewServer(tools.Services{
		Libraries: service.NewLibraries(client),
		Items:     service.NewItems(client),
		Playback:  service.NewPlayback(client),
		Browse:    service.NewBrowse(client),
		Chapters:  service.NewChapters(client),
		Metadata:  service.NewMetadata(client),
	}, toolsets)

	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		stop()
		log.Fatal(err)
	}
	stop()
}
