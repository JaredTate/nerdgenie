package skill_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/skill"
)

// runSlash runs the "/skills" command the way the command registry would.
func runSlash(t *testing.T, built *harness, arguments string) (string, error) {
	t.Helper()
	command := built.store.Command()
	if command.Name != "skills" || command.Help == "" || command.Run == nil {
		t.Fatalf("the command is %+v, want a name, a help line, and something to run", command)
	}
	return command.Run(context.Background(), arguments, contract.CommandContext{Channel: built.channel})
}

func TestTheSkillsCommandListsWhatThereIs(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()

	said, err := runSlash(t, built, "")
	if err != nil {
		t.Fatalf("listing failed: %v", err)
	}
	if !strings.Contains(said, "no skills yet") {
		t.Errorf("the listing is %q, want it to say there are none and how one is made", said)
	}

	if err := built.store.Save(ctx, contract.SkillSavedByPerson, "note", filesFor("note", "Files a note away under its date.", "Do it.")); err != nil {
		t.Fatalf("saving the skill failed: %v", err)
	}
	said, err = runSlash(t, built, "")
	if err != nil {
		t.Fatalf("listing failed: %v", err)
	}
	if !strings.Contains(said, "note: Files a note away under its date.") {
		t.Errorf("the listing is %q, want the skill's name and one-liner", said)
	}
}

func TestTheSkillsCommandShowsOneSkillWithItsChangelog(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	if err := built.store.Save(ctx, contract.SkillSavedByPerson, "note", filesFor("note", "Files a note away under its date.", "Do the one thing.")); err != nil {
		t.Fatalf("saving the skill failed: %v", err)
	}

	said, err := runSlash(t, built, "show note")
	if err != nil {
		t.Fatalf("showing the skill failed: %v", err)
	}
	if !strings.Contains(said, "Do the one thing.") || !strings.Contains(said, "saved for the first time") {
		t.Errorf("showing the skill said %q, want its body and its changelog", said)
	}
	if _, err := runSlash(t, built, "show"); err == nil {
		t.Error("showing with no name was allowed, and it must say which skill it needs")
	}
	if _, err := runSlash(t, built, "show no-such-skill"); err == nil {
		t.Error("showing a skill that is not there was allowed, and it must be refused")
	}
}

func TestTheSkillsCommandRunsOneSkill(t *testing.T) {
	built := newHarness(t, &echoTool{name: "echo"})
	ctx := context.Background()
	if err := built.store.Save(ctx, contract.SkillSavedByPerson, "say-two", twoStepSkill("say-two")); err != nil {
		t.Fatalf("saving the skill failed: %v", err)
	}

	said, err := runSlash(t, built, "run say-two the news")
	if err != nil {
		t.Fatalf("running the skill failed: %v", err)
	}
	if !strings.Contains(said, "you asked for the news") {
		t.Errorf("running the skill said %q, want the report with the arguments in it", said)
	}
	if _, err := runSlash(t, built, "run"); err == nil {
		t.Error("running with no name was allowed, and it must say which skill it needs")
	}
}

func TestTheSkillsCommandRollsBackAndRemoves(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	for _, description := range []string{"The first description of this skill.", "The second description of this skill."} {
		if err := built.store.Save(ctx, contract.SkillSavedByPerson, "note", filesFor("note", description, "Do it.")); err != nil {
			t.Fatalf("saving the skill failed: %v", err)
		}
	}

	said, err := runSlash(t, built, "rollback note")
	if err != nil {
		t.Fatalf("rolling back failed: %v", err)
	}
	if !strings.Contains(said, "version 1") {
		t.Errorf("rolling back said %q, want the version it went back to", said)
	}

	said, err = runSlash(t, built, "remove note")
	if err != nil {
		t.Fatalf("removing failed: %v", err)
	}
	if !strings.Contains(said, ".removed") {
		t.Errorf("removing said %q, want where the folder was kept", said)
	}
}

func TestTheSkillsCommandSaysWhatItDoesNotUnderstand(t *testing.T) {
	built := newHarness(t)
	_, err := runSlash(t, built, "frobnicate note")
	if err == nil || !strings.Contains(err.Error(), "frobnicate") {
		t.Errorf("an unknown word gave %v, want an error naming it and the words that work", err)
	}
}

func TestTheSkillsCommandSaysWhenARunHadNothingToReport(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	files := map[string][]byte{
		skill.DescriptionFile: []byte("# quiet\n\nA skill whose dry run stops at once.\n\n## Permissions\n\n- irreversible step: 1\n"),
		skill.StepsFile:       []byte("1. Do the thing that cannot be undone.\n   tool: echo\n   input: {\"say\": \"done\"}\n"),
	}
	if err := built.store.Save(ctx, contract.SkillSavedByPerson, "quiet", files); err != nil {
		t.Fatalf("saving the skill failed: %v", err)
	}
	built.channel.AnswerPreviewsWith(contract.AnswerReject)
	if _, err := runSlash(t, built, "run quiet"); err == nil {
		t.Error("the refused step ran, and a refusal stops the skill")
	}
}
