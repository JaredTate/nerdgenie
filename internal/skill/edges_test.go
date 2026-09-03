package skill_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/skill"
)

func TestANameThatIsNotAFolderNameIsRefusedEverywhere(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	if _, err := built.store.Rollback(ctx, "Not A Name"); err == nil {
		t.Error("rolling back under a name that is not a folder name was allowed")
	}
	if _, err := built.store.Remove(ctx, "Not A Name"); err == nil {
		t.Error("removing under a name that is not a folder name was allowed")
	}
	if _, err := built.store.Changelog("Not A Name"); err == nil {
		t.Error("reading the changelog of a name that is not a folder name was allowed")
	}
	if _, err := built.store.Load(ctx, "Not A Name"); err == nil {
		t.Error("loading under a name that is not a folder name was allowed")
	}
	if _, err := built.store.Run(ctx, "Not A Name", ""); err == nil {
		t.Error("running under a name that is not a folder name was allowed")
	}
	if _, err := built.store.DryRun(ctx, "Not A Name"); err == nil {
		t.Error("dry running under a name that is not a folder name was allowed")
	}
}

func TestAFileWhereASkillFolderShouldBeIsNotASkill(t *testing.T) {
	built := newHarness(t)
	path := built.home.SkillFolder("a-file")
	if err := os.WriteFile(path, []byte("not a folder"), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the file standing in for a folder: %v", err)
	}
	if _, err := built.store.Load(context.Background(), "a-file"); err == nil {
		t.Error("a file where a skill folder should be loaded as a skill")
	}
}

func TestASkillWithNoChangelogSaysSo(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	if err := built.store.Save(ctx, contract.SkillSavedByPerson, "note", filesFor("note", "A skill whose changelog gets deleted.", "Do it.")); err != nil {
		t.Fatalf("the save failed: %v", err)
	}
	if err := os.Remove(filepath.Join(built.home.SkillFolder("note"), skill.ChangelogFile)); err != nil {
		t.Fatalf("cannot remove the changelog: %v", err)
	}
	if _, err := built.store.Changelog("note"); err == nil {
		t.Error("a skill with no changelog read one, and it must say there is none")
	}
	if _, err := runSlash(t, built, "show note"); err == nil {
		t.Error("showing a skill with no changelog was allowed, and loading is the strict path")
	}

	// Saving it again writes the changelog back, and then it shows.
	if err := built.store.Save(ctx, contract.SkillSavedByPerson, "note", filesFor("note", "A skill whose changelog came back.", "Do it.")); err != nil {
		t.Fatalf("the second save failed: %v", err)
	}
	said, err := runSlash(t, built, "show note")
	if err != nil {
		t.Fatalf("showing the skill failed: %v", err)
	}
	if !strings.Contains(said, "Do it.") || !strings.Contains(said, "changelog for note") {
		t.Errorf("showing said %q, want the body and the changelog", said)
	}
}

func TestASkillCanOnlyBeRemovedSoManyTimes(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	for round := range skill.MaxVersions {
		if err := built.store.Save(ctx, contract.SkillSavedByPerson, "note", filesFor("note", "A skill saved and removed over and over.", "Do it.")); err != nil {
			t.Fatalf("save %d failed: %v", round+1, err)
		}
		if _, err := built.store.Remove(ctx, "note"); err != nil {
			t.Fatalf("removal %d failed: %v", round+1, err)
		}
	}
	if err := built.store.Save(ctx, contract.SkillSavedByPerson, "note", filesFor("note", "The save that cannot be removed.", "Do it.")); err != nil {
		t.Fatalf("the last save failed: %v", err)
	}
	_, err := built.store.Remove(ctx, "note")
	if err == nil || !strings.Contains(err.Error(), "no free name left") {
		t.Errorf("removing past the cap gave %v, want an error saying there is no free name left", err)
	}
}

