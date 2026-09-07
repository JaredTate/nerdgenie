// The idea that a finish is a gate with checks the harness runs for itself,
// rather than something the model may declare, is Prime Agent's, from the
// verifier gates in
// ~/Code/prime-agent/packages/coding-agent/src/core/autonomous.ts. The Go here
// is written fresh.

package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// The bounds on what one done line is checked for.
const (
	// MaxCommandsCheckedPerLine is how many backticked commands one done line
	// may be checked by.
	MaxCommandsCheckedPerLine = 3
	// MaxPathsCheckedPerLine is how many file paths one done line may name.
	MaxPathsCheckedPerLine = 3
	// MaxResultsNamedInARefusal is how many result labels a refusal lists back
	// to the model, because a long task has a hundred results and a refusal is
	// one line the model reads.
	MaxResultsNamedInARefusal = 12
	// MaxMechanicalRefusals is how many times the harness's own file and
	// command checks may send the model back for a done line that names a
	// result. After that the line stands on the model's proof: on the ninth
	// fresh run the check misread a folder's name, refused a line the model
	// had proved, and failed a finished task on its own mistake. A check of
	// the harness is a second opinion, never the judge of a line with proof.
	MaxMechanicalRefusals = 2
)

// doneCheck says what is wrong with the done list, in one line the model can
// act on, and is empty when the task may close. Every line must point at a
// result or at a reply from the user, and where a line names something the
// harness can check for itself, the harness checks it.
func (running *run) doneCheck(ctx context.Context) (string, error) {
	held := running.keeper.Record()
	if err := record.DoneCheck(held); err != nil {
		return "This task cannot close yet. " + err.Error() + " " + theResultsToNameFrom(held), nil
	}
	for at, line := range held.Goal.DoneWhen {
		problem, err := running.checkOneDoneLine(ctx, line)
		if err != nil {
			return "", err
		}
		if problem == "" {
			continue
		}
		if line.ResultID != "" && running.mechanicalRefusals >= MaxMechanicalRefusals {
			running.takeTheLineOnItsProof(ctx, at+1, line, problem)
			continue
		}
		running.mechanicalRefusals++
		return problem, nil
	}
	return "", nil
}

// takeTheLineOnItsProof lets a done line stand on the result the model named
// after the harness's own check has sent the model back as often as it may,
// and writes into the record what the harness could not confirm, so that the
// person reading the record sees which line rests on the model's word.
func (running *run) takeTheLineOnItsProof(ctx context.Context, number int, line contract.DoneLine, problem string) {
	_ = running.keeper.Apply(ctx, record.Update{Decision: &record.NewDecision{
		Text:   fmt.Sprintf("Done line %d stands on the model's proof, which the harness could not confirm: %s", number, problem),
		Reason: fmt.Sprintf("the line names %s and the harness's own check sent the model back %d times", line.ResultID, MaxMechanicalRefusals),
	}})
}

// theResultsToNameFrom names the results this record holds, so that a model
// whose done line pointed at a result that was never written is told which
// labels there are rather than guessing again, and says that the answer itself
// has a label of its own for a line only the answer can prove.
func theResultsToNameFrom(held contract.Record) string {
	labels := []string{}
	for _, one := range held.Work.Results {
		if len(labels) >= MaxResultsNamedInARefusal {
			break
		}
		labels = append(labels, one.ID)
	}
	written := fmt.Sprintf("The results this task has written are %s.", strings.Join(labels, ", "))
	if len(labels) == 0 {
		written = "This task has written no results yet."
	}
	return written + fmt.Sprintf(" A line that only your answer to the user can prove names %q as its result, "+
		"and I write your answer into the record as that result when you give it.", TheReplyLabel)
}

// checkOneDoneLine runs the mechanical checks on one line: a command in
// backticks has to exit zero, a file path has to be there, and a test run the
// line rests on has to be from after the task's last change.
func (running *run) checkOneDoneLine(ctx context.Context, line contract.DoneLine) (string, error) {
	for _, command := range commandsIn(line.Text) {
		wrong, err := running.commandFails(ctx, command)
		if err != nil {
			return "", err
		}
		if wrong != "" {
			return fmt.Sprintf("The done line %q names the command `%s`, and %s, so this line is not true yet.",
				line.Text, command, wrong), nil
		}
	}
	for _, path := range pathsIn(line.Text) {
		if _, err := os.Stat(path); err != nil {
			return fmt.Sprintf("The done line %q names the file %s, and it is not there, so this line is not true yet.",
				line.Text, path), nil
		}
	}
	return running.staleProofRefusal(ctx, line), nil
}

