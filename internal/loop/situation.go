package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	workingcontext "github.com/JaredTate/nerdgenie/internal/context"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/record"
)

// The bounds on the few facts the harness writes into the record for itself.
const (
	// MaxFilesInTheSituation is how many changed files the situation names
	// before it says how many more there are.
	MaxFilesInTheSituation = 8
	// MaxSituationLineLetters is how long one line of the situation may be. The
	// orient line comes from the model, so it has to be cut somewhere.
	MaxSituationLineLetters = 160
)

// writeCostAndBudget writes what the last call cost and how much of the budget
// is left. Both are the harness's own bookkeeping and cost no model call.
func (running *run) writeCostAndBudget(ctx context.Context, usage contract.Usage) error {
	if running.keeper == nil {
		return nil
	}
	if err := running.keeper.SetBudget(ctx, running.budgetLeft()); err != nil {
		return fmt.Errorf("cannot write the budget left into the record: %w", err)
	}
	return workingcontext.WriteCostLine(ctx, running.keeper, usage)
}

// writeSituation fills the record's situation from the facts ordinary code can
// check for itself: that the person asked a stopped task to carry on, the page
// the browser is on, the files changed in this task, the last command and how it
// went, the state of the last test run, and the model's own last orient line.
func (running *run) writeSituation(ctx context.Context) error {
	if running.keeper == nil {
		return nil
	}
	facts := []string{}
	if running.continuedFact != "" {
		facts = append(facts, cutToALine(running.continuedFact))
	}
	if running.browserFact != "" {
		facts = append(facts, cutToALine(running.browserFact))
	}
	facts = append(facts, "files changed in this task: "+running.filesLine(ctx))
	if running.commandFact != "" {
		facts = append(facts, cutToALine(running.commandFact))
	}
	if running.testsFact != "" {
		facts = append(facts, cutToALine(running.testsFact))
	}
	if line := running.theProgressLine(); line != "" {
		facts = append(facts, line)
	}
	if running.lastOrient != "" {
		facts = append(facts, cutToALine("where the work stands: "+running.lastOrient))
	}
	if err := running.keeper.SetSituation(ctx, facts); err != nil {
		return fmt.Errorf("cannot write the situation into the record: %w", err)
	}
	return nil
}

// noteTheBrowserResult keeps what one browser result shows: the page's first
// line as the situation's browser line, the marked loop of a hung page in its
// place when there is one, and the page's address for the reading of its
// scripts. A hung page's line stands until a result carries a page again,
// because an open or a click that fails after the hang is the same hang: the
// fifth sitting lost the marked loop to "cannot open the page" and went back
// to re-reading.
func (running *run) noteTheBrowserResult(text string) {
	marked, address := theMarkedLoopIn(text), addressIn(text)
	switch {
	case marked != "":
		running.browserFact = TheHungPageFact + marked
	case strings.HasPrefix(running.browserFact, TheHungPageFact) && address == "":
		// The page is still hung, and the loop is still the thing to fix.
	default:
		if title := pageTitleIn(text); title != "" {
			running.browserFact = "browser: " + title
		}
	}
	if address != "" {
		running.pageAddress = address
	}
}

// takeTheBrowserFactBackFromTheLog gives a picked-up task the page it was on
// and, when the page hung, its marked loop, by reading every browser result
// the log holds under it in order, the way the sittings before it read them.
// The fourth sitting of the fifth game build's play-test task started with an
// empty browser line because the fact lived in memory, and the model re-read
// the old result four times. A log that cannot be read costs the situation
// one line and nothing more.
func (running *run) takeTheBrowserFactBackFromTheLog(ctx context.Context) {
	events, err := running.theLoop.options.Store.ByTask(ctx, running.keeper.LogKey())
	if err != nil {
		return
	}
	lastCall := ""
	for _, event := range events {
		switch event.Kind {
		case contract.EventToolCall:
			call := contract.ToolCall{}
			if err := json.Unmarshal(event.Body, &call); err == nil {
				lastCall = call.Name
			}
		case contract.EventToolResult:
			if !strings.HasPrefix(lastCall, "browser") {
				continue
			}
			result := record.StoredResult{}
			if err := json.Unmarshal(event.Body, &result); err == nil && result.Text != "" {
				running.noteTheBrowserResult(result.Text)
			}
		}
	}
}

// filesLine names the files this task changed, from the file-change events in
// the log and from the write and edit calls the loop itself saw, by their last
// path element, because the line is rewritten under the conversation on every
// call and eight full paths were a quarter of a thousand tokens of it.
func (running *run) filesLine(ctx context.Context) string {
	changed := slices.Clone(running.filesChanged)
	for _, path := range running.loggedFileChanges(ctx) {
		if !slices.Contains(changed, path) {
			changed = append(changed, path)
		}
	}
	if len(changed) == 0 {
		return "none"
	}
	return shortNames(changed, MaxFilesInTheSituation)
}

