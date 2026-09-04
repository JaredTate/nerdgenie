package skill_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/skill"
)

// filesFor is one small skill folder whose description says what the test made
// it for, which is how a test tells one saved version from the next.
func filesFor(name string, description string, intent string) map[string][]byte {
	return map[string][]byte{
		skill.DescriptionFile: []byte("# " + name + "\n\n" + description + "\n"),
		skill.StepsFile:       []byte("1. " + intent + "\n"),
	}
}

func TestSaveCompletesAFolderThatOnlyGaveTheDescription(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	files := map[string][]byte{skill.DescriptionFile: []byte("# bare\n\nA skill saved with nothing but its description.\n")}
	if err := built.store.Save(ctx, contract.SkillSavedByPerson, "bare", files); err != nil {
		t.Fatalf("saving a folder with only a description failed: %v", err)
	}

	for _, file := range []string{skill.DescriptionFile, skill.StepsFile, skill.TestFile, skill.ChangelogFile} {
		if built.readSkillFile(t, "bare", file) == "" && file != skill.StepsFile {
			t.Errorf("the saved folder's %s is empty, want the file Save fills in", file)
		}
	}
	listed, err := built.store.List(ctx)
	if err != nil || len(listed) != 1 || listed[0].Name != "bare" {
		t.Fatalf("the listing is %v with error %v, want the skill that was just saved", listed, err)
	}
	if listed[0].Description != "A skill saved with nothing but its description." {
		t.Errorf("the one-liner is %q, want the line from the description file", listed[0].Description)
	}
}

func TestSaveRefusesWhatItCannotWrite(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	cases := []struct {
		what  string
		name  string
		files map[string][]byte
	}{
		{"no name", "", filesFor("x", "A skill with no folder name at all.", "Do it.")},
		{"no files", "empty", nil},
		{"a file name that leaves the folder", "escape", map[string][]byte{"../out.md": []byte("no")}},
		{"a heading naming another skill", "mine", filesFor("yours", "A skill whose heading names another.", "Do it.")},
		{"a description file that says nothing", "quiet", map[string][]byte{skill.DescriptionFile: []byte("# quiet\n")}},
	}
	for _, test := range cases {
		if err := built.store.Save(ctx, contract.SkillSavedByPerson, test.name, test.files); err == nil {
			t.Errorf("saving with %s was allowed, and it must be refused", test.what)
		}
	}
}

func TestASecondSaveKeepsTheCopyItReplaced(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	if err := built.store.Save(ctx, contract.SkillSavedByPerson, "note", filesFor("note", "The first description of this skill.", "Do the first thing.")); err != nil {
		t.Fatalf("the first save failed: %v", err)
	}
	if err := built.store.Save(ctx, contract.SkillSavedByPerson, "note", filesFor("note", "The second description of this skill.", "Do the second thing.")); err != nil {
		t.Fatalf("the second save failed: %v", err)
	}

	kept := filepath.Join(built.home.SkillFolder("note"), skill.VersionsFolder, "1", skill.DescriptionFile)
	content, err := os.ReadFile(kept)
	if err != nil {
		t.Fatalf("the copy the second save replaced was not kept: %v", err)
	}
	if !strings.Contains(string(content), "The first description") {
		t.Errorf("version one says %q, want the first description", content)
	}
	if now := built.readSkillFile(t, "note", skill.DescriptionFile); !strings.Contains(now, "The second description") {
		t.Errorf("the skill now says %q, want the second description", now)
	}
}

func TestRollbackRestoresThePreviousVersionAndTheChangelogRecordsBoth(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	if err := built.store.Save(ctx, contract.SkillSavedByPerson, "note", filesFor("note", "The first description of this skill.", "Do the first thing.")); err != nil {
		t.Fatalf("the first save failed: %v", err)
	}
	if err := built.store.Save(ctx, contract.SkillSavedByPerson, "note", filesFor("note", "The second description of this skill.", "Do the second thing.")); err != nil {
		t.Fatalf("the second save failed: %v", err)
	}

	said, err := built.store.Rollback(ctx, "note")
	if err != nil {
		t.Fatalf("the rollback failed: %v", err)
	}
	if !strings.Contains(said, "version 1") {
		t.Errorf("the rollback said %q, want it to name the version it went back to", said)
	}
	if now := built.readSkillFile(t, "note", skill.DescriptionFile); !strings.Contains(now, "The first description") {
		t.Errorf("the skill now says %q, want the first description back", now)
	}

	changelog := built.readSkillFile(t, "note", skill.ChangelogFile)
	if strings.Count(changelog, "saved") != 2 {
		t.Errorf("the changelog is %q, want both saves recorded", changelog)
	}
	if !strings.Contains(changelog, "rolled back to version 1, keeping what was there as version 2") {
		t.Errorf("the changelog is %q, want the rollback recorded with both versions", changelog)
	}
	if !strings.Contains(changelog, startOfTheTests.Format("2006-01-02")) {
		t.Errorf("the changelog is %q, want the date from the clock", changelog)
	}
}

