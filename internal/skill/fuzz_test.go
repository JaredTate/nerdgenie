package skill_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/skill"
)

// The seeds are the shapes a SKILL.md really takes, plus the shapes that would
// break a reader written carelessly: no heading, a permissions line with no
// value, a number where a number cannot go, and a heading with nothing under it.
var descriptionSeeds = []string{
	"# name\n\nOne line about the skill.\n\n## Triggers\n\n- a word\n\n## Permissions\n\n- daily limit: 3\n",
	"",
	"#",
	"# \n\n\n",
	"## Permissions\n- site:\n",
	"# a\n\nb\n\n## Permissions\n\n- daily limit: not a number\n",
	"# a\n\nb\n\n## Permissions\n\n- irreversible step: -4\n",
	"# a\n\nb\n\n## Permissions\n\n- something else: yes\n",
	"# a\n\nb\n\n## triggers\n\n- \n- word\n",
	"\ufeff# a\r\n\r\nb\r\n",
}

func FuzzParseDescriptionFile(f *testing.F) {
	for _, seed := range descriptionSeeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, content string) {
		definition, err := skill.ParseDescriptionFile([]byte(content))
		if err != nil {
			return
		}
		if err := skill.CheckName(definition.Name); err != nil {
			t.Fatalf("a definition that parsed has the name %q, which is not a folder name: %v", definition.Name, err)
		}
		if definition.Description == "" {
			t.Fatal("a definition that parsed has no description, and the description is what rides in the prompt")
		}
		if len([]rune(definition.Description)) > skill.MaxDescriptionRunes {
			t.Fatalf("a definition that parsed has a description of %d characters, over the cap", len([]rune(definition.Description)))
		}
		if strings.Contains(definition.Description, "\n") {
			t.Fatalf("the description %q holds a line break, and it is one line", definition.Description)
		}
		if limit := definition.Permissions.DailyLimit; limit < 1 || limit > skill.MaxDailyLimit {
			t.Fatalf("a definition that parsed has a daily limit of %d, outside the range", limit)
		}
		// Rendering what was read and reading it again gives the same answer, so
		// that a folder saved from a definition says what the definition said.
		again, err := skill.ParseDescriptionFile(skill.RenderDescriptionFile(definition))
		if err != nil {
			t.Fatalf("a definition that parsed did not survive being written out and read back: %v", err)
		}
		if again.Name != definition.Name || again.Description != definition.Description {
			t.Fatalf("reading back gave %+v, want %+v", again, definition)
		}
	})
}

func FuzzParseSteps(f *testing.F) {
	seeds := []string{
		"1. Do the thing.\n   tool: read\n   input: {\"path\": \"x\"}\n   expect: yes\n",
		"",
		"1.",
		"0. Nothing starts at zero.\n",
		"2. Out of order.\n",
		"1. One.\n3. Three.\n",
		"1. One.\n   input: {\"path\": \"x\"}\n",
		"1. One.\n   tool: read\n   input: not json\n",
		"1. One.\n   tool: read\n   input: {\"path\": \"{{arguments}}\"}\n",
		"1. \n",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, content string) {
		steps, err := skill.ParseSteps([]byte(content))
		if err != nil {
			return
		}
		for position, step := range steps {
			if step.Number != position+1 {
				t.Fatalf("step %d of a list that parsed is numbered %d", position+1, step.Number)
			}
			if step.Intent == "" {
				t.Fatalf("step %d of a list that parsed says nothing about what it is for", step.Number)
			}
		}
		if len(steps) > skill.MaxSteps {
			t.Fatalf("a list that parsed has %d steps, over the cap", len(steps))
		}
	})
}

func FuzzParseTestFile(f *testing.F) {
	for _, seed := range []string{"arguments: today\nexpect: rain\n", "", ":", "arguments:", "EXPECT: rain"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, content string) {
		plan, err := skill.ParseTestFile([]byte(content))
		if err != nil {
			return
		}
		if strings.Contains(plan.Arguments, "\n") || strings.Contains(plan.Expect, "\n") {
			t.Fatalf("a dry run that parsed holds a line break in %+v", plan)
		}
	})
}
