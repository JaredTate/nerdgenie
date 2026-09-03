package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	workingcontext "github.com/JaredTate/coeus/internal/context"
	"github.com/JaredTate/coeus/internal/contract"
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
// went, and the model's own last orient line.
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
	if running.lastOrient != "" {
		facts = append(facts, cutToALine("where the work stands: "+running.lastOrient))
	}
	if err := running.keeper.SetSituation(ctx, facts); err != nil {
		return fmt.Errorf("cannot write the situation into the record: %w", err)
	}
	return nil
}

// filesLine names the files this task changed, from the file-change events in
// the log and from the write and edit calls the loop itself saw.
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
	if len(changed) > MaxFilesInTheSituation {
		return fmt.Sprintf("%s and %d more", strings.Join(changed[:MaxFilesInTheSituation], ", "),
			len(changed)-MaxFilesInTheSituation)
	}
	return strings.Join(changed, ", ")
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
	switch {
	case strings.HasPrefix(call.Name, "browser"):
		if first := firstLine(text); first != "" {
			running.browserFact = "browser: " + first
		}
	case call.Name == contract.ToolShell:
		running.commandFact = "last command: " + fieldOfCall(call, "command") + ", " + howItWent(text, failed)
	case call.Name == contract.ToolWrite || call.Name == contract.ToolEdit:
		if path := fieldOfCall(call, "path"); path != "" && !slices.Contains(running.filesChanged, path) {
			running.filesChanged = append(running.filesChanged, path)
		}
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

// exitCodeIn finds an exit code the shell tool wrote into its result, such as
// "exit 0" on a line of its own.
func exitCodeIn(text string) (string, bool) {
	for _, line := range strings.Split(text, "\n") {
		words := strings.Fields(strings.ToLower(strings.TrimSpace(line)))
		for at := 0; at+1 < len(words); at++ {
			if words[at] != "exit" {
				continue
			}
			if number := strings.Trim(words[at+1], ".,"); isWholeNumber(number) {
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