func TestARollbackCanItselfBeRolledBack(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	for _, description := range []string{"The first description of this skill.", "The second description of this skill."} {
		if err := built.store.Save(ctx, contract.SkillSavedByPerson, "note", filesFor("note", description, "Do the thing.")); err != nil {
			t.Fatalf("the save failed: %v", err)
		}
	}
	if _, err := built.store.Rollback(ctx, "note"); err != nil {
		t.Fatalf("the first rollback failed: %v", err)
	}
	if _, err := built.store.Rollback(ctx, "note"); err != nil {
		t.Fatalf("the second rollback failed: %v", err)
	}
	if now := built.readSkillFile(t, "note", skill.DescriptionFile); !strings.Contains(now, "The second description") {
		t.Errorf("the skill now says %q, want the second description back again", now)
	}
}

func TestRollbackRefusesWhatItCannotDo(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	if _, err := built.store.Rollback(ctx, "no-such-skill"); err == nil {
		t.Error("rolling back a skill that is not there was allowed, and it must be refused")
	}
	if err := built.store.Save(ctx, contract.SkillSavedByPerson, "once", filesFor("once", "A skill saved only one time ever.", "Do it.")); err != nil {
		t.Fatalf("the save failed: %v", err)
	}
	_, err := built.store.Rollback(ctx, "once")
	if err == nil || !strings.Contains(err.Error(), "nothing to roll back to") {
		t.Errorf("rolling back a skill with no earlier copy gave %v, want an error saying there is nothing to go back to", err)
	}
}

func TestRemoveKeepsTheFolderUnderARemovedName(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	if err := built.store.Save(ctx, contract.SkillSavedByPerson, "note", filesFor("note", "A skill about to be removed here.", "Do it.")); err != nil {
		t.Fatalf("the save failed: %v", err)
	}

	said, err := built.store.Remove(ctx, "note")
	if err != nil {
		t.Fatalf("removing the skill failed: %v", err)
	}
	if !strings.Contains(said, ".removed") {
		t.Errorf("removing said %q, want it to name where the folder was kept", said)
	}
	if _, err := os.Stat(built.home.SkillFolder("note") + ".removed"); err != nil {
		t.Errorf("the removed folder is not there: %v", err)
	}
	listed, err := built.store.List(ctx)
	if err != nil || len(listed) != 0 {
		t.Errorf("the listing is %v with error %v, want nothing, because a removed skill is not listed", listed, err)
	}
}

func TestRemovingASkillTwiceKeepsBothFolders(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	for range 2 {
		if err := built.store.Save(ctx, contract.SkillSavedByPerson, "note", filesFor("note", "A skill saved and removed twice.", "Do it.")); err != nil {
			t.Fatalf("the save failed: %v", err)
		}
		if _, err := built.store.Remove(ctx, "note"); err != nil {
			t.Fatalf("removing the skill failed: %v", err)
		}
	}
	for _, suffix := range []string{".removed", ".removed-2"} {
		if _, err := os.Stat(built.home.SkillFolder("note") + suffix); err != nil {
			t.Errorf("the folder %s is not there: %v", suffix, err)
		}
	}
}

func TestASaveThatSwapsStepsForAScriptLeavesNoStepsBehind(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	if err := built.store.Save(ctx, contract.SkillSavedByPerson, "swap", filesFor("swap", "A skill that starts out with steps.", "Do it.")); err != nil {
		t.Fatalf("the first save failed: %v", err)
	}
	files := map[string][]byte{
		skill.DescriptionFile: []byte("# swap\n\nA skill that ends up with a script.\n"),
		skill.ScriptFile:      []byte("#!/bin/sh\necho hello\n"),
	}
	if err := built.store.Save(ctx, contract.SkillSavedByPerson, "swap", files); err != nil {
		t.Fatalf("the second save failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(built.home.SkillFolder("swap"), skill.StepsFile)); !os.IsNotExist(err) {
		t.Errorf("the old steps file is still there with error %v, want it taken away", err)
	}
	if _, err := built.store.Load(ctx, "swap"); err != nil {
		t.Errorf("the skill with a script did not load: %v", err)
	}
}

func TestChangelogNamesASkillThatIsNotThere(t *testing.T) {
	built := newHarness(t)
	if _, err := built.store.Changelog("no-such-skill"); err == nil {
		t.Error("the changelog of a skill that is not there was read, and it must be refused")
	}
}
