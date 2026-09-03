package skill_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/skill"
	"github.com/JaredTate/coeus/internal/testkit"
)

// twoStepSkill is a skill whose two steps both call the echo tool, which is
// enough to prove that a replay runs the steps in order and checks each one.
func twoStepSkill(name string) map[string][]byte {
	return map[string][]byte{
		skill.DescriptionFile: []byte("# " + name + "\n\n" +
			"Says two things back, in the order the steps give.\n\n" +
			"## Triggers\n\n- say two things\n\n## Permissions\n\n- daily limit: 5\n"),
		skill.StepsFile: []byte(
			"1. Say the first thing.\n   tool: echo\n   input: {\"say\": \"the first thing\"}\n   expect: first\n\n" +
				"2. Say what the run was asked for.\n   tool: echo\n   input: {\"say\": \"you asked for {{arguments}}\"}\n   expect: asked\n"),
	}
}

func TestATriggerRunsASkillWithTheModelNeverCalled(t *testing.T) {
	echo := &echoTool{name: "echo"}
	built := newHarness(t, echo)
	ctx := context.Background()
	model := testkit.NewFakeModel(testkit.Script{Name: "never-called", ContextLength: 1000})
	if err := built.store.Save(ctx, "say-two", twoStepSkill("say-two")); err != nil {
		t.Fatalf("saving the skill failed: %v", err)
	}

	matched, err := built.store.Match(ctx, "please say two things for me")
	if err != nil {
		t.Fatalf("matching failed: %v", err)
	}
	if !matched.Matched || matched.Name != "say-two" {
		t.Fatalf("the match is %+v, want the skill whose triggers the message holds", matched)
	}

	report, err := built.store.Run(ctx, matched.Name, "the news")
	if err != nil {
		t.Fatalf("running the skill failed: %v\n%s", err, report)
	}
	if echo.calls != 2 {
		t.Errorf("the tool ran %d times, want both steps", echo.calls)
	}
	if !strings.Contains(report, "you asked for the news") {
		t.Errorf("the report is %q, want the arguments put into the second step", report)
	}
	if asked := model.Requests(); len(asked) != 0 {
		t.Errorf("the model was called %d times, and a replayed skill never calls it", len(asked))
	}
}

func TestMatchNeedsEveryTriggerAndOnlyOneSkill(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	if err := built.store.Save(ctx, "say-two", twoStepSkill("say-two")); err != nil {
		t.Fatalf("saving the skill failed: %v", err)
	}

	cases := []struct {
		text  string
		fires bool
	}{
		{"say two things now", true},
		{"SAY TWO THINGS", true},
		{"say two things.", true},
		{"say two", false},
		{"things two say", false},
		{"", false},
	}
	for _, test := range cases {
		matched, err := built.store.Match(ctx, test.text)
		if err != nil {
			t.Fatalf("matching %q failed: %v", test.text, err)
		}
		if matched.Matched != test.fires {
			t.Errorf("matching %q gave %v, want %v", test.text, matched.Matched, test.fires)
		}
	}
}

func TestAMessageThatFiresTwoSkillsFiresNeither(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	for _, name := range []string{"say-two", "say-more"} {
		files := twoStepSkill(name)
		if err := built.store.Save(ctx, name, files); err != nil {
			t.Fatalf("saving the skill %q failed: %v", name, err)
		}
	}
	matched, err := built.store.Match(ctx, "say two things please")
	if err != nil {
		t.Fatalf("matching failed: %v", err)
	}
	if matched.Matched {
		t.Errorf("the match is %+v, want nothing, because two skills wanted the same message", matched)
	}
}

func TestAFolderThatWillNotLoadIsPassedOverByTheListing(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	if err := built.store.Save(ctx, "good", filesFor("good", "A skill that reads perfectly well.", "Do it.")); err != nil {
		t.Fatalf("saving the good skill failed: %v", err)
	}
	built.writeFiles(t, "bad", map[string]string{skill.DescriptionFile: "# bad\n\n## Permissions\n\n- daily limit: not a number\n"})
	built.writeFiles(t, "empty", map[string]string{"README.md": "nothing here"})

	listed, err := built.store.List(ctx)
	if err != nil {
		t.Fatalf("listing failed: %v", err)
	}
	if len(listed) != 1 || listed[0].Name != "good" {
		t.Errorf("the listing is %v, want only the skill that reads", listed)
	}
	if _, err := built.store.Load(ctx, "bad"); err == nil {
		t.Error("the broken skill loaded, and loading is the strict path")
	}
}

