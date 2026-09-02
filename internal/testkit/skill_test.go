package testkit_test

import (
	"context"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
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

	err := skills.Save(ctx, "check-the-blog", map[string][]byte{
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

func TestTheFakeSkillKeepsTheSkillContract(t *testing.T) {
	if err := testkit.CheckSkill(context.Background(), testkit.NewFakeSkill()); err != nil {
		t.Fatalf("the fake skill store does not keep the skill contract: %v", err)
	}
}