func TestASkillWithNoTriggersNeverFiresFromAMessage(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	if err := built.store.Save(ctx, contract.SkillSavedByPerson, "quiet", filesFor("quiet", "A skill with no trigger words at all.", "Do it.")); err != nil {
		t.Fatalf("the save failed: %v", err)
	}
	matched, err := built.store.Match(ctx, "quiet please do it")
	if err != nil {
		t.Fatalf("matching failed: %v", err)
	}
	if matched.Matched {
		t.Errorf("the match is %+v, want nothing, because the skill names no triggers", matched)
	}
}

func TestAStepWhoseAddressIsNotAnAddressIsStillChecked(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	files := map[string][]byte{
		skill.DescriptionFile: []byte("# odd-address\n\nGoes to addresses written in odd ways.\n\n## Permissions\n\n- site: news.example.com\n"),
		skill.StepsFile: []byte(
			"1. Go somewhere named by a bare host.\n   tool: web\n   input: {\"url\": \"news.example.com\"}\n\n" +
				"2. Go somewhere named by something that is not text.\n   tool: web\n   input: {\"url\": 4}\n"),
	}
	if err := built.store.Save(ctx, contract.SkillSavedByPerson, "odd-address", files); err != nil {
		t.Fatalf("the save failed: %v", err)
	}
	_, err := built.store.Run(ctx, "odd-address", "")
	if err == nil || !strings.Contains(err.Error(), "\"web\"") {
		t.Errorf("running the skill gave %v, want it to reach the step and name the missing tool", err)
	}
	if previews := built.channel.Previews(); len(previews) != 0 {
		t.Errorf("the user was asked about %v, want nothing, because a bare host is the site the block names", previews)
	}
}

func TestATestFileKeepsItsExpectationWhenItIsWrittenOut(t *testing.T) {
	written := skill.RenderTestFile("note", skill.DryRunPlan{Arguments: "today", Expect: "rain"})
	plan, err := skill.ParseTestFile(written)
	if err != nil {
		t.Fatalf("the test file did not parse: %v", err)
	}
	if plan.Arguments != "today" || plan.Expect != "rain" {
		t.Errorf("the dry run read back as %+v, want what was written out", plan)
	}
}

func TestAToolThatFailsNamesTheStepItFailedIn(t *testing.T) {
	built := newHarness(t, &failingTool{name: "echo"})
	ctx := context.Background()
	if err := built.store.Save(ctx, contract.SkillSavedByPerson, "say-two", twoStepSkill("say-two")); err != nil {
		t.Fatalf("the save failed: %v", err)
	}
	_, err := built.store.Run(ctx, "say-two", "")
	if err == nil || !strings.Contains(err.Error(), "step 1") {
		t.Errorf("a tool that failed gave %v, want an error naming the step it failed in", err)
	}
}

func TestTheReportStopsGrowingAtItsCap(t *testing.T) {
	long := strings.Repeat("z", skill.MaxResultRunes)
	built := newHarness(t, &echoTool{name: "echo"})
	ctx := context.Background()
	steps := strings.Builder{}
	for number := range skill.MaxSteps {
		fmt.Fprintf(&steps, "%d. Say a long thing.\n   tool: echo\n   input: {\"say\": \"%s\"}\n\n", number+1, long)
	}
	files := map[string][]byte{
		skill.DescriptionFile: []byte("# many-steps\n\nSays a long thing many times over.\n"),
		skill.StepsFile:       []byte(steps.String()),
	}
	if err := built.store.Save(ctx, contract.SkillSavedByPerson, "many-steps", files); err != nil {
		t.Fatalf("the save failed: %v", err)
	}
	report, err := built.store.Run(ctx, "many-steps", "")
	if err != nil {
		t.Fatalf("running the skill failed: %v", err)
	}
	if len(report) > skill.MaxReportBytes {
		t.Errorf("the report is %d bytes, over the cap of %d", len(report), skill.MaxReportBytes)
	}
}
