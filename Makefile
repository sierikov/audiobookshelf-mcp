BINARY    := audiobookshelf-mcp
MAIN      := ./cmd/audiobookshelf-mcp
GO        := go
GOFLAGS   :=
MCPB_FILE := audiobookshelf-mcp.mcpb

.PHONY: all build test lint fmt vet check clean mcpb

all: check build

build:
	$(GO) build $(GOFLAGS) -o $(BINARY) $(MAIN)

mcpb: build
	@if [ ! -f icon.png ]; then echo "ERROR: icon.png not found — add a 512x512 PNG icon to the project root first"; exit 1; fi
	zip -j $(MCPB_FILE) manifest.json icon.png $(BINARY)
	@echo "Built $(MCPB_FILE) — drag it into Claude Desktop to install"

test:
	$(GO) test ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -w ./...
	goimports -w ./...

vet:
	$(GO) vet ./...

check: fmt vet lint test

clean:
	rm -f $(BINARY) $(MCPB_FILE)
