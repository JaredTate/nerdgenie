package main

import (
	"context"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

func TestTheSkillsBoxHandsOnWhoIsSavingTheSkill(t *testing.T) {
	skills := testkit.NewFakeSkill()
	box := &skillsBox{}
	box.fill(skills)
	ctx := context.Background()

	if err := box.Save(ctx, contract.SkillSavedByModel, "written-by-the-model", map[string][]byte{
		"SKILL.md": []byte("# written-by-the-model\nWritten by the model.\n"),
	}); err != nil {
		t.Fatalf("saving through the box failed: %v", err)
	}
	if err := box.Save(ctx, contract.SkillSavedByPerson, "written-by-the-person", map[string][]byte{
		"SKILL.md": []byte("# written-by-the-person\nWritten by the person.\n"),
	}); err != nil {
		t.Fatalf("saving through the box failed: %v", err)
	}

	if source := skills.SourceOf("written-by-the-model"); source != contract.SkillSavedByModel {
		t.Errorf("the store was told %q saved the skill the model wrote, want the model", source)
	}
	if source := skills.SourceOf("written-by-the-person"); source != contract.SkillSavedByPerson {
		t.Errorf("the store was told %q saved the skill the person saved, want the person", source)
	}
}

func TestTheSkillsBoxAnswersThroughTheStoreOnceItIsFilled(t *testing.T) {
	skills := testkit.NewFakeSkill()
	skills.Add(contract.SkillSummary{Name: "weekly-note", Description: "write the weekly note"}, "# weekly-note\nwrite the weekly note\n", "weekly note")
	box := &skillsBox{}
	box.fill(skills)
	ctx := context.Background()

	listed, err := box.List(ctx)
	if err != nil || len(listed) != 1 || listed[0].Name != "weekly-note" {
		t.Fatalf("the box listed %+v, err %v, want the one skill the store holds", listed, err)
	}
	body, err := box.Load(ctx, "weekly-note")
	if err != nil || body == "" {
		t.Fatalf("the box loaded %q, err %v, want the skill's body", body, err)
	}
	if _, err := box.Run(ctx, "weekly-note", "for August"); err != nil {
		t.Fatalf("running through the box failed: %v", err)
	}
	if runs := skills.Runs(); len(runs) != 1 || runs[0] != "weekly-note" {
		t.Errorf("the store was told to run %v, want the weekly note once", runs)
	}
	match, err := box.Match(ctx, "please write the weekly note")
	if err != nil {
		t.Fatalf("matching through the box failed: %v", err)
	}
	if !match.Matched || match.Name != "weekly-note" {
		t.Errorf("the box matched %+v, want the weekly note fired", match)
	}
}

func TestTheSkillsBoxAnswersPlainlyWhileItIsEmpty(t *testing.T) {
	box := &skillsBox{}
	ctx := context.Background()

	if listed, err := box.List(ctx); err != nil || listed != nil {
		t.Errorf("an empty box listed %+v, err %v, want no skills and no error", listed, err)
	}
	if _, err := box.Load(ctx, "anything"); err == nil {
		t.Error("an empty box loaded a skill, and there are none until the store is open")
	}
	if _, err := box.Run(ctx, "anything", ""); err == nil {
		t.Error("an empty box ran a skill, and there are none until the store is open")
	}
	if match, err := box.Match(ctx, "anything at all"); err != nil || match.Matched {
		t.Errorf("an empty box matched %+v, err %v, want nothing fired", match, err)
	}
}

func TestTheSkillsBoxSaysSoWhileItIsEmpty(t *testing.T) {
	box := &skillsBox{}
	err := box.Save(context.Background(), contract.SkillSavedByPerson, "written-too-early", map[string][]byte{
		"SKILL.md": []byte("# written-too-early\nSaved before the store was open.\n"),
	})
	if err == nil {
		t.Fatal("a save through an empty box said nothing was wrong, and nothing was written")
	}
}
