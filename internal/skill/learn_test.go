package skill_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/skill"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// documentationPage is what a command-line tool's own page looks like: prose
// with its commands shown in fenced blocks and after a shell prompt, and the
// same command shown twice with different arguments.
const documentationPage = "# The paperclip tool\n\n" +
	"Paperclip files things away. To see what it can do:\n\n" +
	"```\npaperclip help\n```\n\n" +
	"To file one thing away:\n\n" +
	"$ paperclip file notes.md\n\n" +
	"To file another:\n\n" +
	"$ paperclip file letters.md\n\n" +
	"And to see what has been filed:\n\n" +
	"```\npaperclip list --all\n```\n"

func TestAPageOfDocumentationBecomesASkillThatPassesItsDryRun(t *testing.T) {
	shell := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolShell}, "helped", "filed", "listed")
	built := newHarness(t, shell)
	ctx := context.Background()

	summary, err := built.store.LearnFromPage(ctx, "paperclip", documentationPage)
	if err != nil {
		t.Fatalf("learning from the page failed: %v", err)
	}
	if summary.Name != "paperclip" || summary.Description == "" {
		t.Errorf("the summary is %+v, want the name and a one-liner", summary)
	}

	steps := built.readSkillFile(t, "paperclip", skill.StepsFile)
	for _, command := range []string{"paperclip help", "paperclip file notes.md", "paperclip list --all"} {
		if !strings.Contains(steps, command) {
			t.Errorf("the steps are %q, want the command %q the page showed", steps, command)
		}
	}
	if strings.Contains(steps, "letters.md") {
		t.Errorf("the steps are %q, want one example of each command rather than both", steps)
	}
	if inputs := shell.Inputs(); len(inputs) != 3 {
		t.Errorf("the dry run ran %d commands, want one per step", len(inputs))
	}
}

func TestAPageWhoseCommandsFailSavesNothing(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	if _, err := built.store.LearnFromPage(ctx, "paperclip", documentationPage); err == nil {
		t.Fatal("the skill was learned, and there was no shell tool to run its dry run")
	}
	if listed, err := built.store.List(ctx); err != nil || len(listed) != 0 {
		t.Errorf("the listing is %v with error %v, want nothing saved after a failed dry run", listed, err)
	}
}

func TestAPageWithNoCommandsIsRefused(t *testing.T) {
	built := newHarness(t)
	_, err := built.store.LearnFromPage(context.Background(), "paperclip", "Some prose with no commands in it at all.")
	if err == nil || !strings.Contains(err.Error(), "no commands") {
		t.Errorf("learning from a page with no commands gave %v, want it to say what the page was missing", err)
	}
}

func TestLearnFromPageRefusesANameThatIsNotAFolderName(t *testing.T) {
	built := newHarness(t)
	if _, err := built.store.LearnFromPage(context.Background(), "Paper Clip", documentationPage); err == nil {
		t.Error("a name with a space in it was allowed, and it must be refused")
	}
}

// finishedTask is a task record with a plan whose steps are done, which is what
// the offer to save a skill is built out of.
func finishedTask() contract.Record {
	return contract.Record{
		Goal: contract.Goal{Ask: "File the week's notes away and say what was filed."},
		Work: contract.Work{Plan: []contract.PlanStep{
			{Number: 1, Text: "Read the notes folder.", Done: true, ResultID: "r1"},
			{Number: 2, Text: "File each note under its date.", Done: true, ResultID: "r2"},
			{Number: 3, Text: "Say what was filed.", Done: false},
		}},
	}
}

func TestAFinishedTaskIsOfferedAndApprovalSavesAValidFolder(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	built.channel.AnswerPreviewsWith(contract.AnswerOnce)

	saved, err := built.store.OfferFromTask(ctx, "file-notes", finishedTask())
	if err != nil {
		t.Fatalf("offering the task as a skill failed: %v", err)
	}
	if !saved {
		t.Fatal("the offer was approved and nothing was saved")
	}

	previews := built.channel.Previews()
	if len(previews) != 1 || !strings.Contains(previews[0].Body, "File each note under its date.") {
		t.Fatalf("the previews are %v, want one showing the steps about to be saved", previews)
	}
	if _, err := built.store.Load(ctx, "file-notes"); err != nil {
		t.Errorf("the saved skill did not load, so the folder was not valid: %v", err)
	}
	steps := built.readSkillFile(t, "file-notes", skill.StepsFile)
	if strings.Contains(steps, "Say what was filed.") {
		t.Errorf("the steps are %q, want only the steps the task finished", steps)
	}
	if description := built.readSkillFile(t, "file-notes", skill.DescriptionFile); !strings.Contains(description, "File the week's notes away") {
		t.Errorf("the description file is %q, want the task's own ask as the one-liner", description)
	}
}

