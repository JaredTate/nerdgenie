// Taking a reasoning model's thinking out before anything else reads the reply
// is ZeroClaw's design. Its parser at
// ~/Code/zeroclaw/crates/zeroclaw-tool-call-parser/src/lib.rs strips the tags
// before it looks for a single call, and its channel orchestrator at
// ~/Code/zeroclaw/crates/zeroclaw-channels/src/orchestrator/mod.rs drops the
// tail of a block that never closes rather than leak half a thought. Both rules
// are written fresh here.

package repair

import (
	"regexp"
	"strings"
)

// The tags a reasoning model puts around the part of its reply it is saying to
// itself. They are matched whatever case the model wrote them in, and they are
// compiled once because they are looked for on every reply.
var (
	thinkOpenTag  = regexp.MustCompile(`(?i)<think>`)
	thinkCloseTag = regexp.MustCompile(`(?i)</think>`)
)

// withoutThinking returns the text with every think block taken out, and says
// whether a block was still open when the text ran out.
//
// A model that reasons aloud writes tool calls to itself that it has not decided
// to make, so nothing between the tags is ever read as a call, and nothing
// between them is ever handed back as the answer. A block that never closes
// swallows the rest of the text, because half a thought is still a thought.
//
// The loop is bounded by the length of the text: every turn of it moves past at
// least one opening tag, so it can run no more times than the text has room for.
func withoutThinking(text string) (string, bool) {
	kept := &strings.Builder{}
	at := 0
	for at < len(text) {
		opening := thinkOpenTag.FindStringIndex(text[at:])
		if opening == nil {
			break
		}
		kept.WriteString(text[at : at+opening[0]])
		after := at + opening[1]
		closing := thinkCloseTag.FindStringIndex(text[after:])
		if closing == nil {
			return kept.String(), true
		}
		at = after + closing[1]
	}
	kept.WriteString(text[at:])
	return kept.String(), false
}
