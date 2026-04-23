FROM golang:1.26.1 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /audiobookshelf-mcp ./cmd/audiobookshelf-mcp

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /audiobookshelf-mcp /usr/local/bin/
ENTRYPOINT ["audiobookshelf-mcp"]
