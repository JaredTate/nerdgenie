// Replaying a recorded step by resolving its element against a fresh snapshot
// and only then acting is Stagehand's observe-then-act idea, described in
// docs/research/16-browser-agent-spec.md section 9.13: the descriptor written
// down at record time is a cache of what to do, and it is checked against the
// page before it is trusted. The three rungs, the reference then the role and
// name then the text, are the cascade that section describes.

package browser

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/skill"
)

// Options is what a replayer is built from. Only the browser is needed. Without
// a model no failed step is ever healed; without a screen a heal is proposed in
// the report and never acted on, because the user is asked before the step is
// taken on an element the recording never named; without a skill store an
// approved patch cannot be written down. Each of the three says so in the report
// rather than failing, so an unattended replay is a replay and not an error.
type Options struct {
	// Browser is the worker driving the agent's own Chrome.
	Browser contract.BrowserWorker
	// Model is asked, once per replay, which element a failed step's intent
	// meant, and may be left out.
	Model contract.Model
	// Ask shows the user a preview and waits, and may be left out.
	Ask skill.AskFunc
	// Skills is where an approved patch is written, and may be left out.
	Skills SkillSaver
}

// Replayer runs a recorded browser skill again with no model call, and calls the
// model only when a step fails.
type Replayer struct {
	options Options
}

// New returns a replayer over one browser worker.
func New(options Options) (*Replayer, error) {
	if options.Browser == nil {
		return nil, errors.New("the replayer was built with no browser behind it, so pass the browser worker to replay through")
	}
	return &Replayer{options: options}, nil
}

// Outcome is what one replayed step did.
type Outcome struct {
	// Number is the step's place in the recording.
	Number int
	// Intent is what the step was for.
	Intent string
	// Met says the step did what it was recorded to do.
	Met bool
	// FoundBy says which rung of the cascade named the element, and is empty on
	// a step that acts on no element.
	FoundBy string
	// Seen says what happened instead, when the step did not do what it should.
	Seen string
	// Healed says the model was asked which element the intent meant, and the
	// answer worked.
	Healed bool
}

// Report is what a whole replay did: one outcome per step it took, and the patch
// a self-heal proposed.
type Report struct {
	// Skill is the name of the recording that was replayed.
	Skill string
	// Outcomes are the steps that were taken, in order, ending at the first one
	// that did not do what it should.
	Outcomes []Outcome
	// Patch is the one line a self-heal proposed, or empty when none was.
	Patch string
	// Applied says the user approved the patch and it was written down.
	Applied bool
}

// Met says every step of the replay did what it was recorded to do.
func (report Report) Met() bool {
	for _, outcome := range report.Outcomes {
		if !outcome.Met {
			return false
		}
	}
	return len(report.Outcomes) > 0
}

// String is the report as plain lines, which is what the loop hands the model
// and what a person reads on the screen.
func (report Report) String() string {
	lines := []string{fmt.Sprintf("replay of the skill %q", report.Skill)}
	for _, outcome := range report.Outcomes {
		lines = append(lines, outcome.line())
	}
	if report.Patch != "" {
		lines = append(lines, patchLine(report.Patch, report.Applied))
	}
	return strings.Join(lines, "\n") + "\n"
}

// line is one step's line of a report.
func (outcome Outcome) line() string {
	said := fmt.Sprintf("- step %d %s:", outcome.Number, outcome.Intent)
	switch {
	case outcome.Met && outcome.Healed:
		return said + " met, after the model was asked which element the intent meant"
	case outcome.Met && outcome.FoundBy != "":
		return said + " met, on the element found by " + outcome.FoundBy
	case outcome.Met:
		return said + " met"
	default:
		return said + " not met. " + outcome.Seen
	}
}

// patchLine says what the self-heal proposed and whether it was written down.
func patchLine(patch string, applied bool) string {
	if applied {
		return "- you approved this change to the skill: " + patch
	}
	return "- this change to the skill was proposed and not made: " + patch
}

