package command

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// TestTheShippedSoulNamesNerdGenieAndHisVoice pins the persona a fresh install
// starts with: the agent is Nerd Genie, and when he talks to the person he
// talks like Doc Brown from Back to the Future; what he writes into the record
// and into tool calls stays plain, because those are for the harness.
func TestTheShippedSoulNamesNerdGenieAndHisVoice(t *testing.T) {
	home := contract.NewHome(t.TempDir())
	soul := ""
	for _, file := range personaFiles(home) {
		if file.path == home.SoulFile() {
			soul = file.text
		}
	}
	for _, words := range []string{"Nerd Genie", "Doc Brown", "Back to the Future", "Great Scott", "record", "plain"} {
		if !strings.Contains(soul, words) {
			t.Errorf("the shipped soul says nothing about %q", words)
		}
	}
	if len(soul) > 4000 {
		t.Errorf("the shipped soul is %d bytes, and the context builder reads at most 4000", len(soul))
	}
}

// TestTheShippedConfigurationSaysTheSandboxIsOffAndWhatTheFenceWouldDo pins the
// one setting a person is most likely to want to change. The line is written out
// rather than left to the default, so that whoever opens the file can see which
// way their machine is running and turn it the other way in one edit.
func TestTheShippedConfigurationSaysTheSandboxIsOffAndWhatTheFenceWouldDo(t *testing.T) {
	chosen := modelChoice{name: contract.LocalModelAlias, detected: true, alias: contract.DefaultConfig().Models[0]}
	written := configurationText(chosen, []modelChoice{chosen}, []string{"/home/someone/nerdgenie"})

	lines := strings.Split(written, "\n")
	at := -1
	for offset, line := range lines {
		if strings.HasPrefix(line, "sandbox = ") {
			at = offset
		}
	}
	if at < 0 {
		t.Fatalf("the shipped configuration has no sandbox line at all:\n%s", written)
	}
	if lines[at] != `sandbox = "off"` {
		t.Errorf("the shipped configuration writes %q, want the default written out as %q", lines[at], `sandbox = "off"`)
	}
	if at < 2 || !strings.HasPrefix(lines[at-1], "#") || !strings.HasPrefix(lines[at-2], "#") {
		t.Fatalf("the sandbox line has no two-line comment above it:\n%s", strings.Join(lines[max(at-2, 0):at+1], "\n"))
	}
	comment := lines[at-2] + " " + lines[at-1]
	for _, wanted := range []string{"fence", "sandbox roots"} {
		if !strings.Contains(comment, wanted) {
			t.Errorf("the comment above the sandbox line is %q and does not say %q", comment, wanted)
		}
	}
}

// TestTheShippedConfigurationWritesTheBudgetsCommentedOut is the user's rule in
// the file "nerdgenie init" writes: the three budgets are off unless set, so each
// is written as a comment a person can uncomment, under a [caps] header so that
// the uncommented line lands in the right table, with a comment above saying
// they are off and how to turn one on.
func TestTheShippedConfigurationWritesTheBudgetsCommentedOut(t *testing.T) {
	chosen := modelChoice{name: contract.LocalModelAlias, detected: true, alias: contract.DefaultConfig().Models[0]}
	written := configurationText(chosen, []modelChoice{chosen}, []string{"/home/someone/nerdgenie"})

	caps := strings.Index(written, "[caps]\n")
	models := strings.Index(written, "[[models]]\n")
	if caps < 0 || models < 0 || caps > models {
		t.Fatalf("the [caps] table is at %d and the first model block at %d, and the table has to come first so that its keys stay in it:\n%s", caps, models, written)
	}
	block := written[caps:models]
	for _, line := range []string{"# rounds_per_task = 100\n", "# time_per_task = \"1h\"\n", "# time_per_turn = \"15m\"\n"} {
		if !strings.Contains(block, line) {
			t.Errorf("the caps table does not carry the commented line %q:\n%s", line, block)
		}
	}
	for _, key := range []string{"\nrounds_per_task", "\ntime_per_task", "\ntime_per_turn"} {
		if strings.Contains(written, key) {
			t.Errorf("the shipped configuration sets %q, and every budget is off unless the person turns it on", strings.TrimSpace(key))
		}
	}
	above := written[:caps]
	for _, words := range []string{"off unless you set them", "no cap on its own", "time_per_tool"} {
		if !strings.Contains(above, words) {
			t.Errorf("the comment above the caps table does not say %q:\n%s", words, above)
		}
	}
}

// TestTheShippedConfigurationWritesTheThinkLineWithACommentAboveIt pins the
// think setting in the file "nerdgenie init" writes: every model block carries it,
// empty, with a comment above it naming the levels, so that a person can turn
// one model up and another down in one edit.
// TestTheShippedConfigurationTurnsYoloOn pins that the file "nerdgenie init"
// writes starts the unattended agent with yolo on, with a comment saying how to
// be asked instead. The agent runs where nobody is watching to answer a yes.
func TestTheShippedConfigurationTurnsYoloOn(t *testing.T) {
	chosen := modelChoice{name: contract.LocalModelAlias, detected: true, alias: contract.DefaultConfig().Models[0]}
	written := configurationText(chosen, []modelChoice{chosen}, []string{"/home/someone/nerdgenie"})

	lines := strings.Split(written, "\n")
	at := -1
	for offset, line := range lines {
		if strings.HasPrefix(line, "yolo = ") {
			at = offset
		}
	}
	if at < 0 {
		t.Fatalf("the shipped configuration has no yolo line at all:\n%s", written)
	}
	if lines[at] != "yolo = true" {
		t.Errorf("the shipped configuration writes %q, want yolo on for the unattended agent", lines[at])
	}
	if at < 2 || !strings.HasPrefix(lines[at-1], "#") || !strings.HasPrefix(lines[at-2], "#") {
		t.Fatalf("the yolo line has no comment above it:\n%s", strings.Join(lines[max(at-2, 0):at+1], "\n"))
	}
	comment := lines[at-2] + " " + lines[at-1]
	if !strings.Contains(comment, "unattended") || !strings.Contains(comment, "false") {
		t.Errorf("the comment above the yolo line is %q and does not say it is on for the unattended agent and how to be asked", comment)
	}
}

