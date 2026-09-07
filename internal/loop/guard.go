// The detector here is ZeroClaw's, from
// ~/Code/zeroclaw/crates/zeroclaw-runtime/src/agent/loop_detector.rs, where a
// window of recent calls is kept and the rule is a run of consecutive identical
// ones rather than any repeat at all, and OpenCode's, from
// ~/Code/opencode/packages/opencode/src/session/processor.ts, where the last
// few parts must all be the same tool with the same input before anything is
// refused. The Go here is written fresh.

package loop

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// The numbers the identical-call detector works to.
const (
	// IdenticalCallsAllowed is how many identical calls in a row actually run.
	// Design section 3, rule 4 says the same call is not run twice; a run of
	// two is allowed here because reading the same page twice in a row is how
	// a browser agent waits for a page to settle, which the forty-step fixture
	// does at rounds twenty-nine and thirty. The third is refused and the
	// fourth ends the turn.
	IdenticalCallsAllowed = 2
	// RewindsAllowed is how many times a task's conversation is cleared, with
	// the record left standing, before a run of the same call ends the task.
	// On the live game build the model read one file four times running
	// because a tool had told it a click worked when the page said it had
	// not; the third refusal ended the turn, and a task whose turn ends
	// from the terminal is a stopped task. A small model loops because its
	// conversation is full of the loop. Clearing the messages and leaving the
	// record is the cheapest way to hand it a fresh start. Each stall is
	// written into the record as a failure, so the memory of it survives the
	// clearing; a model that stalls a fourth time is not going to stop.
	RewindsAllowed = 3
	// SameCallHardCap is the second rule, beside the first: how many times the
	// same call with the same arguments may run in a row whatever it answered.
	// The first human trial ran one shell command thirteen times in a row, and
	// the first rule never fired, because every answer carried a new process id.
	// The seventh call is refused, and the ninth ends the turn.
	SameCallHardCap = 6
)

// HarnessStopLineWall is the second line the harness adds to every stop list:
// the first is the budget running out, and this is a wall the browser hit.
const HarnessStopLineWall = "the harness's own line: a browser page showed a login form, a two-factor prompt, or a captcha"

// wallWords are what a browser result says when it has hit one of the three
// walls that stop the agent and hand the window to the user.
var wallWords = []string{"login page", "log in page", "sign in page", "captcha", "two-factor", "two factor"}

// pastCall is one call the detector remembers: what was asked for, and what
// came back of it once it had run.
type pastCall struct {
	// mark is the call's fingerprint: its name and its arguments.
	mark string
	// result is the fingerprint of what the call came back with, and is empty
	// while the call has not run or never ran at all.
	result string
}

// TheRewindLine is the message the model reads after the rounds since its last
// progress are cut, in the harness's own words: what came before them and the
// record are what stand.
const TheRewindLine = "You asked for the same thing over and over, so the rounds since your last progress were cut from the conversation; what came before them and the record are what stand. Do something different from the last few rounds: check the thing you kept re-reading another way, write what you find into the record as a failure with its cause, and go on from there. The same call again ends the task."

// rewindIfDue acts on the stall the round earned, after the round's results
// are remembered. A rethink comes first (rethink.go): one call with the tools
// off whose answer goes into the record and in front of the model on a fresh
// window. When the model gives no answer, the cut stands instead: the stall
// goes into the record as a failure naming the call, every message since the
// last round that made progress goes and everything before it stays byte for
// byte, the run of calls the detector counts starts again, and the newest
// results in full and the rewind line are appended. A film editor with a scene
// that does not work cuts the bad stretch and keeps the reel on either side:
// the rounds that were working keep their place, and the daemon's cache of
// them keeps its value. The record and its results are untouched either way,
// because they live outside the messages, and that is the point: what was
// tried is not forgotten, only the going round in circles.
func (running *run) rewindIfDue(ctx context.Context) {
	if !running.rewindDue {
		return
	}
	running.rewindDue = false
	running.hadFailure = true
	running.roundsSinceProgress = 0
	if running.rethinkIfAnswered(ctx) {
		return
	}
	_ = running.keeper.Apply(ctx, record.Update{Failure: &record.NewFailure{
		Text:  running.stallText,
		Cause: "nothing the last rounds returned changed what was asked next",
	}})
	if running.keepThrough > len(running.messages) {
		running.keepThrough = len(running.messages)
	}
	running.messages = running.messages[:running.keepThrough]
	running.recentCalls = nil
	running.rememberTheOrientation(ctx, true)
	running.remember(contract.Message{Role: contract.RoleUser, Text: TheRewindLine})
	running.keepThrough = len(running.messages)
}

// detectorRefuses says whether this call is one the model has already made over
// and over. It returns the words to send back when the call is refused, and
// true when the turn has to end because the model will not stop asking. Two
// rules are read: a run of the same call whose answers also stayed the same, and
// the hard cap on a run of the same call whatever it answered.
func (running *run) detectorRefuses(call contract.ToolCall) (string, bool) {
	mark := fingerprintOf(call)
	streak := running.sameResultStreak(mark)
	made := running.sameCallRun(mark)
	running.rememberCall(mark)
	if streak > IdenticalCallsAllowed || made > SameCallHardCap+1 {
		return fmt.Sprintf("You have asked for %s with the same arguments %d times in a row, so the turn ends here.",
			call.Name, made+1), true
	}
	if streak == IdenticalCallsAllowed {
		return fmt.Sprintf("You have already called %s with these exact arguments %d times, and it was not run again. Do something different, or answer the user.",
			call.Name, streak), false
	}
	if made >= SameCallHardCap {
		return fmt.Sprintf("You have already called %s with these exact arguments %d times, and it was not run again. "+
			"The answers differed, but they did not change what you did next. Do something different, or answer the user: "+
			"if you are waiting for something to finish, wait longer before asking again, and if the answer is already in front of you, read the result you already have.",
			call.Name, made), false
	}
	return "", false
}