func TestARefusedOfferSavesNothing(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	built.channel.AnswerPreviewsWith(contract.AnswerReject)

	saved, err := built.store.OfferFromTask(ctx, "file-notes", finishedTask())
	if err != nil {
		t.Fatalf("offering the task as a skill failed: %v", err)
	}
	if saved {
		t.Fatal("the offer was refused and something was saved anyway")
	}
	if listed, err := built.store.List(ctx); err != nil || len(listed) != 0 {
		t.Errorf("the listing is %v with error %v, want nothing saved after a refusal", listed, err)
	}
}

func TestATaskWithNoFinishedStepsIsNotOffered(t *testing.T) {
	built := newHarness(t)
	if _, err := built.store.OfferFromTask(context.Background(), "file-notes", contract.Record{}); err == nil {
		t.Error("a task with no finished steps was offered, and there is no procedure in it")
	}
}

func TestAnOfferWithNoScreenSaysSo(t *testing.T) {
	built := newHarness(t)
	quiet, err := skill.New(skill.Options{Home: built.home, Clock: built.clock, Tools: built.tools, Permission: built.permission})
	if err != nil {
		t.Fatalf("cannot build a store with no screen: %v", err)
	}
	if _, err := quiet.OfferFromTask(context.Background(), "file-notes", finishedTask()); err == nil {
		t.Error("the offer was made with no screen to make it on")
	}
}

func TestTheFourthAnswerOfAReviewIsOfferedWhenItIsAProcedure(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	built.channel.AnswerPreviewsWith(contract.AnswerAlways)

	answer := "First read the folder; then file each note under its date; finally say what was filed."
	saved, err := built.store.OfferFromReview(ctx, "file-notes", answer)
	if err != nil {
		t.Fatalf("offering the review's answer failed: %v", err)
	}
	if !saved {
		t.Fatal("the offer was approved and nothing was saved")
	}
	steps := built.readSkillFile(t, "file-notes", skill.StepsFile)
	if !strings.Contains(steps, "then file each note under its date") {
		t.Errorf("the steps are %q, want the answer split into its steps", steps)
	}
}

func TestAFourthAnswerThatIsAFactIsNotOffered(t *testing.T) {
	built := newHarness(t)
	saved, err := built.store.OfferFromReview(context.Background(), "a-fact", "The notes folder is on the second disk.")
	if err != nil {
		t.Fatalf("offering a fact failed: %v", err)
	}
	if saved {
		t.Error("a fact was saved as a skill, and a fact belongs in memory")
	}
	if previews := built.channel.Previews(); len(previews) != 0 {
		t.Errorf("the user was shown %v, want nothing, because a fact is not a procedure", previews)
	}
}

func TestDescribesAProcedureTellsAProcedureFromAFact(t *testing.T) {
	procedures := []string{
		"First read the folder, then file each note.",
		"1. Read the folder.\n2. File each note.",
		"- Read the folder.\n- File each note.",
		"Step one is to read the folder.",
	}
	for _, answer := range procedures {
		if !skill.DescribesAProcedure(answer) {
			t.Errorf("%q was read as a fact, want a procedure", answer)
		}
	}
	facts := []string{
		"The notes folder is on the second disk.",
		"",
		"- The notes folder is on the second disk.",
	}
	for _, answer := range facts {
		if skill.DescribesAProcedure(answer) {
			t.Errorf("%q was read as a procedure, want a fact", answer)
		}
	}
}

func TestOfferFromReviewRefusesANameThatIsNotAFolderName(t *testing.T) {
	built := newHarness(t)
	if _, err := built.store.OfferFromReview(context.Background(), "Not A Name", "First this, then that."); err == nil {
		t.Error("a name with spaces in it was allowed, and it must be refused")
	}
}
