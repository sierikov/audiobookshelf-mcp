package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolsetID identifies a group of related tools.
type ToolsetID string

const (
	toolsetLibraries ToolsetID = "libraries"
	toolsetItems     ToolsetID = "items"
	toolsetPlayback  ToolsetID = "playback"
	toolsetBrowse    ToolsetID = "browse"
	toolsetChapters  ToolsetID = "chapters"
	toolsetMetadata  ToolsetID = "metadata"
)

// ToolsetMeta describes a toolset.
type ToolsetMeta struct {
	ID          ToolsetID
	Description string
	Default     bool
}

var (
	metaLibraries = ToolsetMeta{toolsetLibraries, "Library listing and search", true}
	metaItems     = ToolsetMeta{toolsetItems, "Item details and progress tracking", true}
	metaPlayback  = ToolsetMeta{toolsetPlayback, "Listening stats and sessions", true}
	metaBrowse    = ToolsetMeta{toolsetBrowse, "Browse series, authors, collections", false}
	metaChapters  = ToolsetMeta{toolsetChapters, "Find, fix and save audiobook chapters (includes writes)", false}
	metaMetadata  = ToolsetMeta{toolsetMetadata, "Survey and fix genres, tags, language and descriptions (includes writes)", false}
)

// AllToolsetMetas returns all defined toolset metadata.
func AllToolsetMetas() []ToolsetMeta {
	return []ToolsetMeta{metaLibraries, metaItems, metaPlayback, metaBrowse, metaChapters, metaMetadata}
}

// DefaultToolsets returns the set of toolsets enabled by default.
func DefaultToolsets() map[ToolsetID]bool {
	m := make(map[ToolsetID]bool)
	for _, ts := range AllToolsetMetas() {
		if ts.Default {
			m[ts.ID] = true
		}
	}
	return m
}

// ToolHandlerFunc is the handler signature for all tools.
// Arguments are unmarshalled as map[string]any for explicit schema control.
type ToolHandlerFunc func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, error)

// ServerTool pairs a tool definition with its toolset and a registration function.
type ServerTool struct {
	Tool    mcp.Tool
	Toolset ToolsetMeta
	Handler ToolHandlerFunc
}

// inputSchema prepares a tool's input schema for checking calls. It works on
// a copy in which every object refuses properties it doesn't declare, so a
// misspelled or invented argument is an error rather than silently ignored.
func inputSchema(t *mcp.Tool) (*jsonschema.Resolved, error) {
	schema, ok := t.InputSchema.(*jsonschema.Schema)
	if !ok {
		return nil, fmt.Errorf("input schema is %T, want *jsonschema.Schema", t.InputSchema)
	}
	closed := schema.CloneSchemas()
	closeObjects(closed)
	return closed.Resolve(nil)
}

// closeObjects sets additionalProperties to false on every object schema in
// s, through properties and array items, unless the schema sets it already.
// Each object gets its own false schema: Resolve refuses a schema that
// appears twice.
func closeObjects(s *jsonschema.Schema) {
	if s == nil {
		return
	}
	if s.Type == "object" && s.AdditionalProperties == nil {
		s.AdditionalProperties = &jsonschema.Schema{Not: &jsonschema.Schema{}}
	}
	for _, p := range s.Properties {
		closeObjects(p)
	}
	closeObjects(s.Items)
}

// Register adds this tool to the MCP server using the non-generic s.AddTool
// method. It checks every call's arguments against the tool's input schema,
// closed to undeclared properties, and that closed schema is the one clients
// see. A call that fails gets a tool error and never reaches the handler. A
// schema that doesn't resolve is a programming error, so Register panics.
func (st *ServerTool) Register(s *mcp.Server) {
	tool := st.Tool // shallow copy to avoid mutation
	handler := st.Handler
	schema, err := inputSchema(&tool)
	if err != nil {
		panic(fmt.Sprintf("tool %s: %v", tool.Name, err))
	}
	tool.InputSchema = schema.Schema()
	s.AddTool(&tool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := parseArguments(req.Params.Arguments)
		if err == nil {
			err = schema.Validate(args)
		}
		if err != nil {
			return errResult("invalid arguments for " + tool.Name + ": " + strings.TrimPrefix(err.Error(), "validating root: "))
		}
		return handler(ctx, req, args)
	})
}

// parseArguments decodes a call's arguments. Absent or null arguments are an
// empty object, so the schema's required fields still apply.
func parseArguments(raw json.RawMessage) (map[string]any, error) {
	var args map[string]any
	if len(raw) > 0 && json.Unmarshal(raw, &args) != nil {
		return nil, errors.New("arguments must be an object")
	}
	if args == nil {
		args = make(map[string]any)
	}
	return args, nil
}
