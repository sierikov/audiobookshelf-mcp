package tools

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/sierikov/audiobookshelf-mcp/internal/service"
)

// limitSchema describes an optional page size: a whole number from 1 to
// [service.MaxPageLimit].
func limitSchema(description string) *jsonschema.Schema {
	return &jsonschema.Schema{Type: "integer", Description: description, Minimum: new(1.0), Maximum: new(float64(service.MaxPageLimit))}
}

// pageSchema describes an optional page number, starting at 0.
func pageSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "integer", Description: "Page number, starting at 0 (default 0)", Minimum: new(0.0)}
}

// The readers below don't check types: Register has already checked every
// call's arguments against the tool's input schema. A value of another type
// can only come from a direct call, as in a unit test, and reads as absent.

// RequiredParam returns the named argument.
func RequiredParam[T any](args map[string]any, name string) (T, error) {
	value, ok := args[name].(T)
	if !ok {
		return value, fmt.Errorf("missing required parameter: %s", name)
	}
	return value, nil
}

// OptionalParam returns the named argument, and whether it was given.
func OptionalParam[T any](args map[string]any, name string) (T, bool) {
	value, ok := args[name].(T)
	return value, ok
}

// OptionalIntParam returns the named number as an int, or defaultVal when it
// is absent. JSON numbers arrive as float64.
func OptionalIntParam(args map[string]any, name string, defaultVal int) int {
	if n, ok := args[name].(float64); ok {
		return int(n)
	}
	return defaultVal
}

// requiredStrings returns the named list of strings.
func requiredStrings(args map[string]any, name string) ([]string, error) {
	raw, err := RequiredParam[[]any](args, name)
	if err != nil {
		return nil, err
	}
	return stringsOf(raw), nil
}

// stringsOf returns the strings in raw, a list of arguments; an entry of
// another type reads as "".
func stringsOf(raw []any) []string {
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		s, _ := item.(string)
		out = append(out, s)
	}
	return out
}
