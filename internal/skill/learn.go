// Learning a skill from a page of documentation follows the authoring standards
// in Hermes' learn prompt at ~/Code/hermes-agent/agent/learn_prompt.py: a
// lowercase hyphenated name, one sentence of description, and commands taken
// from the source word for word with one example of each rather than invented.
// Offering a finished task's procedure back to the user is ZeroClaw's creator at
// ~/Code/zeroclaw/crates/zeroclaw-runtime/src/skills/creator.rs, which builds a
// skill out of the tool calls a finished run made; Coeus asks before it writes,
// because a skill saved without a yes is a procedure nobody chose.

package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// MaxPageBytes is how much of a documentation page is read when a skill is
// learned from it, because the page comes from the web and a page with no limit
// on it is a way to fill the machine.
const MaxPageBytes = 512 * 1024

// The words that turn the fourth answer of an after-action review from a fact
// into a procedure. A fact is one thing that is true; a procedure is a thing
// done in an order, and an order needs more than one step to have.
var procedureWords = []string{"first", "then", "next", "after that", "finally", "step "}

// LearnFromPage writes a skill from a page of documentation: the commands the
// page shows, one example of each, in the order they appear. The skill has to
// pass its own dry run before anything is written, so a page whose commands do
// not work saves nothing.
func (store *Store) LearnFromPage(ctx context.Context, name string, page string) (contract.SkillSummary, error) {
	if err := CheckName(name); err != nil {
		return contract.SkillSummary{}, err
	}
	if len(page) > MaxPageBytes {
		page = page[:MaxPageBytes]
	}

	commands := commandsInPage(page)
	if len(commands) == 0 {
		return contract.SkillSummary{}, fmt.Errorf("the page about %q shows no commands to learn, so point at a page with its commands written out", name)
	}

	files := filesForCommands(name, commands)
	folder, err := ParseFolder(files)
	if err != nil {
		return contract.SkillSummary{}, err
	}
	if _, err := store.dryRun(ctx, folder); err != nil {
		return contract.SkillSummary{}, fmt.Errorf("the skill learned from the page about %q did not pass its own dry run, so nothing was saved: %w", name, err)
	}
	if err := store.Save(ctx, contract.SkillSavedByModel, name, files); err != nil {
		return contract.SkillSummary{}, err
	}
	return folder.Definition.Summary(), nil
}

// commandsInPage returns the commands a documentation page shows, one example
// of each: the lines inside fenced code blocks and the lines a page marks with
// a shell prompt. Two commands count as the same when their first two words are
// the same, which is how "git commit -m one" and "git commit -m two" become one
// example.
func commandsInPage(page string) []string {
	commands := []string{}
	seen := map[string]bool{}
	fenced := false
	for _, raw := range strings.Split(strings.ReplaceAll(page, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
			continue
		}
		command := strings.TrimSpace(strings.TrimPrefix(line, "$ "))
		if command == "" || (!fenced && !strings.HasPrefix(line, "$ ")) {
			continue
		}
		shape := shapeOfCommand(command)
		if seen[shape] {
			continue
		}
		seen[shape] = true
		commands = append(commands, command)
		if len(commands) >= MaxSteps {
			break
		}
	}
	return commands
}

// shapeOfCommand is the first two words of a command, which is what tells one
// command apart from another example of the same one.
func shapeOfCommand(command string) string {
	words := strings.Fields(command)
	if len(words) > 2 {
		words = words[:2]
	}
	return strings.Join(words, " ")
}

// filesForCommands builds the folder of a skill learned from a page: one shell
// step per command, in the order the page showed them.
func filesForCommands(name string, commands []string) map[string][]byte {
	steps := make([]Step, 0, len(commands))
	for position, command := range commands {
		input, err := json.Marshal(map[string]string{"command": command})
		if err != nil {
			continue
		}
		steps = append(steps, Step{
			Number: position + 1,
			Intent: "Run " + command + ", which the documentation shows.",
			Tool:   contract.ToolShell,
			Input:  string(input),
		})
	}
	definition := Definition{
		Name:        name,
		Description: "Runs " + name + " the way its own documentation says to.",
		Triggers:    []string{name},
		Permissions: Permissions{DailyLimit: DefaultDailyLimit},
	}
	return newFolderFiles(definition, steps)
}

// newFolderFiles renders a definition and its steps as the four files of a
// skill folder, which is the shape Save takes.
func newFolderFiles(definition Definition, steps []Step) map[string][]byte {
	return map[string][]byte{
		DescriptionFile: RenderDescriptionFile(definition),
		StepsFile:       RenderSteps(steps),
		TestFile:        RenderTestFile(definition.Name, DryRunPlan{}),
		ChangelogFile:   []byte("# changelog for " + definition.Name + "\n\n"),
	}
}

