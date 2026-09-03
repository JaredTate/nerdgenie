// The detector here is ZeroClaw's, from
// ~/Code/zeroclaw/crates/zeroclaw-runtime/src/agent/loop_detector.rs, where a
// window of recent calls is kept and the rule is a run of consecutive identical
// ones rather than any repeat at all, and OpenCode's, from
// ~/Code/opencode/packages/opencode/src/session/processor.ts, where the last
// few parts must all be the same tool with the same input before anything is
// refused. The Go here is written fresh.

package loop

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
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
	// MinWordLettersInAPhrase is how long each word of a stop line's phrase has
	// to be before the phrase is worth matching on. Short words are in every
	// sentence and would fire the line on anything.
	MinWordLettersInAPhrase = 4
	// MaxPhrasesPerStopLine bounds how many phrases one stop line is read as.
	MaxPhrasesPerStopLine = 20
)

// HarnessStopLineWall is the second line the harness adds to every stop list:
// the first is the budget running out, and this is a wall the browser hit.
const HarnessStopLineWall = "the harness's own line: a browser page showed a login form, a two-factor prompt, or a captcha"

// wallWords are what a browser result says when it has hit one of the three
// walls that stop the agent and hand the window to the user.
var wallWords = []string{"login page", "log in page", "sign in page", "captcha", "two-factor", "two factor"}

// budgetIsSpent says why the task's budget is gone, and is empty while it has
// budget left.
func (running *run) budgetIsSpent() string {
	if running.roundsUsed >= running.roundsAllowed {
		return fmt.Sprintf("the budget of %d rounds is used up", running.roundsAllowed)
	}
	if running.spent() >= running.timeAllowed {
		return fmt.Sprintf("the budget of %s is used up", running.timeAllowed)
	}
	return ""
}

// spent is how long this task has been running on the harness's clock.
func (running *run) spent() time.Duration {
	return running.theLoop.options.Clock.Now().Sub(running.startedAt)
}

// detectorRefuses says whether this call is one the model has already made over
// and over. It returns the words to send back when the call is refused, and
// true when the turn has to end because the model will not stop asking.
func (running *run) detectorRefuses(call contract.ToolCall) (string, bool) {
	mark := fingerprintOf(call)
	streak := 0
	for at := len(running.recentCalls) - 1; at >= 0 && running.recentCalls[at] == mark; at-- {
		streak++
	}
	running.rememberCall(mark)
	if streak > IdenticalCallsAllowed {
		return fmt.Sprintf("You have asked for %s with the same arguments %d times in a row, so the turn ends here.",
			call.Name, streak+1), true
	}
	if streak == IdenticalCallsAllowed {
		return fmt.Sprintf("You have already called %s with these exact arguments %d times, and it was not run again. Do something different, or answer the user.",
			call.Name, streak), false
	}
	return "", false
}

// rememberCall keeps one call in the detector's window, which holds the last
// few calls of the task and no more.
func (running *run) rememberCall(mark string) {
	window := running.theLoop.options.Caps.IdenticalCallWindow
	running.recentCalls = append(running.recentCalls, mark)
	if len(running.recentCalls) > window {
		running.recentCalls = running.recentCalls[len(running.recentCalls)-window:]
	}
}

// forgetTheCalls empties the detector's window, which is what a message from
// the user or a stop does: the world has changed, so a call that would have
// been a repeat is worth making again.
func (running *run) forgetTheCalls() {
	running.recentCalls = nil
}

// fingerprintOf is what makes two calls the same call: the tool's name and its
// arguments with the whitespace taken out and the fields in one order, so that
// the same call written twice in two ways is still the same call.
func fingerprintOf(call contract.ToolCall) string {
	return call.Name + "\x00" + canonicalArguments(call.Input)
}

// canonicalArguments writes one call's arguments in a form that does not depend
// on how the model spaced or ordered them.
func canonicalArguments(arguments json.RawMessage) string {
	var held any
	if err := json.Unmarshal(arguments, &held); err != nil {
		return strings.TrimSpace(string(arguments))
	}
	written, err := json.Marshal(held)
	if err != nil {
		return strings.TrimSpace(string(arguments))
	}
	return string(written)
}

// stopLineFiredBy returns the line of the stop list that this tool result sets
// off, and is empty when nothing did. The model's own lines are checked first,
// so the user is told the line they were promised rather than the harness's.
func (running *run) stopLineFiredBy(toolName string, text string) string {
	seen := strings.ToLower(text)
	for _, line := range running.stopList() {
		for _, phrase := range notablePhrases(line) {
			if strings.Contains(seen, phrase) {
				return line
			}
		}
	}
	if strings.HasPrefix(toolName, "browser") {
		for _, wall := range wallWords {
			if strings.Contains(seen, wall) {
				return HarnessStopLineWall
			}
		}
	}
	return ""
}

// stopList is what the model wrote into the record's stop list, and is empty
// before there is a record.
func (running *run) stopList() []string {
	if running.keeper == nil {
		return nil
	}
	return running.keeper.Record().Rules.StopWhen
}

// notablePhrases is how a stop line written in plain English is matched against
// what the harness can see: every pair of neighbouring words long enough to
// mean something on their own. "the account shows a login page or a captcha"
// gives "account shows" and "login page", and a result holding either of those
// is the thing the line was written about.
func notablePhrases(line string) []string {
	words := strings.FieldsFunc(strings.ToLower(line), func(letter rune) bool {
		return !(letter >= 'a' && letter <= 'z') && !(letter >= '0' && letter <= '9')
	})
	phrases := []string{}
	for at := 0; at+1 < len(words) && len(phrases) < MaxPhrasesPerStopLine; at++ {
		if len(words[at]) < MinWordLettersInAPhrase || len(words[at+1]) < MinWordLettersInAPhrase {
			continue
		}
		phrases = append(phrases, words[at]+" "+words[at+1])
	}
	return phrases
}
