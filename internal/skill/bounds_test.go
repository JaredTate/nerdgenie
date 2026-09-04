package skill_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/skill"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// describing is a whole SKILL.md built from a body, so that a test can put one
// line of a permissions block under test without writing the rest out again.
func describing(name string, body string) []byte {
	return []byte("# " + name + "\n\nA skill written to test one bound at a time.\n" + body)
}

func TestTheBoundsOnADescriptionFile(t *testing.T) {
	cases := []struct {
		what    string
		content []byte
	}{
		{"a file over the size limit", []byte(strings.Repeat("a", skill.MaxFileBytes+1))},
		{"a description over the size limit", []byte("# long\n\n" + strings.Repeat("b", skill.MaxDescriptionRunes+1) + "\n")},
		{"no description at all", []byte("# quiet\n\n## Triggers\n\n- a word\n")},
		{"too many triggers", describing("many", "\n## Triggers\n\n"+strings.Repeat("- one\n", skill.MaxTriggers+1))},
		{"a trigger over the size limit", describing("long", "\n## Triggers\n\n- "+strings.Repeat("c", skill.MaxTriggerRunes+1)+"\n")},
		{"too many websites", describing("sites", "\n## Permissions\n\n"+strings.Repeat("- site: a.example.com\n", skill.MaxSites+1))},
		{"a daily limit over the cap", describing("big", fmt.Sprintf("\n## Permissions\n\n- daily limit: %d\n", skill.MaxDailyLimit+1))},
		{"a daily limit of nothing", describing("none", "\n## Permissions\n\n- daily limit: 0\n")},
		{"a step numbered zero", describing("zero", "\n## Permissions\n\n- irreversible step: 0\n")},
		{"a permissions line with no value", describing("empty", "\n## Permissions\n\n- site:\n")},
	}
	for _, test := range cases {
		if _, err := skill.ParseDescriptionFile(test.content); err == nil {
			t.Errorf("%s was allowed, and it must be refused", test.what)
		}
	}
}

func TestADescriptionFileSurvivesBeingWrittenOutAndReadBack(t *testing.T) {
	written := describing("whole", "\n## Triggers\n\n- a word\n\n## Permissions\n\n"+
		"- browser profile: news\n- site: news.example.com\n- daily limit: 12\n- irreversible step: 1\n")
	definition, err := skill.ParseDescriptionFile(written)
	if err != nil {
		t.Fatalf("the whole description file did not parse: %v", err)
	}
	again, err := skill.ParseDescriptionFile(skill.RenderDescriptionFile(definition))
	if err != nil {
		t.Fatalf("the description file did not survive being written out: %v", err)
	}
	if fmt.Sprintf("%+v", again) != fmt.Sprintf("%+v", definition) {
		t.Errorf("reading back gave %+v, want %+v", again, definition)
	}

	// A definition with no daily limit of its own is written out with the
	// default, so that a folder never says a limit of nothing.
	bare := skill.Definition{Name: "bare", Description: "Has no daily limit of its own."}
	read, err := skill.ParseDescriptionFile(skill.RenderDescriptionFile(bare))
	if err != nil {
		t.Fatalf("a definition with no daily limit did not survive being written out: %v", err)
	}
	if read.Permissions.DailyLimit != skill.DefaultDailyLimit {
		t.Errorf("the daily limit written out is %d, want the default of %d", read.Permissions.DailyLimit, skill.DefaultDailyLimit)
	}
}

