package testkit

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
)

// maxRequestBodyDepth bounds how deep the walk over a request body goes. Both
// wire protocols nest three or four levels, so twenty is far more than either
// needs and still stops a body written to make the walk run forever.
const maxRequestBodyDepth = 20

// providerRequestBody is the part of either wire protocol's request body that
// carries text the model reads. The Anthropic Messages API puts the system
// prompt in its own field and the OpenAI Chat Completions API puts it in the
// first message, so both fields are read and whichever is absent contributes
// nothing. Everything inside them is left unparsed, because the two protocols
// agree on the outer names and on almost nothing below them.
type providerRequestBody struct {
	// System is the Anthropic system prompt, a string or a list of blocks.
	System json.RawMessage `json:"system"`
	// Messages is the conversation, in either protocol's shape.
	Messages json.RawMessage `json:"messages"`
	// Tools is the tool specification list, in either protocol's shape.
	Tools json.RawMessage `json:"tools"`
}

// WholeRequestBodyText is everything the model would read in one request body on
// the wire, joined into one string: the system prompt, every message, every tool
// result, and the tool specifications. It is the wire-level twin of
// WholeRequestText, and it is what the fake provider server checks a step's
// expectations against, so that a harness which drops the user's correction is
// caught whichever protocol it speaks.
//
// A body that is not JSON reads as no text at all, which fails every expectation
// rather than passing it by accident.
func WholeRequestBodyText(body []byte) string {
	var request providerRequestBody
	if err := json.Unmarshal(body, &request); err != nil {
		return ""
	}
	pieces := []string{}
	for _, part := range []json.RawMessage{request.System, request.Messages, request.Tools} {
		pieces = append(pieces, textInRawJSON(part)...)
	}
	return strings.Join(pieces, "\n")
}

// textInRawJSON collects the text inside one unparsed piece of a request body.
func textInRawJSON(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	return textInValue(value, 0)
}

// textInValue collects every string a decoded piece of JSON holds as a value,
// leaving the field names out, because a field name is the protocol talking and
// only the values are what the model reads. Object fields are visited in sorted
// order so that the same body always reads the same way.
func textInValue(value any, depth int) []string {
	if depth > maxRequestBodyDepth {
		return nil
	}
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []any:
		found := []string{}
		for _, item := range typed {
			found = append(found, textInValue(item, depth+1)...)
		}
		return found
	case map[string]any:
		found := []string{}
		for _, key := range slices.Sorted(maps.Keys(typed)) {
			found = append(found, textInValue(typed[key], depth+1)...)
		}
		return found
	default:
		return nil
	}
}