// loggedFileChanges is every file the tools wrote down as changed under this
// task. A log that cannot be read costs the situation one line and nothing
// more, so the error is not carried up.
func (running *run) loggedFileChanges(ctx context.Context) []string {
	events, err := running.theLoop.options.Store.ByTask(ctx, running.keeper.LogKey())
	if err != nil {
		return nil
	}
	paths := []string{}
	for _, event := range events {
		if event.Kind != contract.EventFileChange {
			continue
		}
		body := contract.FileChangeBody{}
		if err := json.Unmarshal(event.Body, &body); err == nil && body.Path != "" {
			paths = append(paths, body.Path)
		}
	}
	return paths
}

// noteWhatTheResultShows reads the few facts the harness keeps for itself out
// of one tool result: the page the browser is on, the last command and how it
// went, and the files a write or an edit touched.
func (running *run) noteWhatTheResultShows(call contract.ToolCall, text string, failed bool) {
	if !failed {
		running.noteAReadOfSomethingNew(call)
		running.noteAResultThatSaysSomethingNew(call, text)
	}
	switch {
	case strings.HasPrefix(call.Name, "browser"):
		running.noteTheBrowserResult(text)
		running.noteAChangedPage(text)
	case call.Name == contract.ToolShell:
		running.commandFact = "last command: " + fieldOfCall(call, "command") + ", " + howItWent(text, failed)
	case call.Name == contract.ToolTask && !failed && fieldOfCall(call, "operation") == "failure":
		running.roundsSinceAFailureWrite = 0
	case call.Name == contract.ToolWrite || call.Name == contract.ToolEdit:
		path := fieldOfCall(call, "path")
		if path != "" && !slices.Contains(running.filesChanged, path) {
			running.filesChanged = append(running.filesChanged, path)
			// The first change of a file is progress, a write or an edit,
			// the way the first read of one is; the same file changed round
			// after round is not.
			if !failed && !writesAProbe(call) {
				running.noteProgress()
			}
		}
		if path != "" && !failed && !slices.Contains(running.changedSinceTheLastRun, path) {
			running.changedSinceTheLastRun = append(running.changedSinceTheLastRun, path)
		}
	case call.Name == contract.ToolJob && !failed:
		running.noteTheJobMade(text)
	}
}

// noteTheJobMade writes down a job this task made, read off the job tool's
// answer, which begins "created job 4". The stopped report of a person's task
// names the job it made when that job is running, because the person's next
// word would otherwise be taken for the task and land on the job's.
func (running *run) noteTheJobMade(answer string) {
	rest, made := strings.CutPrefix(firstLine(answer), "created job ")
	if !made {
		return
	}
	jobID, _, _ := strings.Cut(rest, " ")
	if jobID == "" || slices.Contains(running.jobsMade, jobID) {
		return
	}
	if strings.Contains(rest, " and started task ") {
		running.jobToHandTo = jobID
	}
	running.jobsMade = append(running.jobsMade, jobID)
	if len(running.jobsMade) > MaxJobsMadeNoted {
		running.jobsMade = running.jobsMade[1:]
	}
}

// howItWent says how the last command ended: the exit code when the result
// names one, and whether it worked when it does not.
func howItWent(text string, failed bool) string {
	if code, found := exitCodeIn(text); found {
		return "exit " + code
	}
	if failed {
		return "it failed"
	}
	return "it worked"
}

// exitCodeIn finds an exit code the shell tool wrote into its result: its own
// first line, "finished with exit code 0", or a bare "exit 0" on a line of its
// own. The checker read only the bare form once, and on the eighth fresh run
// every command the model expected to exit 0 was told it had no exit code.
func exitCodeIn(text string) (string, bool) {
	for _, line := range strings.Split(text, "\n") {
		words := strings.Fields(strings.ToLower(strings.TrimSpace(line)))
		for at := 0; at+1 < len(words); at++ {
			if words[at] != "exit" {
				continue
			}
			next := words[at+1]
			if next == "code" && at+2 < len(words) {
				next = words[at+2]
			}
			if number := strings.Trim(next, ".,:;"); isWholeNumber(number) {
				return number, true
			}
		}
	}
	return "", false
}

// isWholeNumber says whether the text is a run of digits and nothing else.
func isWholeNumber(text string) bool {
	if text == "" {
		return false
	}
	for _, letter := range text {
		if letter < '0' || letter > '9' {
			return false
		}
	}
	return true
}

// fieldOfCall reads one named piece of text out of a call's arguments.
func fieldOfCall(call contract.ToolCall, name string) string {
	written := map[string]any{}
	if err := json.Unmarshal(call.Input, &written); err != nil {
		return ""
	}
	text, isText := written[name].(string)
	if !isText {
		return ""
	}
	return text
}

// cutToALine keeps one line of the situation inside its cap and on one line.
func cutToALine(text string) string {
	one := strings.Join(strings.Fields(text), " ")
	letters := []rune(one)
	if len(letters) <= MaxSituationLineLetters {
		return one
	}
	return string(letters[:MaxSituationLineLetters-3]) + "..."
}