// Replay walks a recorded skill through the browser with no model call. Each
// step's element is found again on a fresh snapshot, the step is acted out with
// the expectation it was recorded with, and the first step whose expectation is
// not met stops the replay and says what was seen instead.
func (replayer *Replayer) Replay(ctx context.Context, folder skill.Folder) (Report, error) {
	steps, err := StepsOf(folder)
	if err != nil {
		return Report{}, err
	}

	report := Report{Skill: folder.Definition.Name}
	healsLeft := replayer.healsAllowed()
	for at, step := range steps {
		outcome, mustNotRun, err := replayer.takeStep(ctx, folder, step)
		if err != nil {
			return report, err
		}
		if !outcome.Met && !mustNotRun && healsLeft > 0 {
			healsLeft--
			outcome, report.Patch, report.Applied = replayer.heal(ctx, folder, steps, at, outcome)
		}
		report.Outcomes = append(report.Outcomes, outcome)
		if !outcome.Met {
			break
		}
	}
	return report, nil
}

// healsAllowed is how many failed steps this replay may ask the model about,
// which is none at all when there is no model to ask.
func (replayer *Replayer) healsAllowed() int {
	if replayer.options.Model == nil {
		return 0
	}
	return HealAttempts
}

// takeStep runs one recorded step. Its second answer says the step must not be
// run at all, which is what a refusal of a step that cannot be undone means: such
// a step is never healed either, because healing a step is taking it. A step that
// could not be carried out is an outcome saying so rather than an error, because
// the replay's answer to a page that changed is a report, not a crash. Only a
// browser that cannot be reached at all is an error.
func (replayer *Replayer) takeStep(ctx context.Context, folder skill.Folder, step Step) (Outcome, bool, error) {
	outcome := Outcome{Number: step.Number, Intent: step.Intent}
	if refused := replayer.askAboutAStepThatCannotBeUndone(ctx, folder, step); refused != "" {
		outcome.Seen = refused
		return outcome, true, nil
	}
	if step.Tool == contract.ToolBrowserOpen {
		opened, err := replayer.openStep(ctx, step, outcome)
		return opened, false, err
	}

	page, err := replayer.options.Browser.Read(ctx, contract.ReadOptions{})
	if err != nil {
		return Outcome{}, false, fmt.Errorf("cannot read the page before step %d of the skill %q: %w", step.Number, folder.Definition.Name, err)
	}
	ref, foundBy, found := FindElement(page, step.Element)
	if !found {
		outcome.Seen = fmt.Sprintf("expected %q, and there is no element on the page matching %s, on %s",
			step.Expectation, step.Element, WhatIsShown(page))
		return outcome, false, nil
	}
	outcome.FoundBy = foundBy
	acted, err := replayer.actOnStep(ctx, step, ref, outcome)
	return acted, false, err
}

// openStep goes to the address a step recorded and judges the page it landed on
// by the word rule, because opening a page changes everything and so there is no
// change for the worker to judge.
func (replayer *Replayer) openStep(ctx context.Context, step Step, outcome Outcome) (Outcome, error) {
	page, err := replayer.options.Browser.Open(ctx, step.Address)
	if err != nil {
		outcome.Seen = fmt.Sprintf("expected %q, and %s could not be opened: %v", step.Expectation, step.Address, err)
		return outcome, nil
	}
	outcome.Met = StateMet(step.Expectation, page)
	if !outcome.Met {
		outcome.Seen = fmt.Sprintf("expected %q, and what opened was %s", step.Expectation, WhatIsShown(page))
	}
	return outcome, nil
}

// actOnStep does the step's action on the element the cascade found, handing the
// worker the recorded expectation so that the worker judges a replayed step the
// same way it judges one the model asked for.
func (replayer *Replayer) actOnStep(ctx context.Context, step Step, ref string, outcome Outcome) (Outcome, error) {
	change, err := replayer.act(ctx, step, ref)
	if err != nil {
		outcome.Seen = fmt.Sprintf("expected %q, and the browser could not act on %s: %v", step.Expectation, ref, err)
		return outcome, nil
	}
	outcome.Met = change.ExpectationMet
	if !outcome.Met {
		outcome.Seen = shorten(change.Seen, MaxSeenRunes)
	}
	return outcome, nil
}

// act is the one place a recorded step becomes a call on the browser worker.
func (replayer *Replayer) act(ctx context.Context, step Step, ref string) (contract.Diff, error) {
	if step.Tool == contract.ToolBrowserType {
		return replayer.options.Browser.Type(ctx, ref, step.Typed, step.Expectation)
	}
	return replayer.options.Browser.Click(ctx, ref, step.Expectation)
}

