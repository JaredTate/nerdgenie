package context

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// twoSkills are the skill list a fresh home might carry: the quality skill that
// ships with the program and one a person wrote.
func twoSkills() []contract.SkillSummary {
	return []contract.SkillSummary{
		{Name: "qa", Description: "Walks an app step by step and reports whether every expected state is what it sees."},
		{Name: "tidy-notes", Description: "Keeps the notes folder tidy and says what it moved."},
	}
}

// TestTheSkillListRidesInThePersonaLayerAboveBoundaryA is what the first human
// trial found: the skill store had a List call and nothing ever made it, so the
// model could reach a skill only if it already knew the name, and the skill that
// ships with the program was never seen. The names and one-line descriptions
// belong in the prompt, design section 8 says, and they change as rarely as the
// persona does, so they ride in the persona block, above cache boundary A.
func TestTheSkillListRidesInThePersonaLayerAboveBoundaryA(t *testing.T) {
	builder := newTestBuilder(t, Options{Skills: twoSkills()})
	writePersonaFile(t, builder.home.SoulFile(), "I am Coeus.")

	request, err := builder.Build(t.Context(), sampleInput())
	if err != nil {
		t.Fatalf("cannot build the working context: %v", err)
	}

	persona := blockNamed(t, request, BlockPersona)
	if persona.Boundary != contract.CacheBoundaryA {
		t.Errorf("the persona block ends boundary %q, want %q, because the skill list must sit above A", persona.Boundary, contract.CacheBoundaryA)
	}
	if !strings.Contains(persona.Text, skillsHeading) {
		t.Errorf("the persona block does not say what the skill list is:\n%s", persona.Text)
	}
	for _, wanted := range []string{
		"qa: Walks an app step by step and reports whether every expected state is what it sees.",
		"tidy-notes: Keeps the notes folder tidy and says what it moved.",
	} {
		if !strings.Contains(persona.Text, wanted) {
			t.Errorf("the persona block does not carry the line %q:\n%s", wanted, persona.Text)
		}
	}
	if soul, skills := strings.Index(persona.Text, "I am Coeus."), strings.Index(persona.Text, skillsHeading); soul > skills {
		t.Errorf("the skill list at %d comes before SOUL.md at %d, and who the agent is comes first", skills, soul)
	}
	if !strings.Contains(skillsHeading, "`skill`") {
		t.Errorf("the heading %q does not name the `skill` tool, so the model is not told how to load one", skillsHeading)
	}
	for _, message := range request.Messages {
		if strings.Contains(message.Text, skillsHeading) {
			t.Error("the skill list is below the cache line as well, and it belongs above it once")
		}
	}
}

// TestASkillListAloneMakesAPersonaBlock proves a fresh install, which has no
// SOUL.md yet and the one shipped skill, still tells the model about that skill.
func TestASkillListAloneMakesAPersonaBlock(t *testing.T) {
	builder := newTestBuilder(t, Options{Skills: twoSkills()[:1]})

	request, err := builder.Build(t.Context(), sampleInput())
	if err != nil {
		t.Fatalf("cannot build the working context: %v", err)
	}
	persona := blockNamed(t, request, BlockPersona)
	if !strings.Contains(persona.Text, "qa: Walks an app") {
		t.Errorf("a home with a skill and no SOUL.md tells the model nothing about the skill:\n%s", persona.Text)
	}
	if strings.HasPrefix(persona.Text, "\n") || strings.HasSuffix(persona.Text, "\n") {
		t.Errorf("the persona block has a blank edge where SOUL.md would have been: %q", persona.Text)
	}
}

// TestAnEmptySkillListWritesNothing proves a home with no skills pays nothing for
// the list: no heading, and with no SOUL.md either, no persona block at all.
func TestAnEmptySkillListWritesNothing(t *testing.T) {
	withSoul := newTestBuilder(t, Options{})
	writePersonaFile(t, withSoul.home.SoulFile(), "I am Coeus.")
	request, err := withSoul.Build(t.Context(), sampleInput())
	if err != nil {
		t.Fatalf("cannot build the working context: %v", err)
	}
	if persona := blockNamed(t, request, BlockPersona); strings.Contains(persona.Text, skillsHeading) {
		t.Errorf("a home with no skills is told about a skill list:\n%s", persona.Text)
	}

	bare := newTestBuilder(t, Options{})
	request, err = bare.Build(t.Context(), sampleInput())
	if err != nil {
		t.Fatalf("cannot build the working context: %v", err)
	}
	for _, block := range request.SystemBlocks {
		if block.Name == BlockPersona {
			t.Errorf("a home with no SOUL.md and no skills has a persona block, and an empty block is refused on the wire: %q", block.Text)
		}
	}
	if text := skillsText(nil); text != "" {
		t.Errorf("an empty skill list writes %q, want nothing", text)
	}
}

