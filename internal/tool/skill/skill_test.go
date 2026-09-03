package skill_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
	"github.com/JaredTate/coeus/internal/tool/skill"
)

// newTool builds the skill tool over the fake skill store, with one skill in it.
func newTool(t *testing.T) (*skill.Tool, *testkit.FakeSkill) {
	t.Helper()
	skills := testkit.NewFakeSkill()
	skills.Add(contract.SkillSummary{Name: "post-a-note", Description: "Posts a note to the board."},
		"# post-a-note\n\nOpen the board and write the note.\n", "post a note")
	return skill.New(skill.Settings{Skills: skills}), skills
}

// run calls the tool with the fields written as JSON.
func run(t *testing.T, tool *skill.Tool, fields map[string]any) (contract.ToolOutput, error) {
	t.Helper()
	written, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("cannot write the tool's input as JSON: %v", err)
	}
	return tool.Run(context.Background(), written)
}

func TestTheDescriptionFitsInTheCapAndTakesTheFixedFieldNames(t *testing.T) {
	tool, _ := newTool(t)
	spec := tool.Spec()

	if spec.Name != contract.ToolSkill {
		t.Errorf("the tool calls itself %q, want %q", spec.Name, contract.ToolSkill)
	}
	if words := contract.DescriptionWordCount(spec.Description); words > contract.MaxToolDescriptionWords {
		t.Errorf("the description is %d words, and the cap is %d", words, contract.MaxToolDescriptionWords)
	}
	names := []string{}
	for _, field := range spec.Fields {
		names = append(names, field.Name)
	}
	if strings.Join(names, ",") != "action,name,query,files" {
		t.Errorf("the tool takes the fields %v, and the permission function reduces a skill call by name", names)
	}
	if len(spec.Classes) != 3 {
		t.Errorf("the tool claims the classes %v, and it reads, runs, and writes", spec.Classes)
	}
}

func TestViewingASkillBringsBackItsBody(t *testing.T) {
	tool, _ := newTool(t)

	output, err := run(t, tool, map[string]any{"action": "view", "name": "post-a-note"})
	if err != nil {
		t.Fatalf("viewing a skill failed: %v", err)
	}
	testkit.Golden(t, "a_viewed_skill.txt", []byte(output.Text))
}

func TestRunningASkillRunsItAndBringsBackWhatItSaid(t *testing.T) {
	tool, skills := newTool(t)

	output, err := run(t, tool, map[string]any{"action": "run", "name": "post-a-note", "query": "the weekly note"})
	if err != nil {
		t.Fatalf("running a skill failed: %v", err)
	}
	if !strings.Contains(output.Text, "the weekly note") {
		t.Errorf("running the skill said %q, want what it did with the words it was given", output.Text)
	}
	if runs := skills.Runs(); len(runs) != 1 || runs[0] != "post-a-note" {
		t.Errorf("the skills that ran are %v, want the one that was asked for", runs)
	}
}

func TestSavingASkillWritesItsFolder(t *testing.T) {
	tool, skills := newTool(t)

	if _, err := run(t, tool, map[string]any{
		"action": "save",
		"name":   "water-the-plants",
		"files": map[string]any{
			"SKILL.md":  "# water-the-plants\n",
			"STEPS.md":  "Open the tap.\n",
			"CHANGES.md": "First written today.\n",
		},
	}); err != nil {
		t.Fatalf("saving a skill failed: %v", err)
	}
	files := skills.Files("water-the-plants")
	if len(files) != 3 || string(files["STEPS.md"]) != "Open the tap.\n" {
		t.Errorf("the skill folder holds %v, want the three files it was given", files)
	}
}

func TestBadInputIsRefusedWithALineTheModelCanActOn(t *testing.T) {
	tool, _ := newTool(t)

	if _, err := tool.Run(context.Background(), json.RawMessage("not json")); err == nil {
		t.Errorf("input that is not JSON was treated as a call")
	}
	for _, broken := range []map[string]any{
		{"action": "dance", "name": "post-a-note"},
		{"action": "view"},
		{"action": "run"},
		{"action": "save", "name": "empty"},
		{"action": "view", "name": "no-such-skill"},
	} {
		if _, err := run(t, tool, broken); err == nil {
			t.Errorf("the call %v was treated as something the tool could do", broken)
		}
	}
}

func TestAToolWithNoSkillsWiredInSaysSo(t *testing.T) {
	tool := skill.New(skill.Settings{})

	_, err := run(t, tool, map[string]any{"action": "view", "name": "anything"})
	if err == nil {
		t.Fatalf("a skill was viewed with no skill store behind the tool")
	}
	if !strings.Contains(err.Error(), "skill") {
		t.Errorf("the refusal reads %q and does not say what is missing", err)
	}
}
