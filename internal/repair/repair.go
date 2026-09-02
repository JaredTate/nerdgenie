// The order of trust, the envelope shapes, and the rule that a bad call becomes
// something the model can fix rather than a crash are borrowed designs, written
// fresh in Go here. ZeroClaw's parser at
// ~/Code/zeroclaw/crates/zeroclaw-tool-call-parser/src/lib.rs is where the
// shapes and their order come from. OpenClaw's grammar at
// ~/Code/openclaw/packages/tool-call-repair/src/grammar.ts is where the cap on
// the length of a written name comes from. OpenCode's invalid-call path at
// ~/Code/opencode/packages/opencode/src/tool/invalid.ts is where the rule that a
// bad call becomes an error the model can fix comes from.

package repair

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/JaredTate/coeus/internal/contract"
)

// MaxSearchedBytes is how much of a reply is searched for tool calls. Anything
// past it is left alone and comes back as text, because a model that has not
// written its call in the first hundred and twenty-eight kilobytes is not going
// to write one at all.
const MaxSearchedBytes = 128 * 1024

// MaxFailedParses is how many parses may fail in a row before this package stops
// fighting the model. After this many, text that still looks like a tool call
// comes back as the answer with no problem, which is design section 3, rule 5's
// promise that the loop never gets stuck on something the model wrote.
const MaxFailedParses = 2

// maxCallsInOneReply is the most calls one reply may ask for. It is the same
// number as the window the guard compares a new call against, because a reply
// that asks for more calls than the guard can remember is a reply that has
// stopped making sense.
var maxCallsInOneReply = contract.DefaultConfig().Caps.IdenticalCallWindow

// Repair is one tool name the model wrote wrong and the real name it was read
// as, so that the loop can log the repair rather than hide it.
type Repair struct {
	// ModelWrote is the name exactly as the model wrote it.
	ModelWrote string
	// RealName is the real tool the name was read as.
	RealName string
}

// Result is what Find made of one model reply.
//
// Either Calls holds the tool calls to run or Problem holds a message for the
// model, and never both. Text is what is left of the reply once the tool-call
// envelopes are taken out, which is the model's answer when there are no calls.
type Result struct {
	// Calls are the tool calls found, in the order they appear in the reply.
	Calls []contract.ToolCall
	// Text is the reply with the tool-call envelopes taken out.
	Text string
	// Repairs names every tool name that was read as a different real name.
	Repairs []Repair
	// Note says what the package had to do to the reply to keep it inside its
	// bounds, and is empty when it did nothing.
	Note string
	// Problem is the message to hand back to the model when something looked
	// like a tool call and could not be read. It is empty when nothing went
	// wrong, and when it is set there are no calls.
	Problem string
}

// Find reads one reply and returns the tool calls in it.
//
// The specs are the tools that really exist, and they are what a written name is
// repaired against and what a problem message names. The count of failed parses
// is how many replies in a row have already failed to parse; the loop keeps that
// count and this package only applies the rule that comes with it.
func Find(reply contract.Reply, specs []contract.ToolSpec, failedParses int) Result {
	searched, tail := splitAtCap(reply.Text)
	visible, thinkingRunsOn := withoutThinking(searched)
	if thinkingRunsOn {
		// The thinking never ended inside the text that was searched, so the
		// rest of the reply is thinking too and none of it is the answer.
		tail = ""
	}
	answer := joinSegments([]string{visible}, tail)

	if len(reply.ToolCalls) > 0 {
		return finish(fromProvider(reply.ToolCalls), answer, answer, specs, failedParses)
	}
	found := scan(visible)
	leftover := joinSegments(textOutside(visible, found), tail)
	return finish(found, leftover, answer, specs, failedParses)
}

// finish turns the candidates into the result, and applies the rule that after
// two failed parses in a row the text is the answer. The answer is the whole
// reply with the thinking taken out, which is what comes back when this package
// gives up on the parse.
func finish(found []candidate, leftover string, answer string, specs []contract.ToolSpec, failedParses int) Result {
	built := build(found, specs)
	if built.Problem != "" {
		if failedParses >= MaxFailedParses {
			return Result{Text: answer}
		}
		return Result{Text: leftover, Problem: built.Problem}
	}
	built.Text = leftover
	return built
}

// build walks the candidates in order and turns each one into a call, stopping
// at the first one that cannot be read.
func build(found []candidate, specs []contract.ToolSpec) Result {
	result := Result{}
	for _, one := range found {
		if one.unreadable {
			return Result{Problem: problemUnreadable(specs)}
		}
		if len(result.Calls) >= maxCallsInOneReply {
			result.Note = fmt.Sprintf("Only the first %d tool calls in this reply were taken, because a reply may ask for at most %d.",
				maxCallsInOneReply, maxCallsInOneReply)
			break
		}
		call, repaired, problem := buildOne(one, specs, len(result.Calls)+1)
		if problem != "" {
			return Result{Problem: problem}
		}
		result.Calls = append(result.Calls, call)
		if repaired != nil {
			result.Repairs = append(result.Repairs, *repaired)
		}
	}
	return result
}

// buildOne turns one candidate into one tool call, repairing its name and
// reading its arguments.
func buildOne(one candidate, specs []contract.ToolSpec, position int) (contract.ToolCall, *Repair, string) {
	spec, problem := repairName(one.name, specs)
	if problem != "" {
		return contract.ToolCall{}, nil, problem
	}
	arguments, problem := decodeArguments(one.arguments, spec, specs)
	if problem != "" {
		return contract.ToolCall{}, nil, problem
	}
	var repaired *Repair
	if spec.Name != one.name {
		repaired = &Repair{ModelWrote: one.name, RealName: spec.Name}
	}
	return contract.ToolCall{ID: callID(one.id, position), Name: spec.Name, Input: arguments}, repaired, ""
}

// callID keeps the provider's identifier when there is one and numbers the calls
// found in text when there is not, so that a tool result can always name the
// call it answers.
func callID(given string, position int) string {
	if given != "" {
		return given
	}
	return fmt.Sprintf("call%d", position)
}

// fromProvider turns the calls the provider already parsed into candidates, so
// that they go through the same name check and the same argument check as the
// calls found in text.
func fromProvider(calls []contract.ToolCall) []candidate {
	found := make([]candidate, 0, len(calls))
	for _, call := range calls {
		found = append(found, candidate{id: call.ID, name: call.Name, arguments: call.Input})
	}
	return found
}

// splitAtCap cuts the reply at the searching cap, on a character boundary, and
// returns the part that is searched and the tail that is not.
func splitAtCap(text string) (string, string) {
	if len(text) <= MaxSearchedBytes {
		return text, ""
	}
	cut := MaxSearchedBytes
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut], text[cut:]
}

// textOutside returns the pieces of the text that no candidate sits on, in
// order.
func textOutside(text string, found []candidate) []string {
	segments := []string{}
	at := 0
	for _, one := range found {
		if one.start < at || one.end > len(text) || one.start > one.end {
			continue
		}
		segments = append(segments, text[at:one.start])
		at = one.end
	}
	return append(segments, text[at:])
}

// joinSegments puts the leftover pieces back together as the model's answer,
// dropping the whitespace the envelopes left behind.
func joinSegments(segments []string, tail string) string {
	kept := []string{}
	for _, segment := range append(segments, tail) {
		trimmed := strings.TrimSpace(segment)
		if trimmed != "" {
			kept = append(kept, trimmed)
		}
	}
	return strings.Join(kept, "\n")
}