func TestAFailedStepSaysWhatItExpectedAndWhatItSaw(t *testing.T) {
	built := newHarness(t, testkit.NewScriptedTool(contract.ToolSpec{Name: "echo"}, "something else entirely"))
	ctx := context.Background()
	if err := built.store.Save(ctx, "say-two", twoStepSkill("say-two")); err != nil {
		t.Fatalf("saving the skill failed: %v", err)
	}

	report, err := built.store.Run(ctx, "say-two", "the news")
	if err == nil {
		t.Fatal("the skill ran to the end, and its first step should have failed")
	}
	if !strings.Contains(err.Error(), "\"first\"") || !strings.Contains(err.Error(), "something else entirely") {
		t.Errorf("the error is %q, want what the step expected and what it saw", err)
	}
	if strings.TrimSpace(report) != "" {
		t.Errorf("the report is %q, want nothing, because the first step is the one that failed", report)
	}
}

func TestAStepWrittenInWordsHandsTheWorkBackToTheModel(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	files := map[string][]byte{
		skill.DescriptionFile: []byte("# in-words\n\nA procedure written out in plain words.\n"),
		skill.StepsFile:       []byte("1. Look at the page and decide what to do.\n"),
	}
	if err := built.store.Save(ctx, "in-words", files); err != nil {
		t.Fatalf("saving the skill failed: %v", err)
	}
	_, err := built.store.Run(ctx, "in-words", "")
	if err == nil || !strings.Contains(err.Error(), "names no tool") {
		t.Errorf("running a skill written in words gave %v, want it to say the model has to carry the step out", err)
	}
}

func TestAToolTheMachineDoesNotHaveIsNamed(t *testing.T) {
	built := newHarness(t)
	ctx := context.Background()
	if err := built.store.Save(ctx, "say-two", twoStepSkill("say-two")); err != nil {
		t.Fatalf("saving the skill failed: %v", err)
	}
	_, err := built.store.Run(ctx, "say-two", "")
	if err == nil || !strings.Contains(err.Error(), "\"echo\"") {
		t.Errorf("running a skill whose tool is missing gave %v, want an error naming the tool", err)
	}
}

func TestADryRunStopsBeforeTheStepThatCannotBeUndone(t *testing.T) {
	echo := &echoTool{name: "echo"}
	built := newHarness(t, echo)
	ctx := context.Background()
	files := twoStepSkill("say-two")
	files[skill.DescriptionFile] = []byte("# say-two\n\nSays two things back, in the order the steps give.\n\n" +
		"## Permissions\n\n- daily limit: 5\n- irreversible step: 2\n")
	files[skill.TestFile] = []byte("arguments: the news\nexpect: the first thing\n")
	if err := built.store.Save(ctx, "say-two", files); err != nil {
		t.Fatalf("saving the skill failed: %v", err)
	}

	report, err := built.store.DryRun(ctx, "say-two")
	if err != nil {
		t.Fatalf("the dry run failed: %v\n%s", err, report)
	}
	if echo.calls != 1 {
		t.Errorf("the tool ran %d times, want only the step before the one that cannot be undone", echo.calls)
	}
	if !strings.Contains(report, "step 2 stops the dry run") {
		t.Errorf("the report is %q, want it to say where the dry run stopped", report)
	}
}

func TestADryRunThatDoesNotSeeWhatItExpectedFails(t *testing.T) {
	built := newHarness(t, &echoTool{name: "echo"})
	ctx := context.Background()
	files := twoStepSkill("say-two")
	files[skill.TestFile] = []byte("arguments: the news\nexpect: something nobody said\n")
	if err := built.store.Save(ctx, "say-two", files); err != nil {
		t.Fatalf("saving the skill failed: %v", err)
	}
	if _, err := built.store.DryRun(ctx, "say-two"); err == nil {
		t.Error("the dry run passed, and its expectation was never met")
	}
}

