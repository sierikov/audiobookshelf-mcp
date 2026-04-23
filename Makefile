BINARY    := audiobookshelf-mcp
MAIN      := ./cmd/audiobookshelf-mcp
GO        := go
GOFLAGS   :=

.PHONY: all build test lint fmt vet check clean

all: check build

build:
	$(GO) build $(GOFLAGS) -o $(BINARY) $(MAIN)

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
	rm -f $(BINARY)