// sameCallRun is how many of the newest calls in the window, in a row, asked for
// this same thing, whatever each came back with. A different call in between
// ends the run, because the same call after doing something else is ordinary
// work: reading a page again after a click, or running the tests after an edit.
func (running *run) sameCallRun(mark string) int {
	made := 0
	for at := len(running.recentCalls) - 1; at >= 0 && running.recentCalls[at].mark == mark; at-- {
		made++
	}
	return made
}

// sameResultStreak is how many of the newest calls in the window, in a row,
// asked for this same thing and came back with the same answer, which is the
// first rule's count. A call whose answer changed starts a new streak.
func (running *run) sameResultStreak(mark string) int {
	streak := 0
	for at := len(running.recentCalls) - 1; at >= 0 && running.recentCalls[at].mark == mark; at-- {
		if streak > 0 && somethingChanged(running.recentCalls[at], running.recentCalls[at+1]) {
			break
		}
		streak++
	}
	return streak
}

// somethingChanged says whether two neighbouring calls in the window came back
// with different results, which is what makes them two calls rather than the
// same one asked twice. A call that never ran, such as one the detector itself
// refused, has no result and breaks nothing.
func somethingChanged(earlier pastCall, later pastCall) bool {
	return earlier.result != "" && later.result != "" && earlier.result != later.result
}

// rememberCall keeps one call in the detector's window, which holds the last
// few calls of the task and no more.
func (running *run) rememberCall(mark string) {
	window := running.theLoop.options.Caps.IdenticalCallWindow
	running.recentCalls = append(running.recentCalls, pastCall{mark: mark})
	if len(running.recentCalls) > window {
		running.recentCalls = running.recentCalls[len(running.recentCalls)-window:]
	}
}

// noteTheResult writes what the call that has just run came back with into the
// detector's window, as a fingerprint of a fixed size, because a result may be
// megabytes and the window holds only what tells two calls apart.
func (running *run) noteTheResult(text string) {
	if len(running.recentCalls) == 0 {
		return
	}
	running.recentCalls[len(running.recentCalls)-1].result = fingerprintOfText(text)
}

// fingerprintOfText is one result in a fixed number of letters, so that two
// results can be told apart without either being kept.
func fingerprintOfText(text string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(text)))
}

// forgetTheCalls empties the detector's window, which is what a message from
// the user or a stop does: the world has changed, so a call that would have
// been a repeat is worth making again.
func (running *run) forgetTheCalls() {
	running.recentCalls = nil
}

// theFieldsThatSayWhy are the free-text fields of a call that say what the
// model means by it and change nothing about what the tool does: the intent
// the browser and desktop tools ask for, and the expectation they judge. On
// the fifth game build the model asked the desktop tool to launch an
// application it never named eleven times in a row, and the guard saw no
// repeat because every call's intent was worded a little differently.
var theFieldsThatSayWhy = []string{"intent", "why", "goal", "expectation", "reason"}

// fingerprintOf is what makes two calls the same call: the tool's name and its
// arguments with the whitespace taken out, the fields in one order, and the
// fields that only say why left out, so that the same call written twice in
// two ways is still the same call.
func fingerprintOf(call contract.ToolCall) string {
	return call.Name + "\x00" + canonicalArguments(call.Input)
}

// canonicalArguments writes one call's arguments in a form that does not depend
// on how the model spaced, ordered or explained them.
func canonicalArguments(arguments json.RawMessage) string {
	var held any
	if err := json.Unmarshal(arguments, &held); err != nil {
		return strings.TrimSpace(string(arguments))
	}
	if fields, isObject := held.(map[string]any); isObject {
		for _, name := range theFieldsThatSayWhy {
			delete(fields, name)
		}
	}
	written, err := json.Marshal(held)
	if err != nil {
		return strings.TrimSpace(string(arguments))
	}
	return string(written)
}

// stopLineFiredBy returns the line of the stop list that this tool result sets
// off, and is empty when nothing did. Only the harness's own line is read here.
// The lines the model writes are statements about the world that the harness
// cannot check for itself — "the product notes file cannot be found" and "the
// post is longer than the limit after two tries" are about the work, not about
// anything the harness can see — so the model says when one of its own lines
// has come true, with the stop_now operation of the task tool, and the harness
// only recognises the wall it can recognise for itself.
func (running *run) stopLineFiredBy(toolName string, text string) string {
	if !strings.HasPrefix(toolName, "browser") {
		return ""
	}
	seen := strings.ToLower(text)
	for _, wall := range wallWords {
		if !strings.Contains(seen, wall) {
			continue
		}
		if line := running.stopLineAboutTheWall(); line != "" {
			return line
		}
		return HarnessStopLineWall
	}
	return ""
}

// stopLineAboutTheWall is the model's own line about the same wall, when it
// wrote one, so that the user is told the line they were promised rather than
// the harness's words for it. Only the wall's own few words are looked for,
// because the wall is the one thing on a stop list the harness can recognise
// for itself.
func (running *run) stopLineAboutTheWall() string {
	if running.keeper == nil {
		return ""
	}
	for _, line := range running.keeper.Record().Rules.StopWhen {
		written := strings.ToLower(line)
		for _, wall := range wallWords {
			if strings.Contains(written, wall) {
				return line
			}
		}
	}
	return ""
}
