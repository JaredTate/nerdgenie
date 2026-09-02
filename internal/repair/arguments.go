package repair

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// decodeArguments reads what the model wrote as the arguments of one call. The
// arguments must be a JSON object; a model that wrapped the object in a string,
// which is what one provider's own interface does, has it read once more; and a
// model that wrote a list or a number gets a problem naming the tool and the
// shape it expects. A call with no arguments at all, or with nothing written
// where the arguments go, is a call with an empty object, because a tool that
// needs nothing is still a tool.
func decodeArguments(raw []byte, spec contract.ToolSpec, specs []contract.ToolSpec) (json.RawMessage, string) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return json.RawMessage("{}"), ""
	}
	if object, ok := compactObject(trimmed); ok {
		return object, ""
	}
	var held string
	if json.Unmarshal(trimmed, &held) == nil {
		if object, ok := compactObject([]byte(strings.TrimSpace(held))); ok {
			return object, ""
		}
	}
	return nil, problemArguments(spec, specs)
}

// compactObject reads a JSON object and returns it with the whitespace taken
// out, so that what goes to the tool is what the model wrote and nothing else.
func compactObject(raw []byte) (json.RawMessage, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(trimmed, &fields) != nil {
		return nil, false
	}
	tightened := &bytes.Buffer{}
	if json.Compact(tightened, trimmed) != nil {
		return nil, false
	}
	return json.RawMessage(tightened.Bytes()), true
}