// askAboutAStepThatCannotBeUndone shows the user a step the skill's permissions
// block marks as one that cannot be undone, and returns what to report when the
// step must not run. An empty answer means go ahead.
func (replayer *Replayer) askAboutAStepThatCannotBeUndone(ctx context.Context, folder skill.Folder, step Step) string {
	if !marksAsIrreversible(folder, step.Number) {
		return ""
	}
	if replayer.options.Ask == nil {
		return fmt.Sprintf("step %d cannot be undone and there is no screen to ask on, so the replay stopped before it: %s",
			step.Number, step.Intent)
	}
	answer, err := replayer.options.Ask(ctx, contract.Preview{
		ID:    fmt.Sprintf("%s-step-%d", folder.Definition.Name, step.Number),
		Title: fmt.Sprintf("Step %d of the skill %q cannot be undone.", step.Number, folder.Definition.Name),
		Body:  step.Intent + "\n" + step.Element.String() + "\nExpected: " + step.Expectation,
	})
	switch {
	case err != nil:
		return fmt.Sprintf("step %d cannot be undone and you could not be asked about it: %v", step.Number, err)
	case answer.Answer == contract.AnswerReject:
		return strings.TrimSpace(fmt.Sprintf("step %d cannot be undone and you refused it. %s", step.Number, answer.Reason))
	default:
		return ""
	}
}

// marksAsIrreversible says whether the skill's permissions block names this step
// as one that cannot be undone.
func marksAsIrreversible(folder skill.Folder, number int) bool {
	return holdsNumber(folder.Definition.Permissions.IrreversibleSteps, number)
}

// holdsNumber says whether the list of step numbers holds this one.
func holdsNumber(numbers []int, wanted int) bool {
	for _, number := range numbers {
		if number == wanted {
			return true
		}
	}
	return false
}

// FindElement walks the descriptor's three rungs down a fresh snapshot and
// returns the reference to act on and which rung found it. The reference comes
// first because it is exact, the role and name next because they survive a page
// being renumbered, and the visible text last because it survives an element
// changing what kind of thing it is.
func FindElement(page contract.Snapshot, wanted Descriptor) (string, string, bool) {
	if wanted.Ref != "" {
		for _, element := range page.Elements {
			if element.Ref == wanted.Ref {
				return element.Ref, "its reference", true
			}
		}
	}
	if wanted.Role != "" && wanted.Name != "" {
		for _, element := range page.Elements {
			if strings.EqualFold(element.Role, wanted.Role) && strings.EqualFold(element.Name, wanted.Name) {
				return element.Ref, "its role and name", true
			}
		}
	}
	return findByText(page, wanted)
}

// findByText is the last rung: the text the element showed when the step was
// recorded, looked for inside the name of anything on the page now. Two things
// keep the rung from finding whatever it likes. Text shorter than a word is not
// looked for at all, because two letters sit inside almost any name. And an
// element whose role has changed is taken only when the recorded text makes up
// most of its name, so that a page renaming a link from "Change the page" to
// "Change the page now" is still followed while a link called "Delete" is not
// followed onto the button called "Delete my account".
func findByText(page contract.Snapshot, wanted Descriptor) (string, string, bool) {
	shown := strings.ToLower(strings.TrimSpace(wanted.Shown))
	if shown == "" {
		shown = strings.ToLower(strings.TrimSpace(wanted.Name))
	}
	if len([]rune(shown)) < shortestTextToLookFor {
		return "", "", false
	}
	for _, element := range page.Elements {
		if strings.Contains(strings.ToLower(element.Name), shown) && strings.EqualFold(element.Role, wanted.Role) {
			return element.Ref, "the text it showed", true
		}
	}
	for _, element := range page.Elements {
		name := strings.ToLower(element.Name)
		if strings.Contains(name, shown) && mostOfTheName(shown, name) {
			return element.Ref, "the text it showed", true
		}
	}
	return "", "", false
}

// mostOfTheName says whether the recorded text makes up enough of the name it
// was found inside for the two to be the same thing.
func mostOfTheName(shown string, name string) bool {
	return len([]rune(shown))*100 >= len([]rune(name))*leastOfTheNameOutOfAHundred
}