// staleProofRefusal says when a done line rests on a test run from before the
// task's last write or edit, so that the model runs the tests again and points
// the line at the new result. A green run proves the files as they were when
// it ran, and a change after it is a change the run never saw. The refusal is
// empty for a line whose proof is not a test run, or whose run came after the
// last change, or that rests on the reply.
func (running *run) staleProofRefusal(ctx context.Context, line contract.DoneLine) string {
	if line.ResultID == "" || line.ResultID == TheReplyLabel {
		return ""
	}
	proofAt, changeAt := -1, -1
	for at, one := range running.keeper.Record().Work.Results {
		if one.ID == line.ResultID {
			proofAt = at
		}
		if strings.HasPrefix(one.Summary, contract.ToolWrite+": ") || strings.HasPrefix(one.Summary, contract.ToolEdit+": ") {
			changeAt = at
		}
	}
	if proofAt < 0 || changeAt <= proofAt {
		return ""
	}
	text, err := running.keeper.Read(ctx, line.ResultID)
	if err != nil {
		return ""
	}
	if _, found := testStateIn(text); !found {
		return ""
	}
	return fmt.Sprintf("The done line %q rests on %s, a test run from before your last change; run the tests again and point the line at the new result.",
		line.Text, line.ResultID)
}

// MaxWordsInAPath is how many of the words after a path are tried as part of
// it before it is called missing: a folder such as "Tater Tots Tetrisv1"
// writes as three words, and the ninth fresh run's scaffold task ended failed
// on a done line whose folder the check read as its first word alone.
const MaxWordsInAPath = 4

// commandFails rules on one command, runs it inside the sandbox when it is
// allowed, and says what was wrong with it, or nothing when it exited zero. A
// machine with no sandbox cannot check a command at all, and the line stands on
// the result behind it instead.
//
// The command is a command the model wrote, and the words of a page reach a
// done line whenever the model copies them, so it goes through the permission
// function exactly as the shell tool's own calls do and is written into the log
// as a tool call. Nothing the harness runs is outside the rulebook.
func (running *run) commandFails(ctx context.Context, command string) (string, error) {
	if running.theLoop.options.Sandbox == nil {
		return "", nil
	}
	call := theDoneCheckCall(command)
	if err := running.theLoop.logEvent(ctx, running.taskID(), contract.EventToolCall, call); err != nil {
		return "", err
	}
	allowed, refused, err := running.permit(ctx, call)
	if err != nil {
		return "", err
	}
	if !allowed {
		return "it was not allowed to run, because " + refused.said, nil
	}
	result, err := running.theLoop.options.Sandbox.Run(ctx, contract.SandboxCommand{
		Program:   "sh",
		Arguments: []string{"-c", command},
		Timeout:   running.theLoop.options.Caps.TimePerTool,
	})
	if err != nil {
		return "it could not be run: " + err.Error(), nil
	}
	if result.TimedOut {
		return "its time was up before it finished", nil
	}
	if result.ExitCode != 0 {
		return fmt.Sprintf("it exited %d", result.ExitCode), nil
	}
	return "", nil
}

// theDoneCheckCall is the command a done line named, written as the shell call
// it is, so that the rulebook rules on it by the same rules and the log holds
// it in the same shape as every other call.
func theDoneCheckCall(command string) contract.ToolCall {
	written, err := json.Marshal(struct {
		Command string `json:"command"`
	}{Command: command})
	if err != nil {
		written = []byte(`{}`)
	}
	return contract.ToolCall{ID: "done-check", Name: contract.ToolShell, Input: written}
}

// commandsIn is every command a done line wrote between backticks, which is how
// a line says "the harness can check this by running it".
func commandsIn(text string) []string {
	pieces := strings.Split(text, "`")
	commands := []string{}
	for at := 1; at < len(pieces); at += 2 {
		if written := strings.TrimSpace(pieces[at]); written != "" && len(commands) < MaxCommandsCheckedPerLine {
			commands = append(commands, written)
		}
	}
	return commands
}

// pathsIn is every file path a done line names. A path is a word that begins
// the way a path begins, so that an ordinary sentence with a slash in it is
// never mistaken for one.
func pathsIn(text string) []string {
	paths := []string{}
	words := strings.Fields(strings.ReplaceAll(text, "`", " "))
	for at, word := range words {
		word = strings.Trim(word, ".,;:\"'()")
		if !looksLikeAPath(word) || len(paths) >= MaxPathsCheckedPerLine {
			continue
		}
		paths = append(paths, pathAcrossSpaces(expandHome(word), words[at+1:]))
	}
	return paths
}

// pathAcrossSpaces returns the path as written when it is there, and otherwise
// the path with the words after it joined on one at a time, up to a few, the
// first of which is there; a path that is there under no length is returned as
// written, so that the line is refused naming what the model wrote.
func pathAcrossSpaces(path string, following []string) string {
	if _, err := os.Stat(path); err == nil {
		return path
	}
	longer := path
	for at, word := range following {
		if at >= MaxWordsInAPath {
			break
		}
		longer += " " + strings.Trim(word, ".,;:\"'()")
		if _, err := os.Stat(longer); err == nil {
			return longer
		}
	}
	return path
}

// looksLikeAPath says whether a word is written the way a full or an explicitly
// relative path is written.
func looksLikeAPath(word string) bool {
	if strings.Contains(word, "://") {
		return false
	}
	for _, opening := range []string{"/", "./", "../", "~/"} {
		if strings.HasPrefix(word, opening) {
			return true
		}
	}
	return false
}

// expandHome turns a path written with a tilde into one the filesystem knows.
func expandHome(path string) string {
	rest, found := strings.CutPrefix(path, "~/")
	if !found {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, rest)
}