func TestTheBoundsOnAStepList(t *testing.T) {
	many := strings.Builder{}
	for number := range skill.MaxSteps + 1 {
		fmt.Fprintf(&many, "%d. Do the thing.\n", number+1)
	}
	cases := []struct {
		what    string
		content []byte
	}{
		{"a file over the size limit", []byte(strings.Repeat("a", skill.MaxFileBytes+1))},
		{"more steps than the cap", []byte(many.String())},
		{"a step out of order", []byte("1. One.\n3. Three.\n")},
		{"a step with no words of its own", []byte("1.\n   tool: read\n")},
		{"an input with no tool", []byte("1. One.\n   input: {\"path\": \"x\"}\n")},
		{"an input that is not a JSON object", []byte("1. One.\n   tool: read\n   input: not json at all\n")},
	}
	for _, test := range cases {
		if _, err := skill.ParseSteps(test.content); err == nil {
			t.Errorf("%s was allowed, and it must be refused", test.what)
		}
	}
}

func TestAStepGoesOnOverSeveralLines(t *testing.T) {
	steps, err := skill.ParseSteps([]byte("1. Read the note,\n   which is where the list is kept.\n   tool: read\n"))
	if err != nil {
		t.Fatalf("a step written over two lines did not parse: %v", err)
	}
	if len(steps) != 1 || steps[0].Intent != "Read the note, which is where the list is kept." {
		t.Errorf("the steps are %+v, want the two lines joined into one intent", steps)
	}
	written := skill.RenderSteps(steps)
	again, err := skill.ParseSteps(written)
	if err != nil || len(again) != 1 || again[0].Intent != steps[0].Intent {
		t.Errorf("writing the step out and reading it back gave %+v with error %v", again, err)
	}
}

func TestATestFileOverTheSizeLimitIsRefused(t *testing.T) {
	if _, err := skill.ParseTestFile([]byte(strings.Repeat("a", skill.MaxFileBytes+1))); err == nil {
		t.Error("a test file over the size limit was allowed, and it must be refused")
	}
}

func TestAFileOverTheSizeLimitIsRefusedByName(t *testing.T) {
	built := newHarness(t)
	built.writeSkill(t, "testdata/skills/news-headlines", "news-headlines")
	big := filepath.Join(built.home.SkillFolder("news-headlines"), skill.StepsFile)
	if err := os.WriteFile(big, []byte(strings.Repeat("a", skill.MaxFileBytes+1)), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the oversized file: %v", err)
	}
	_, err := skill.ReadFolder(built.home.SkillFolder("news-headlines"))
	if err == nil || !strings.Contains(err.Error(), skill.StepsFile) {
		t.Errorf("reading a folder with an oversized file gave %v, want an error naming the file", err)
	}
	if err := skill.CheckFileNames(map[string][]byte{skill.StepsFile: make([]byte, skill.MaxFileBytes+1)}); err == nil {
		t.Error("saving an oversized file was allowed, and it must be refused")
	}
}

func TestAFolderInPlaceOfAFileIsPassedOver(t *testing.T) {
	built := newHarness(t)
	built.writeSkill(t, "testdata/skills/news-headlines", "news-headlines")
	instead := filepath.Join(built.home.SkillFolder("news-headlines"), skill.ScriptFile)
	if err := os.MkdirAll(instead, contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the folder standing in for a file: %v", err)
	}
	if _, err := skill.ReadFolder(built.home.SkillFolder("news-headlines")); err != nil {
		t.Errorf("a folder where a file could have been stopped the load: %v", err)
	}
}

func TestReadSummaryNamesAFolderThatIsNotASkill(t *testing.T) {
	built := newHarness(t)
	if _, err := skill.ReadSummary(built.home.SkillFolder("nothing-here")); err == nil {
		t.Error("a folder with no description file read as a skill, and it must be refused")
	}
	built.writeFiles(t, "mismatched", map[string]string{skill.DescriptionFile: "# other\n\nNames a skill other than its folder.\n"})
	if _, err := skill.ReadSummary(built.home.SkillFolder("mismatched")); err == nil {
		t.Error("a folder whose heading names another skill read as a skill, and it must be refused")
	}
}

