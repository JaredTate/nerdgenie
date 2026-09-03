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

func TestTheSkillsBoxSaysSoWhileItIsEmpty(t *testing.T) {
	box := &skillsBox{}
	err := box.Save(context.Background(), contract.SkillSavedByPerson, "written-too-early", map[string][]byte{
		"SKILL.md": []byte("# written-too-early\nSaved before the store was open.\n"),
	})
	if err == nil {
		t.Fatal("a save through an empty box said nothing was wrong, and nothing was written")
	}
}
