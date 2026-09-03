package browser

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/skill"
)

// healRules is what the model is told before it is asked which element a failed
// step meant. It is short on purpose: the model is being asked one question, and
// the harness checks the answer by acting on it rather than by believing it.
const healRules = "A saved browser procedure has stopped, because the element one of its steps used is not on the page any more. " +
	"You are shown what the step was for and every element on the page as it stands. " +
	"Answer with the reference of the one element that matches what the step was for, such as e12, and nothing else. " +
	"If none of them matches, answer none."

// maxHealReplyTokens caps what the model may write back, because the answer is
// one reference.
const maxHealReplyTokens = 200

// heal is the one model call a replay may make. It asks which element the failed
// step's intent meant, checks the answer by acting on it with the recorded
// expectation, and puts the change to the descriptor to the user as one line.
// Nothing is written without a yes.
func (replayer *Replayer) heal(ctx context.Context, folder skill.Folder, steps []Step, at int, failed Outcome) (Outcome, string, bool) {
	step := steps[at]
	if step.Tool == contract.ToolBrowserOpen {
		return noted(failed, "an opening step has no element to look for again, so the address in it needs fixing by hand"), "", false
	}
	page, err := replayer.options.Browser.Read(ctx, contract.ReadOptions{})
	if err != nil {
		return noted(failed, fmt.Sprintf("the page could not be read to ask the model about it: %v", err)), "", false
	}

	element, asked := replayer.askTheModel(ctx, page, step)
	if !asked {
		return noted(failed, "the model was asked which element this step meant and named none of the ones on the page"), "", false
	}
	change, err := replayer.act(ctx, step, element.Ref)
	if err != nil || !change.ExpectationMet {
		return noted(failed, fmt.Sprintf("the model named %s and acting on it did not do what the step should", element.Ref)), "", false
	}

	patched := step
	patched.Element = Descriptor{Ref: element.Ref, Role: element.Role, Name: element.Name, Shown: element.Name}
	patch := patchFor(folder, step, patched)
	healed := Outcome{Number: step.Number, Intent: step.Intent, Met: true, Healed: true, FoundBy: "the model"}
	return healed, patch, replayer.offerThePatch(ctx, folder, steps, at, patched, patch)
}

// noted adds one line to what a failed step reports, so that a reader sees both
// what the step expected and what was tried afterwards.
func noted(failed Outcome, what string) Outcome {
	failed.Seen = shorten(strings.TrimSpace(failed.Seen+" Then "+what), MaxSeenRunes)
	return failed
}

// patchFor is the one line the user is shown and the one line the changelog
// keeps: which step changed, what it was for, and which element it moves to.
func patchFor(folder skill.Folder, was Step, now Step) string {
	return fmt.Sprintf("step %d of the skill %q, %s the element changes from %s to %s",
		was.Number, folder.Definition.Name, was.Intent, was.Element, now.Element)
}

// askTheModel makes the one model call and reads the reference out of what came
// back. An answer naming nothing on the page is no answer.
func (replayer *Replayer) askTheModel(ctx context.Context, page contract.Snapshot, step Step) (contract.Element, bool) {
	reply, err := replayer.options.Model.Send(ctx, healQuestion(page, step), nil)
	if err != nil {
		return contract.Element{}, false
	}
	return elementNamedIn(reply.Text, page)
}

// healQuestion is the whole of what the model is sent: the rules, what the step
// was for, what it expected, and the page as it stands.
func healQuestion(page contract.Snapshot, step Step) contract.Request {
	return contract.Request{
		SystemBlocks: []contract.SystemBlock{{Name: "self-heal", Text: healRules}},
		Messages: []contract.Message{{Role: contract.RoleUser, Text: fmt.Sprintf(
			"The step is for: %s\nIt expects: %s\nIt used to act on %s.\nThe page %q at %s now holds:\n%s",
			step.Intent, step.Expectation, step.Element, page.Title, page.URL, elementList(page))}},
		MaxOutputTokens: maxHealReplyTokens,
	}
}

// elementList writes the page out for the model, one element a line, capped so
// that a page with thousands of them is not sent whole to answer one question.
func elementList(page contract.Snapshot) string {
	lines := []string{}
	for _, element := range page.Elements {
		if len(lines) >= MaxElementsShownToTheModel {
			lines = append(lines, fmt.Sprintf("(%d more elements are not listed)", len(page.Elements)-len(lines)))
			break
		}
		lines = append(lines, fmt.Sprintf("%s %s %q", element.Ref, element.Role, element.Name))
	}
	return strings.Join(lines, "\n")
}

// elementNamedIn finds the first reference in the model's reply that is really
// on the page. Reading the reply this way rather than as JSON is what makes a
// small model's chattier answer as usable as a terse one.
func elementNamedIn(reply string, page contract.Snapshot) (contract.Element, bool) {
	for _, word := range strings.FieldsFunc(strings.ToLower(reply), notALetterOrDigit) {
		for _, element := range page.Elements {
			if element.Ref != "" && strings.EqualFold(element.Ref, word) {
				return element, true
			}
		}
	}
	return contract.Element{}, false
}

// offerThePatch shows the user the one line and writes the change only on a yes.
func (replayer *Replayer) offerThePatch(ctx context.Context, folder skill.Folder, steps []Step, at int, patched Step, patch string) bool {
	if replayer.options.Ask == nil || replayer.options.Skills == nil {
		return false
	}
	answer, err := replayer.options.Ask(ctx, contract.Preview{
		ID:    fmt.Sprintf("%s-heal-step-%d", folder.Definition.Name, patched.Number),
		Title: fmt.Sprintf("The skill %q found its element somewhere else. Change the step?", folder.Definition.Name),
		Body:  patch,
	})
	if err != nil || answer.Answer == contract.AnswerReject {
		return false
	}
	changed := make([]Step, len(steps))
	copy(changed, steps)
	changed[at] = patched
	return replayer.writeThePatch(ctx, folder, changed, patch) == nil
}

// writeThePatch saves the skill with the one changed step, keeping every other
// file as it stands and writing the patch into the changelog above the line the
// store adds, so that a person reading the changelog sees what changed and not
// only that something did.
func (replayer *Replayer) writeThePatch(ctx context.Context, folder skill.Folder, steps []Step, patch string) error {
	if folder.Path == "" {
		return fmt.Errorf("the skill %q is not on disk, so there is nowhere to write the change", folder.Definition.Name)
	}
	files, err := otherFilesOf(folder.Path)
	if err != nil {
		return err
	}
	files[skill.StepsFile] = RenderSteps(steps)
	files[skill.ChangelogFile] = append(endingInANewline(files[skill.ChangelogFile]), []byte("- self-heal: "+patch+"\n")...)
	return replayer.options.Skills.Save(ctx, folder.Definition.Name, files)
}

// otherFilesOf reads the files of a skill folder that a patch leaves alone, so
// that writing one changed step back does not rewrite anything else.
func otherFilesOf(path string) (map[string][]byte, error) {
	files := map[string][]byte{}
	for _, name := range []string{skill.DescriptionFile, skill.TestFile, skill.ChangelogFile} {
		content, err := os.ReadFile(filepath.Join(path, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("cannot read the %s of the skill at %s, so check that the home folder can be read: %w", name, path, err)
		}
		files[name] = content
	}
	return files, nil
}

// endingInANewline makes sure a file a line is about to be added to ends where a
// line can be added.
func endingInANewline(content []byte) []byte {
	if len(content) == 0 || content[len(content)-1] == '\n' {
		return content
	}
	return append(content, '\n')
}