func TestOnlySoManySkillsAreKept(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	for number := range skill.MaxSkills {
		name := fmt.Sprintf("skill-%d", number)
		built.writeFiles(t, name, map[string]string{skill.DescriptionFile: "# " + name + "\n\nOne of very many skills indeed.\n"})
	}
	err := built.store.Save(ctx, contract.SkillSavedByPerson, "one-too-many", filesFor("one-too-many", "The skill that does not fit.", "Do it."))
	if err == nil || !strings.Contains(err.Error(), "one-too-many") {
		t.Errorf("saving past the cap gave %v, want an error naming the skill that did not fit", err)
	}
	if err := built.store.Save(ctx, contract.SkillSavedByPerson, "skill-0", filesFor("skill-0", "A skill that was already there.", "Do it.")); err != nil {
		t.Errorf("saving over a skill that was already there was refused at the cap: %v", err)
	}
}

func TestOnlySoManyVersionsAreKept(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	for round := range skill.MaxVersions + 3 {
		files := filesFor("note", fmt.Sprintf("Version number %d of this skill.", round+1), "Do it.")
		if err := built.store.Save(ctx, contract.SkillSavedByPerson, "note", files); err != nil {
			t.Fatalf("save %d failed: %v", round+1, err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(built.home.SkillFolder("note"), skill.VersionsFolder))
	if err != nil {
		t.Fatalf("cannot read the versions folder: %v", err)
	}
	if len(entries) != skill.MaxVersions {
		t.Errorf("there are %d kept versions, want the cap of %d", len(entries), skill.MaxVersions)
	}
}

func TestRestoringAVersionWithNothingInItIsRefused(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	if err := built.store.Save(ctx, contract.SkillSavedByPerson, "note", filesFor("note", "A skill with an empty version beside it.", "Do it.")); err != nil {
		t.Fatalf("the save failed: %v", err)
	}
	empty := filepath.Join(built.home.SkillFolder("note"), skill.VersionsFolder, "1")
	if err := os.MkdirAll(empty, contract.HomeFolderMode); err != nil {
		t.Fatalf("cannot make the empty version folder: %v", err)
	}
	if _, err := built.store.Rollback(ctx, "note"); err == nil {
		t.Error("rolling back to a version with nothing in it was allowed, and it must be refused")
	}
}

func TestAStepWithNoInputRunsWithNothingInIt(t *testing.T) {
	echo := &echoTool{name: "echo"}
	built := newHarness(t, echo)
	ctx := context.Background()
	files := map[string][]byte{
		skill.DescriptionFile: []byte("# bare-step\n\nA skill whose one step takes no input.\n"),
		skill.StepsFile:       []byte("1. Say nothing in particular.\n   tool: echo\n"),
	}
	if err := built.store.Save(ctx, contract.SkillSavedByPerson, "bare-step", files); err != nil {
		t.Fatalf("saving the skill failed: %v", err)
	}
	report, err := built.store.Run(ctx, "bare-step", "")
	if err != nil {
		t.Fatalf("running a step with no input failed: %v", err)
	}
	if !strings.Contains(report, "{}") {
		t.Errorf("the report is %q, want the empty input the step was given", report)
	}
}

func TestALongResultIsCutShortInTheReport(t *testing.T) {
	long := strings.Repeat("a very long line of output ", 200)
	built := newHarness(t, testkit.NewScriptedTool(contract.ToolSpec{Name: "echo"}, long))
	ctx := context.Background()
	files := map[string][]byte{
		skill.DescriptionFile: []byte("# long-result\n\nA skill whose one step says a great deal.\n"),
		skill.StepsFile:       []byte("1. Say a great deal.\n   tool: echo\n"),
	}
	if err := built.store.Save(ctx, contract.SkillSavedByPerson, "long-result", files); err != nil {
		t.Fatalf("saving the skill failed: %v", err)
	}
	report, err := built.store.Run(ctx, "long-result", "")
	if err != nil {
		t.Fatalf("running the skill failed: %v", err)
	}
	if !strings.Contains(report, "cut short before the end") {
		t.Errorf("the report is %q, want the long result cut short", report)
	}
	if len(report) > skill.MaxReportBytes {
		t.Errorf("the report is %d bytes, over the cap of %d", len(report), skill.MaxReportBytes)
	}
}

func TestAWebsiteIsMatchedByItsHostHoweverItIsWritten(t *testing.T) {
	fetched := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolWeb}, "one", "two", "three")
	built := newHarness(t, fetched)
	ctx := context.Background()
	files := map[string][]byte{
		skill.DescriptionFile: []byte("# hosts\n\nGoes to the same site written three ways.\n\n## Permissions\n\n- site: news.example.com\n"),
		skill.StepsFile: []byte(
			"1. Go to the site by its address.\n   tool: web\n   input: {\"url\": \"https://news.example.com/today\"}\n\n" +
				"2. Go to a part of the same site.\n   tool: web\n   input: {\"url\": \"https://sport.news.example.com/\"}\n\n" +
				"3. Search the web, which visits nothing in particular.\n   tool: web\n   input: {\"query\": \"the news\"}\n"),
	}
	if err := built.store.Save(ctx, contract.SkillSavedByPerson, "hosts", files); err != nil {
		t.Fatalf("saving the skill failed: %v", err)
	}
	if _, err := built.store.Run(ctx, "hosts", ""); err != nil {
		t.Errorf("running a skill whose steps stay inside the block failed: %v", err)
	}
	if previews := built.channel.Previews(); len(previews) != 0 {
		t.Errorf("the user was asked about %v, want nothing, because every step stayed inside the block", previews)
	}
}

