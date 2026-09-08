package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
	"github.com/JaredTate/nerdgenie/internal/workorder"
)

// A done line the person wrote with a check in brackets is proved by the
// harness and never by the model's word: "[tests pass: npm test]" runs the
// command and reads the test count, "[exit 0: cmd]" reads the exit code,
// "[shows: "text" at url]" opens the page in the browser and looks for the
// text, "[exists: path]" looks for the file, and "[looks: url at 1440, 390]"
// opens the page at each width and reads whether it overflows and whether
// the console holds an error (lookscheck.go). A check that passes is
// written into the record as a result the line then points at, so the proof
// is in the log like any other. A check that fails sends the model back with
// the line named and the output's tail, and never counts as one of the
// model's own done-check nudges; the third failure of one check in one task
// goes into the record as a failure with its cause, so the model goes on by
// another route with the lesson in front of it, and the finish stays refused
// until the check passes.

// CheckFailuresBeforeAFailure is how many times one check may fail in one
// task before the harness writes the failure into the record.
const CheckFailuresBeforeAFailure = 3

// MaxCheckTailLetters is how much of a failed check's output rides in the
// refusal, from its end, where the failing tests and the exit are.
const MaxCheckTailLetters = 600

// checkOutcome is what one run of a check showed.
type checkOutcome struct {
	// passed says the check held.
	passed bool
	// said is what the check showed in a few words, for the result's summary
	// or the refusal: "all 10 passing", "2 failing of 10: a, b", "it exited 1".
	said string
	// output is the whole of what the check produced, kept as the result.
	output string
}

// proveTheCheckedLines runs every bracketed check on the done list, marks
// the lines whose checks passed with the results it wrote for them, and hands
// back the refusal for the first check that failed, or nothing when every
// check held. It says on the run whether the refusal came from a check.
func (running *run) proveTheCheckedLines(ctx context.Context) (string, error) {
	running.refusedByACheck = false
	for at, line := range running.keeper.Record().Goal.DoneWhen {
		check, found := workorder.ReadCheck(line.Text)
		if !found {
			continue
		}
		shown, err := running.runTheCheck(ctx, check)
		if err != nil {
			return "", err
		}
		if shown.passed {
			if err := running.markTheLineWithTheCheck(ctx, at+1, line, check, shown); err != nil {
				return "", err
			}
			continue
		}
		running.refusedByACheck = true
		return running.refuseOnTheCheck(ctx, at+1, line, check, shown), nil
	}
	return "", nil
}

// markTheLineWithTheCheck writes the check's output as a result and points
// the line at it, once; a line already resting on a check's result is left.
func (running *run) markTheLineWithTheCheck(ctx context.Context, number int, line contract.DoneLine, check workorder.Check, shown checkOutcome) error {
	if line.Done && line.ResultID != "" {
		return nil
	}
	summary := "check: " + check.Kind + ": " + check.Argument
	if shown.said != "" {
		summary += ": " + shown.said
	}
	label, err := running.keeper.AddResult(ctx, summary, summary+"\n"+shown.output)
	if err != nil {
		return fmt.Errorf("cannot write the check of done line %d into the record: %w", number, err)
	}
	return running.markTheDoneLine(ctx, number, label)
}

// refuseOnTheCheck is the line the model reads when a check failed, and the
// failure written after the third time.
func (running *run) refuseOnTheCheck(ctx context.Context, number int, line contract.DoneLine, check workorder.Check, shown checkOutcome) string {
	if line.Done {
		lines := append([]contract.DoneLine(nil), running.keeper.Record().Goal.DoneWhen...)
		lines[number-1].Done, lines[number-1].ResultID = false, ""
		_ = running.keeper.Apply(ctx, record.Update{DoneWhen: lines})
	}
	if running.checkFailures == nil {
		running.checkFailures = map[int]int{}
	}
	running.checkFailures[number]++
	if running.checkFailures[number] == CheckFailuresBeforeAFailure {
		_ = running.keeper.Apply(ctx, record.Update{Failure: &record.NewFailure{
			Text:  fmt.Sprintf("the check [%s: %s] of done line %d failed %d times: %s", check.Kind, check.Argument, number, CheckFailuresBeforeAFailure, shown.said),
			Cause: "the work the line names is not done yet, so take another route to it",
		}})
	}
	refusal := fmt.Sprintf("The done line %q has its check, and %s, so this line is not true yet.", line.Text, shown.said)
	if tail := tailOf(shown.output); tail != "" {
		refusal += "\nWhat the check showed, from its end:\n" + tail
	}
	return refusal
}

// runTheCheck runs one check of any of the five kinds; the looks check is
// in lookscheck.go.
func (running *run) runTheCheck(ctx context.Context, check workorder.Check) (checkOutcome, error) {
	switch check.Kind {
	case "tests pass":
		return running.checkTheTestsPass(ctx, check.Argument)
	case "exit 0":
		return running.checkTheExitCode(ctx, check.Argument)
	case "shows":
		return running.checkThePageShows(ctx, check.Text, check.URL)
	case "looks":
		return running.checkThePageLooks(ctx, check.URL, check.Widths)
	default:
		return running.checkTheFileExists(check.Argument), nil
	}
}

