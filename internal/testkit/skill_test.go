package testkit_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

func TestTheFakeSkillListsNamesAndOneLinersAndLoadsBodiesOnUse(t *testing.T) {
	ctx := context.Background()
	skills := testkit.NewFakeSkill()
	skills.Add(contract.SkillSummary{Name: "post-to-x", Description: "Posts one message to X."}, "open x.com, click compose, type, post", "post to x")

	listed, err := skills.List(ctx)
	if err != nil {
		t.Fatalf("listing the skills failed: %v", err)
	}
	if len(listed) != 1 || listed[0].Name != "post-to-x" {
		t.Fatalf("the skills listed as %+v, want the one that was added", listed)
	}

	body, err := skills.Load(ctx, "post-to-x")
	if err != nil {
		t.Fatalf("loading the skill body failed: %v", err)
	}
	if body == "" {
		t.Error("the skill body is empty, and it should be the steps that were added")
	}
	if _, err := skills.Load(ctx, "nothing"); err == nil {
		t.Error("loading a skill that is not there was reported as a success, want an error naming it")
	}
}

func TestTheFakeSkillMatchesItsTriggerWordsAndRecordsWhatWasRun(t *testing.T) {
	ctx := context.Background()
	skills := testkit.NewFakeSkill()
	skills.Add(contract.SkillSummary{Name: "post-to-x", Description: "Posts one message to X."}, "the steps", "post to x")

	matched, err := skills.Match(ctx, "please post to x for me")
	if err != nil {
		t.Fatalf("matching failed: %v", err)
	}
	if !matched.Matched || matched.Name != "post-to-x" {
		t.Errorf("the matcher returned %+v, want a match on post-to-x", matched)
	}

	missed, err := skills.Match(ctx, "what is the weather")
	if err != nil {
		t.Fatalf("matching failed: %v", err)
	}
	if missed.Matched {
		t.Errorf("the matcher matched %q on a message with no trigger in it", missed.Name)
	}

	if _, err := skills.Run(ctx, "post-to-x", ""); err != nil {
		t.Fatalf("running the skill failed: %v", err)
	}
	if runs := skills.Runs(); len(runs) != 1 || runs[0] != "post-to-x" {
		t.Errorf("the fake recorded the runs %v, want one run of post-to-x", runs)
	}
}

func TestTheFakeSkillSavesAFolderAndThenListsIt(t *testing.T) {
	ctx := context.Background()
	skills := testkit.NewFakeSkill()

	err := skills.Save(ctx, contract.SkillSavedByPerson, "check-the-blog", map[string][]byte{
		"SKILL.md": []byte("# check-the-blog\nChecks the blog is up.\n"),
	})
	if err != nil {
		t.Fatalf("saving a skill folder failed: %v", err)
	}

	listed, err := skills.List(ctx)
	if err != nil {
		t.Fatalf("listing the skills failed: %v", err)
	}
	if len(listed) != 1 || listed[0].Name != "check-the-blog" {
		t.Errorf("the skills listed as %+v, want the one that was saved", listed)
	}
	if files := skills.Files("check-the-blog"); len(files) != 1 {
		t.Errorf("the saved skill holds %d files, want the one that was saved", len(files))
	}
}

func TestSavingOverASkillKeepsItsBodyAndItsTriggerWords(t *testing.T) {
	ctx := context.Background()
	skills := testkit.NewFakeSkill()
	skills.Add(contract.SkillSummary{Name: "post-to-x", Description: "Posts one message to X."},
		"open x.com, click compose, type, post", "post to x")

	err := skills.Save(ctx, contract.SkillSavedByPerson, "post-to-x", map[string][]byte{
		"SKILL.md": []byte("# post-to-x\nPosts one message to X.\n\nOpen x.com, click compose, type, post.\n"),
	})

	if err != nil {
		t.Fatalf("saving over an existing skill failed: %v", err)
	}
	body, err := skills.Load(ctx, "post-to-x")
	if err != nil {
		t.Fatalf("loading the skill after the save failed: %v", err)
	}
	if !strings.Contains(body, "click compose") {
		t.Errorf("the skill body after the save is %q, want what the SKILL.md holds", body)
	}
	matched, err := skills.Match(ctx, "please post to x for me")
	if err != nil {
		t.Fatalf("matching after the save failed: %v", err)
	}
	if !matched.Matched || matched.Name != "post-to-x" {
		t.Errorf("the matcher returned %+v after the save, and saving must not throw the trigger words away", matched)
	}
}

func TestTheDescriptionIsTheLineUnderTheHeadingNotTheHeading(t *testing.T) {
	ctx := context.Background()
	skills := testkit.NewFakeSkill()

	err := skills.Save(ctx, contract.SkillSavedByPerson, "post-to-x", map[string][]byte{
		"SKILL.md": []byte("# post-to-x\nPosts one message to X.\n"),
	})

	if err != nil {
		t.Fatalf("saving the skill failed: %v", err)
	}
	listed, err := skills.List(ctx)
	if err != nil {
		t.Fatalf("listing the skills failed: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("the skills listed as %+v, want the one that was saved", listed)
	}
	if listed[0].Description != "Posts one message to X." {
		t.Errorf("the description is %q, and the heading is the folder name rather than a description", listed[0].Description)
	}
}

func TestTheFakeSkillKeepsTheSkillContract(t *testing.T) {
	if err := testkit.CheckSkill(context.Background(), testkit.NewFakeSkill()); err != nil {
		t.Fatalf("the fake skill store does not keep the skill contract: %v", err)
	}
}

func TestTheFakeSkillRemembersWhoSavedEachSkill(t *testing.T) {
	ctx := context.Background()
	skills := testkit.NewFakeSkill()

	if err := skills.Save(ctx, contract.SkillSavedByModel, "written-by-the-model", map[string][]byte{
		"SKILL.md": []byte("# written-by-the-model\nWritten by the model.\n"),
	}); err != nil {
		t.Fatalf("saving the skill the model wrote failed: %v", err)
	}
	if err := skills.Save(ctx, contract.SkillSavedByPerson, "written-by-the-person", map[string][]byte{
		"SKILL.md": []byte("# written-by-the-person\nWritten by the person.\n"),
	}); err != nil {
		t.Fatalf("saving the skill the person saved failed: %v", err)
	}

	if source := skills.SourceOf("written-by-the-model"); source != contract.SkillSavedByModel {
		t.Errorf("the fake says %q saved the skill the model wrote, want the model", source)
	}
	if source := skills.SourceOf("written-by-the-person"); source != contract.SkillSavedByPerson {
		t.Errorf("the fake says %q saved the skill the person saved, want the person", source)
	}
}

func TestTheFakeSkillRefusesASaveWithNobodySavingIt(t *testing.T) {
	err := testkit.NewFakeSkill().Save(context.Background(), contract.SkillSource("nobody"), "written-by-nobody", map[string][]byte{
		"SKILL.md": []byte("# written-by-nobody\nWritten by nobody at all.\n"),
	})
	if err == nil {
		t.Fatal("the fake saved a skill nobody saved, and the real store refuses one, so a test against the fake would be told the wrong thing")
	}
}