func TestAFolderBuiltInMemoryKnowsItIsNotOnDisk(t *testing.T) {
	built := newHarness(t)
	files := map[string][]byte{
		skill.DescriptionFile: []byte("# scripted\n\nA skill whose procedure is one executable.\n"),
		skill.ScriptFile:      []byte("#!/bin/sh\necho hello\n"),
		skill.TestFile:        []byte("arguments:\n"),
		skill.ChangelogFile:   []byte("# changelog for scripted\n"),
	}
	folder, err := skill.ParseFolder(files)
	if err != nil {
		t.Fatalf("the folder did not parse: %v", err)
	}
	if folder.Path != "" {
		t.Errorf("a folder built in memory has the path %q, want none", folder.Path)
	}
	if !strings.Contains(folder.Body, skill.ScriptFile) {
		t.Errorf("the body is %q, want it to say the procedure is the script", folder.Body)
	}

	if err := built.store.Save(context.Background(), contract.SkillSavedByPerson, "scripted", files); err != nil {
		t.Fatalf("saving the folder failed: %v", err)
	}
	saved, err := skill.ReadFolder(built.home.SkillFolder("scripted"))
	if err != nil {
		t.Fatalf("reading the saved folder failed: %v", err)
	}
	if saved.Path != built.home.SkillFolder("scripted") {
		t.Errorf("the saved folder's path is %q, want where it was written", saved.Path)
	}
}

func TestAVeryLongAskBecomesAOneLineDescription(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	built.channel.AnswerPreviewsWith(contract.AnswerOnce)
	record := finishedTask()
	record.Goal.Ask = strings.Repeat("a very long ask indeed ", 40)

	saved, err := built.store.OfferFromTask(ctx, "long-ask", record)
	if err != nil || !saved {
		t.Fatalf("offering a task with a very long ask gave %v and saved %v", err, saved)
	}
	listed, err := built.store.List(ctx)
	if err != nil || len(listed) != 1 {
		t.Fatalf("the listing is %v with error %v, want the one skill", listed, err)
	}
	if len([]rune(listed[0].Description)) > skill.MaxDescriptionRunes {
		t.Errorf("the description is %d characters, over the cap", len([]rune(listed[0].Description)))
	}
}
