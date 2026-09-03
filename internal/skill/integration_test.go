//go:build integration

// This file is the integration test for skills: the real clock, the real
// permission function, and a real skills folder on a real disk under a
// temporary home. Nothing here is a fake except the tools, because a tool that
// really ran a program would be testing the sandbox rather than the skills.

package skill_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/clock"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/permission"
	"github.com/JaredTate/coeus/internal/skill"
	"github.com/JaredTate/coeus/internal/testkit"
)

func TestASkillGoesThroughItsWholeLifeOnARealDisk(t *testing.T) {
	home := testkit.NewTempHome(t)
	realClock := clock.System()
	decider, err := permission.New(contract.DefaultConfig(), realClock)
	if err != nil {
		t.Fatalf("cannot build the permission function: %v", err)
	}
	channel := testkit.NewFakeChannel("terminal")
	echo := &echoTool{name: "echo"}
	store, err := skill.New(skill.Options{
		Home:       home,
		Clock:      realClock,
		Tools:      testkit.NewFakeToolRegistry(echo),
		Permission: decider,
		Standing:   decider,
		Ask:        channel.ShowPreview,
	})
	if err != nil {
		t.Fatalf("cannot build the skill store: %v", err)
	}

	ctx := context.Background()
	if err := store.Save(ctx, contract.SkillSavedByPerson, "say-two", twoStepSkill("say-two")); err != nil {
		t.Fatalf("saving the skill failed: %v", err)
	}
	if err := store.Save(ctx, contract.SkillSavedByPerson, "say-two", filesFor("say-two", "A second version of the same skill.", "Do nothing at all.")); err != nil {
		t.Fatalf("the second save failed: %v", err)
	}
	if _, err := store.Rollback(ctx, "say-two"); err != nil {
		t.Fatalf("the rollback failed: %v", err)
	}

	report, err := store.Run(ctx, "say-two", "the news")
	if err != nil {
		t.Fatalf("running the restored skill failed: %v\n%s", err, report)
	}
	if !strings.Contains(report, "you asked for the news") {
		t.Errorf("the report is %q, want the restored steps to have run", report)
	}
	if _, err := store.Remove(ctx, "say-two"); err != nil {
		t.Fatalf("removing the skill failed: %v", err)
	}
	if listed, err := store.List(ctx); err != nil || len(listed) != 0 {
		t.Errorf("the listing is %v with error %v, want nothing after the skill was removed", listed, err)
	}
}

func TestTheFilesASkillWritesHaveTheModesTheLayoutCallsFor(t *testing.T) {
	home := testkit.NewTempHome(t)
	store, err := skill.New(skill.Options{
		Home:       home,
		Clock:      clock.System(),
		Tools:      testkit.NewFakeToolRegistry(),
		Permission: testkit.NewFakePermission(contract.RulingAllow),
	})
	if err != nil {
		t.Fatalf("cannot build the skill store: %v", err)
	}

	files := map[string][]byte{
		skill.DescriptionFile: []byte("# scripted\n\nA skill whose procedure is one executable.\n"),
		skill.ScriptFile:      []byte("#!/bin/sh\necho hello\n"),
	}
	if err := store.Save(context.Background(), contract.SkillSavedByPerson, "scripted", files); err != nil {
		t.Fatalf("saving the skill failed: %v", err)
	}

	folder := home.SkillFolder("scripted")
	about, err := os.Stat(folder)
	if err != nil {
		t.Fatalf("the skill folder is not there: %v", err)
	}
	if about.Mode().Perm() != contract.HomeFolderMode.Perm() {
		t.Errorf("the folder's mode is %v, want %v", about.Mode().Perm(), contract.HomeFolderMode.Perm())
	}
	cases := map[string]os.FileMode{
		skill.DescriptionFile: contract.DataFileMode.Perm(),
		skill.ScriptFile:      contract.HomeFolderMode.Perm(),
	}
	for file, wanted := range cases {
		about, err := os.Stat(filepath.Join(folder, file))
		if err != nil {
			t.Fatalf("the file %s is not there: %v", file, err)
		}
		if about.Mode().Perm() != wanted {
			t.Errorf("the mode of %s is %v, want %v", file, about.Mode().Perm(), wanted)
		}
	}
}