func TestTheShippedConfigurationWritesTheThinkLineWithACommentAboveIt(t *testing.T) {
	chosen := modelChoice{name: contract.LocalModelAlias, detected: true, alias: contract.DefaultConfig().Models[0]}
	written := configurationText(chosen, []modelChoice{chosen}, []string{"/home/someone/nerdgenie"})

	lines := strings.Split(written, "\n")
	at := -1
	for offset, line := range lines {
		if strings.HasPrefix(line, "think = ") {
			at = offset
		}
	}
	if at < 0 {
		t.Fatalf("the shipped configuration has no think line at all:\n%s", written)
	}
	if lines[at] != `think = ""` {
		t.Errorf("the shipped configuration writes %q, want the empty level %q, which leaves the provider's own default alone", lines[at], `think = ""`)
	}
	if at < 1 || !strings.HasPrefix(lines[at-1], "#") {
		t.Fatalf("the think line has no comment above it:\n%s", strings.Join(lines[max(at-2, 0):at+1], "\n"))
	}
	comment := lines[max(at-3, 0)] + " " + lines[max(at-2, 0)] + " " + lines[at-1]
	for _, level := range contract.ThinkLevels() {
		if !strings.Contains(comment, string(level)) {
			t.Errorf("the comment above the think line is %q and does not name the level %q", comment, level)
		}
	}
}

// TestTheShippedConfigurationCarriesACommentedOutCodexExampleAfterTheCodexBlock
// pins the one example block "nerdgenie init" writes for a model it does not set up
// itself: OpenAI's Codex backend on the ChatGPT subscription, reached with the
// login the codex program keeps, so that Nerd Genie's own loop drives the model. Every
// line of it is a comment, so the file loads exactly as it did without it, and
// a person turns it on by uncommenting it. It sits after the codex program's
// own block, so that the two ways of reaching the same subscription are read
// together.
func TestTheShippedConfigurationCarriesACommentedOutCodexExampleAfterTheCodexBlock(t *testing.T) {
	chosen := modelChoice{name: contract.LocalModelAlias, detected: true, alias: contract.DefaultConfig().Models[0]}
	codex := modelChoice{name: contract.CodexProgram, detected: true, alias: contract.ModelAlias{
		Name: contract.CodexProgram, Provider: contract.ProviderCommandLine, Program: contract.CodexProgram,
		ModelName: "gpt-5.5", ContextLength: cloudContextLength}}
	written := configurationText(chosen, []modelChoice{chosen, codex}, []string{"/home/someone/nerdgenie"})

	programLine := strings.Index(written, "\nprogram = \"codex\"\n")
	if programLine < 0 {
		t.Fatalf("the configuration has no block for the codex program:\n%s", written)
	}
	wanted := []string{
		"# [[models]]",
		"# name = \"gpt\"",
		"# provider = \"codex\"",
		"# model_name = \"gpt-5.6-sol\"",
		"# context_length = 400000",
		"# think = \"medium\"",
	}
	last := programLine
	for _, line := range wanted {
		at := strings.Index(written, "\n"+line+"\n")
		if at < 0 {
			t.Errorf("the configuration has no commented-out line %q:\n%s", line, written)
			continue
		}
		if at < last {
			t.Errorf("the line %q comes before the one above it or before the codex program's block", line)
		}
		last = at
	}
	example := written[programLine:]
	for _, words := range []string{"Codex", "ChatGPT", "login", "own loop", "uncomment"} {
		if !strings.Contains(example, words) {
			t.Errorf("the comment on the example block says nothing about %q:\n%s", words, example)
		}
	}
	for _, line := range strings.Split(written, "\n") {
		if strings.Contains(line, "gpt-5.6-sol") && !strings.HasPrefix(line, "#") {
			t.Errorf("the line %q is not a comment, and the example block must not change what the file loads as", line)
		}
	}
}

// TestTheCodexExampleIsWrittenWhenNoCodexProgramWasFound pins that the example
// is part of every file "nerdgenie init" writes, not only of one on a machine with
// the codex program installed, because the example is how a person learns the
// provider exists.
func TestTheCodexExampleIsWrittenWhenNoCodexProgramWasFound(t *testing.T) {
	chosen := modelChoice{name: contract.LocalModelAlias, detected: true, alias: contract.DefaultConfig().Models[0]}
	written := configurationText(chosen, []modelChoice{chosen}, []string{"/home/someone/nerdgenie"})

	if !strings.Contains(written, "\n# provider = \"codex\"\n") {
		t.Errorf("the configuration written without the codex program has no commented-out codex example:\n%s", written)
	}
}