// OfferFromTask offers to save a finished task's plan as a skill, and saves it
// when the user says yes. The steps are the task's own plan in the words it was
// written in, so the user is approving something they can read.
func (store *Store) OfferFromTask(ctx context.Context, name string, record contract.Record) (bool, error) {
	if err := CheckName(name); err != nil {
		return false, err
	}
	steps := []Step{}
	for _, planned := range record.Work.Plan {
		if !planned.Done || strings.TrimSpace(planned.Text) == "" {
			continue
		}
		steps = append(steps, Step{Number: len(steps) + 1, Intent: strings.TrimSpace(planned.Text)})
		if len(steps) >= MaxSteps {
			break
		}
	}
	if len(steps) == 0 {
		return false, fmt.Errorf("the task has no finished plan steps to save as the skill %q, so there is no procedure to offer", name)
	}

	definition := Definition{
		Name:        name,
		Description: oneLine(record.Goal.Ask, "Repeats what the task called "+name+" did."),
		Triggers:    strings.Fields(strings.ReplaceAll(name, "-", " ")),
		Permissions: Permissions{DailyLimit: DefaultDailyLimit},
	}
	return store.offer(ctx, name, newFolderFiles(definition, steps), "the task you just finished")
}

// OfferFromReview offers to save the fourth answer of an after-action review as
// a skill, when that answer describes a way of doing something rather than a
// fact. An answer that is a fact belongs in memory instead, and this says so by
// offering nothing.
func (store *Store) OfferFromReview(ctx context.Context, name string, answer string) (bool, error) {
	if err := CheckName(name); err != nil {
		return false, err
	}
	if !DescribesAProcedure(answer) {
		return false, nil
	}

	steps := []Step{}
	for _, line := range strings.Split(strings.ReplaceAll(answer, ";", "\n"), "\n") {
		written := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "-*0123456789. "))
		if written == "" {
			continue
		}
		steps = append(steps, Step{Number: len(steps) + 1, Intent: written})
		if len(steps) >= MaxSteps {
			break
		}
	}
	definition := Definition{
		Name:        name,
		Description: oneLine(answer, "Does what the after-action review said to keep doing."),
		Triggers:    strings.Fields(strings.ReplaceAll(name, "-", " ")),
		Permissions: Permissions{DailyLimit: DefaultDailyLimit},
	}
	return store.offer(ctx, name, newFolderFiles(definition, steps), "the review of the task you just finished")
}

// DescribesAProcedure says whether a line of text is a way of doing something
// rather than a fact. A procedure has an order to it, so it either lists its
// steps or names them with words like "first" and "then".
func DescribesAProcedure(answer string) bool {
	folded := strings.ToLower(answer)
	for _, word := range procedureWords {
		if strings.Contains(folded, word) {
			return true
		}
	}
	listed := 0
	for _, line := range strings.Split(answer, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "*") {
			listed++
			continue
		}
		if number, _, starts := stepHeading(trimmed); starts && number > 0 {
			listed++
		}
	}
	return listed > 1
}

// offer shows the user the folder that is about to be written and saves it only
// when they say yes.
func (store *Store) offer(ctx context.Context, name string, files map[string][]byte, from string) (bool, error) {
	if store.ask == nil {
		return false, fmt.Errorf("there is no screen to offer the skill %q on, so wire a channel before offering to save one", name)
	}
	if _, err := ParseFolder(files); err != nil {
		return false, err
	}

	answer, err := store.ask(ctx, contract.Preview{
		ID:    "save-skill-" + name,
		Title: fmt.Sprintf("Save %s as the skill %q?", from, name),
		Body:  string(files[DescriptionFile]) + "\n" + string(files[StepsFile]),
	})
	if err != nil {
		return false, fmt.Errorf("cannot offer to save the skill %q, so check the screen you are on: %w", name, err)
	}
	if answer.Answer == contract.AnswerReject {
		return false, nil
	}
	if err := store.Save(ctx, contract.SkillSavedByPerson, name, files); err != nil {
		return false, err
	}
	return true, nil
}

// oneLine cuts a piece of text down to the single line a description may be,
// falling back to the given words when there is nothing usable in it.
func oneLine(text string, fallback string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	first = strings.TrimSpace(first)
	if first == "" {
		return fallback
	}
	if runes := []rune(first); len(runes) > MaxDescriptionRunes {
		return string(runes[:MaxDescriptionRunes-3]) + "..."
	}
	return first
}
