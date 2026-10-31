package abs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// FlexString is a string that also decodes from a JSON number or null.
// Metadata providers disagree on shape: FantLab sends publishedYear as a
// number, Audible as a string. A number keeps its literal text, so 1961
// becomes "1961" and null becomes "".
type FlexString string

// UnmarshalJSON implements [json.Unmarshaler].
func (f *FlexString) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	switch {
	case bytes.Equal(data, []byte("null")):
		*f = ""
	case len(data) > 0 && data[0] == '"':
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*f = FlexString(s)
	case len(data) > 0 && (data[0] == '-' || data[0] >= '0' && data[0] <= '9'):
		*f = FlexString(data)
	default:
		return fmt.Errorf("cannot decode %s as a string or number", data)
	}
	return nil
}

// FlexStrings is a list of strings that also decodes from a JSON string or
// null. Some providers send tags as one comma-separated string; it is split
// on commas, each part trimmed and empty parts dropped. null becomes nil.
type FlexStrings []string

// UnmarshalJSON implements [json.Unmarshaler].
func (f *FlexStrings) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	switch {
	case bytes.Equal(data, []byte("null")):
		*f = nil
	case len(data) > 0 && data[0] == '"':
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		var parts FlexStrings
		for part := range strings.SplitSeq(s, ",") {
			if part = strings.TrimSpace(part); part != "" {
				parts = append(parts, part)
			}
		}
		*f = parts
	case len(data) > 0 && data[0] == '[':
		var list []string
		if err := json.Unmarshal(data, &list); err != nil {
			return err
		}
		*f = list
	default:
		return fmt.Errorf("cannot decode %s as a list of strings or a string", data)
	}
	return nil
}