// checkTheTestsPass runs the command and reads its output as a test run.
func (running *run) checkTheTestsPass(ctx context.Context, command string) (checkOutcome, error) {
	output, exitCode, problem, err := running.runForOutput(ctx, command)
	if err != nil || problem != "" {
		return checkOutcome{said: problem, output: output}, err
	}
	state, found := testStateIn(output)
	if !found {
		if exitCode != 0 {
			return checkOutcome{said: fmt.Sprintf("it exited %d and its output did not read as a test run", exitCode), output: output}, nil
		}
		return checkOutcome{said: "its output did not read as a test run", output: output}, nil
	}
	return checkOutcome{passed: state.failed == 0, said: strings.TrimPrefix(state.line(), "tests: "), output: output}, nil
}

// checkTheExitCode runs the command and reads its exit code.
func (running *run) checkTheExitCode(ctx context.Context, command string) (checkOutcome, error) {
	output, exitCode, problem, err := running.runForOutput(ctx, command)
	if err != nil || problem != "" {
		return checkOutcome{said: problem, output: output}, err
	}
	if exitCode != 0 {
		return checkOutcome{said: fmt.Sprintf("it exited %d", exitCode), output: output}, nil
	}
	return checkOutcome{passed: true, said: "it exited 0", output: output}, nil
}

// runForOutput runs a command the way the done check's command runner does,
// through the rulebook and the sandbox, and hands back what it printed and
// how it ended, or the reason it could not be run.
func (running *run) runForOutput(ctx context.Context, command string) (string, int, string, error) {
	if running.theLoop.options.Sandbox == nil {
		return "", 0, "there is no sandbox on this machine to run it in", nil
	}
	command = running.inTheProjectFolder(command)
	call := theDoneCheckCall(command)
	if err := running.theLoop.logEvent(ctx, running.taskID(), contract.EventToolCall, call); err != nil {
		return "", 0, "", err
	}
	allowed, refused, err := running.permit(ctx, call)
	if err != nil {
		return "", 0, "", err
	}
	if !allowed {
		return "", 0, "it was not allowed to run, because " + refused.said, nil
	}
	result, err := running.theLoop.options.Sandbox.Run(ctx, contract.SandboxCommand{
		Program:   "sh",
		Arguments: []string{"-c", command},
		Timeout:   running.theLoop.options.Caps.TimePerTool,
	})
	if err != nil {
		return "", 0, "it could not be run: " + err.Error(), nil
	}
	output := strings.TrimSpace(string(result.StandardOutput) + "\n" + string(result.StandardError))
	if result.TimedOut {
		return output, result.ExitCode, "its time was up before it finished", nil
	}
	return output, result.ExitCode, "", nil
}

// checkThePageShows opens the page with the browser tool and looks for the
// text in what the tool read, without regard to case.
func (running *run) checkThePageShows(ctx context.Context, text string, url string) (checkOutcome, error) {
	browser, found := running.tools().Lookup(contract.ToolBrowserOpen)
	if !found {
		return checkOutcome{said: "there is no browser tool on this machine to open it with"}, nil
	}
	arguments, err := json.Marshal(map[string]string{"url": url, "intent": "the done check looks for " + text})
	if err != nil {
		return checkOutcome{}, err
	}
	output, err := running.underTheTimeLimit(ctx, browser, contract.ToolCall{ID: "done-check-page", Name: contract.ToolBrowserOpen, Input: arguments})
	if err != nil {
		return checkOutcome{said: "the page could not be opened: " + err.Error()}, nil
	}
	if !strings.Contains(strings.ToLower(output.Text), strings.ToLower(text)) {
		return checkOutcome{said: fmt.Sprintf("the page at %s does not show %q", url, text), output: output.Text}, nil
	}
	return checkOutcome{passed: true, said: fmt.Sprintf("the page shows %q", text), output: output.Text}, nil
}

// checkTheFileExists looks for the path, under the working folder when it is
// not written from the root.
func (running *run) checkTheFileExists(path string) checkOutcome {
	whole := aPathUnder(running.folder(), path)
	if _, err := os.Stat(whole); err != nil {
		return checkOutcome{said: fmt.Sprintf("the file %s is not there", path)}
	}
	return checkOutcome{passed: true, said: fmt.Sprintf("the file %s is there", path)}
}

// tailOf is the end of a check's output, MaxCheckTailLetters at most, cut on
// a line.
func tailOf(output string) string {
	output = strings.TrimSpace(output)
	if len(output) <= MaxCheckTailLetters {
		return output
	}
	cut := output[len(output)-MaxCheckTailLetters:]
	if at := strings.Index(cut, "\n"); at >= 0 {
		cut = cut[at+1:]
	}
	return cut
}