func TestAStepThatCannotBeUndoneIsPreviewed(t *testing.T) {
	echo := &echoTool{name: "echo"}
	built := newHarness(t, echo)
	ctx := context.Background()
	files := twoStepSkill("say-two")
	files[skill.DescriptionFile] = []byte("# say-two\n\nSays two things back, in the order the steps give.\n\n" +
		"## Permissions\n\n- daily limit: 5\n- irreversible step: 2\n")
	if err := built.store.Save(ctx, "say-two", files); err != nil {
		t.Fatalf("saving the skill failed: %v", err)
	}

	built.channel.AnswerPreviewsWithReason("that post would go to the wrong account")
	_, err := built.store.Run(ctx, "say-two", "the news")
	if err == nil || !strings.Contains(err.Error(), "was refused") {
		t.Errorf("running with the preview refused gave %v, want the run to stop there", err)
	}
	if !strings.Contains(err.Error(), "the wrong account") {
		t.Errorf("the refusal is %q, want the words the user gave with it", err)
	}
	previews := built.channel.Previews()
	if len(previews) != 1 || !strings.Contains(previews[0].Title, "cannot be undone") {
		t.Errorf("the previews are %v, want one saying the step cannot be undone", previews)
	}
	if echo.calls != 1 {
		t.Errorf("the tool ran %d times, want only the step before the one that was refused", echo.calls)
	}
}

func TestARefusedStepStopsTheSkill(t *testing.T) {
	built := newHarness(t, &echoTool{name: "echo"})
	ctx := context.Background()
	built.permission.Rule("echo", contract.PermissionDecision{Ruling: contract.RulingDeny, Reason: "a rule of yours refuses it"})
	if err := built.store.Save(ctx, "say-two", twoStepSkill("say-two")); err != nil {
		t.Fatalf("saving the skill failed: %v", err)
	}
	_, err := built.store.Run(ctx, "say-two", "")
	if err == nil || !strings.Contains(err.Error(), "refused by your rules") {
		t.Errorf("running a refused step gave %v, want it refused with the reason", err)
	}
}

func TestAStepThatNeedsAYesStopsWhenThereIsNoScreen(t *testing.T) {
	built := newHarness(t, &echoTool{name: "echo"})
	ctx := context.Background()
	quiet, err := skill.New(skill.Options{Home: built.home, Clock: built.clock, Tools: built.tools, Permission: built.permission})
	if err != nil {
		t.Fatalf("cannot build a store with no screen: %v", err)
	}
	built.permission.Rule("echo", contract.PermissionDecision{Ruling: contract.RulingAsk, Reason: "it wants a yes"})
	if err := built.store.Save(ctx, "say-two", twoStepSkill("say-two")); err != nil {
		t.Fatalf("saving the skill failed: %v", err)
	}
	_, err = quiet.Run(ctx, "say-two", "")
	if err == nil || !strings.Contains(err.Error(), "no screen to ask on") {
		t.Errorf("running with no screen gave %v, want it to say what it needed", err)
	}
}

func TestASkillWithAScriptRunsTheScript(t *testing.T) {
	shell := testkit.NewScriptedTool(contract.ToolSpec{Name: contract.ToolShell}, "the script said hello")
	built := newHarness(t, shell)
	ctx := context.Background()
	files := map[string][]byte{
		skill.DescriptionFile: []byte("# scripted\n\nA skill whose procedure is one executable.\n"),
		skill.ScriptFile:      []byte("#!/bin/sh\necho hello\n"),
	}
	if err := built.store.Save(ctx, "scripted", files); err != nil {
		t.Fatalf("saving the skill failed: %v", err)
	}

	report, err := built.store.Run(ctx, "scripted", "loudly")
	if err != nil {
		t.Fatalf("running the scripted skill failed: %v", err)
	}
	if !strings.Contains(report, "the script said hello") {
		t.Errorf("the report is %q, want what the script said", report)
	}
	inputs := shell.Inputs()
	if len(inputs) != 1 || !strings.Contains(string(inputs[0]), skill.ScriptFile) || !strings.Contains(string(inputs[0]), "loudly") {
		t.Errorf("the shell was given %v, want the script's path and the arguments", inputs)
	}
}

func TestNewRefusesAStoreThatCouldNotRunASkill(t *testing.T) {
	built := newHarness(t)
	cases := []skill.Options{
		{Clock: built.clock, Tools: built.tools, Permission: built.permission},
		{Home: built.home, Tools: built.tools, Permission: built.permission},
		{Home: built.home, Clock: built.clock, Permission: built.permission},
		{Home: built.home, Clock: built.clock, Tools: built.tools},
	}
	for _, options := range cases {
		if _, err := skill.New(options); err == nil {
			t.Errorf("a store built from %+v was allowed, and it must be refused", options)
		}
	}
}
