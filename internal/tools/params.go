package tools

import "fmt"

// RequiredParam extracts a required parameter from args.
func RequiredParam[T any](args map[string]any, name string) (value T, err error) {
	var zero T
	candidate, ok := args[name]
	if !ok {
		return zero, fmt.Errorf("missing required parameter: %s", name)
	}
	value, ok = candidate.(T)
	if !ok {
		return zero, fmt.Errorf("parameter %s has wrong type: expected %T, got %T", name, zero, candidate)
	}
	return value, nil
}

// OptionalParam extracts an optional parameter from args.
func OptionalParam[T any](args map[string]any, name string) (value T, ok bool, err error) {
	var zero T
	candidate, ok := args[name]
	if !ok {
		return zero, false, nil
	}
	value, ok = candidate.(T)
	if !ok {
		return zero, false, fmt.Errorf("parameter %s has wrong type: expected %T, got %T", name, zero, candidate)
	}
	return value, true, nil
}

// OptionalIntParam extracts an optional integer parameter, handling JSON number →  int conversion.
func OptionalIntParam(args map[string]any, name string, defaultVal int) (int, error) {
	v, ok := args[name]
	if !ok {
		return defaultVal, nil
	}
	switch n := v.(type) {
	case float64:
		return int(n), nil
	case int:
		return n, nil
	default:
		return 0, fmt.Errorf("parameter %s: expected number, got %T", name, v)
	}
}