// TestTheSkillListIsCappedInLengthAndInLineWidth is the bound: a skills folder of
// two hundred entries, each with the longest description the store allows, would
// otherwise put forty thousand characters in front of the model on every call.
// Twenty skills and two hundred characters a line is the most the list may cost.
func TestTheSkillListIsCappedInLengthAndInLineWidth(t *testing.T) {
	many := []contract.SkillSummary{}
	for at := range MaxSkillsInPrompt + 5 {
		many = append(many, contract.SkillSummary{
			Name:        fmt.Sprintf("skill-%02d", at),
			Description: strings.Repeat("é", 300),
		})
	}

	text := skillsText(many)
	lines := strings.Split(text, "\n")
	if got := len(lines) - 1; got != MaxSkillsInPrompt {
		t.Errorf("the list holds %d skill lines, want %d, because the cap is what keeps a big folder out of the prompt", got, MaxSkillsInPrompt)
	}
	if lines[0] != skillsHeading {
		t.Errorf("the first line is %q, want the heading", lines[0])
	}
	for _, line := range lines[1:] {
		if width := utf8.RuneCountInString(line); width > MaxSkillLineRunes {
			t.Errorf("a skill line is %d characters wide and the most allowed is %d", width, MaxSkillLineRunes)
		}
		if !utf8.ValidString(line) {
			t.Errorf("a skill line was cut in the middle of a character: %q", line)
		}
		if !strings.HasPrefix(line, "skill-") {
			t.Errorf("a skill line does not begin with the skill's name: %q", line)
		}
	}
	if strings.Contains(text, "skill-20") {
		t.Error("the twenty-first skill is in the list, so the cap did not hold")
	}
}

// TestTheSkillListDoesNotMoveDuringATask holds the cache promise for the new
// block: the list is read when the builder is made, so nothing that happens
// during the task changes the bytes above boundary A.
func TestTheSkillListDoesNotMoveDuringATask(t *testing.T) {
	run := newFixtureRun(t)
	builder := newTestBuilder(t, Options{Home: roomyHome(t), Boundary: goldenBoundary, Skills: twoSkills()})

	run.playTo(t, 2)
	first, err := builder.Build(t.Context(), run.input(24000))
	if err != nil {
		t.Fatalf("cannot build the working context at round 2: %v", err)
	}
	run.playTo(t, 11)
	later, err := builder.Build(t.Context(), run.input(24000))
	if err != nil {
		t.Fatalf("cannot build the working context at round 11: %v", err)
	}
	if aboveTheCacheLine(first) != aboveTheCacheLine(later) {
		t.Error("what is above the cache line changed between rounds 2 and 11 with a skill list in it")
	}
	if !strings.Contains(aboveTheCacheLine(later), "tidy-notes: Keeps the notes folder tidy") {
		t.Error("the skill list is not above the cache line at round 11")
	}
}

// blockNamed finds one system block by name, and fails the test when the prompt
// has no such block.
func blockNamed(t *testing.T, request contract.Request, name string) contract.SystemBlock {
	t.Helper()
	for _, block := range request.SystemBlocks {
		if block.Name == name {
			return block
		}
	}
	t.Fatalf("the system prompt has no %q block: %s", name, blockNames(request))
	return contract.SystemBlock{}
}

// TestTheSkillListReachesTheModelThroughTheFakeStore proves the shape the
// wiring hands over is the one the store's List returns, so that the two cannot
// drift apart.
func TestTheSkillListReachesTheModelThroughTheFakeStore(t *testing.T) {
	store := testkit.NewFakeSkill()
	store.Add(contract.SkillSummary{Name: "qa", Description: "Walks an app step by step."}, "the body")
	listed, err := store.List(t.Context())
	if err != nil {
		t.Fatalf("cannot list the fake store: %v", err)
	}

	builder := newTestBuilder(t, Options{Skills: listed})
	request, err := builder.Build(t.Context(), sampleInput())
	if err != nil {
		t.Fatalf("cannot build the working context: %v", err)
	}
	if !strings.Contains(blockNamed(t, request, BlockPersona).Text, "qa: Walks an app step by step.") {
		t.Error("what the store lists did not reach the persona block")
	}
}
