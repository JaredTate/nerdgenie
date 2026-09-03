package loose

import (
	"encoding/json"
	"strings"
)

// textOf is the text of one field however the model wrote it: a string as it
// stands, a number or a true as it was written, a list of strings one to a line,
// and an object by the text inside it. The depth is what stops a list of lists
// of lists from being followed forever.
func textOf(raw json.RawMessage, depth int) (string, bool) {
	if depth <= 0 || isNothing(raw) {
		return "", false
	}
	trimmed := strings.TrimSpace(string(raw))
	switch {
	case strings.HasPrefix(trimmed, `"`):
		text := ""
		if err := json.Unmarshal(raw, &text); err != nil {
			return "", false
		}
		return text, true
	case strings.HasPrefix(trimmed, "["):
		return textOfList(raw, depth)
	case strings.HasPrefix(trimmed, "{"):
		return textOfObject(raw, depth)
	default:
		return trimmed, true
	}
}

// textOfList is the strings of a list, one to a line, which is how a model
// writes a piece of text it thinks of as lines.
func textOfList(raw json.RawMessage, depth int) (string, bool) {
	items := []json.RawMessage{}
	if err := json.Unmarshal(raw, &items); err != nil {
		return "", false
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		line, readable := textOf(item, depth-1)
		if !readable {
			return "", false
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return "", false
	}
	return strings.Join(lines, "\n"), true
}

// textOfObject is the text inside an object, which a model writes when it wraps
// a field it was asked for in a shape of its own.
func textOfObject(raw json.RawMessage, depth int) (string, bool) {
	inside := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &inside); err != nil {
		return "", false
	}
	for _, name := range []string{"text", "value", "content", "line"} {
		if held, there := inside[name]; there {
			return textOf(held, depth-1)
		}
	}
	return "", false
}
