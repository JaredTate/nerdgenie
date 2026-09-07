package loop_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// thePage is an architecture page of three sections, as a project keeps one.
const thePage = `# Architecture

## Engine

The board and the pieces live in src/engine.js.

## Hazards

Nothing yet.

## Shell

The page and the keyboard live in src/shell.js.
`

// aReviewedTaskWithAPage is a task that earns its review through a correction,
// in a work folder that holds the page, and a model that answers the four
// questions and then the fifth.
func aReviewedTaskWithAPage(t *testing.T, page string, fifth string) (*harness, string) {
	t.Helper()
	steps := []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		callStep("The user has corrected me.", callFor("c2", "read", `{"path":"brand.md"}`)),
		answerStep("The hazards are wired. What changed: src/hazards.js. What I checked: the tests. What is left: nothing."),
		aReviewReply("Keep the hazard values in the config file."),
		answerStep(fifth),
	}
	built, _ := midTurnHarness(t, steps, "no, keep the values in the config file")
	path := filepath.Join(built.workFolder, loop.ArchitectureFile)
	if page != "" {
		if err := os.WriteFile(path, []byte(page), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return built, path
}

// TestTheReviewAsksWhichArchitectureSectionChanged: after the four questions,
// a task in a folder that holds ARCHITECTURE.md is asked which section it
// changed and what it should say now, and the answer replaces that section,
// dated, with the other sections untouched.
func TestTheReviewAsksWhichArchitectureSectionChanged(t *testing.T) {
	built, path := aReviewedTaskWithAPage(t, thePage, "## Hazards\nThe states are NORMAL, DRAGON_WARNING and DRAGON_ATTACK. The machine is raise(kind, piece) in src/hazards.js, and its values live in src/config.js.")

	built.ask(t, "wire the hazards")

	requests := built.model.Requests()
	last := requests[len(requests)-1]
	if !strings.Contains(theTextOf(last), loop.TheFifthQuestion) {
		t.Fatalf("the last request does not carry the fifth question; it reads:\n%s", theTextOf(last))
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	page := string(written)
	for _, want := range []string{"## Hazards\n\nThe states are NORMAL, DRAGON_WARNING and DRAGON_ATTACK.", "src/config.js", "(updated by task 1, 2026-01-10)", "## Engine\n\nThe board and the pieces live in src/engine.js.", "## Shell\n\nThe page and the keyboard live in src/shell.js."} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %q; it reads:\n%s", want, page)
		}
	}
	if strings.Contains(page, "Nothing yet.") {
		t.Errorf("the old hazards body is still on the page:\n%s", page)
	}
}

// TestAnAnswerNamingNoSectionChangesNothing: "none", an empty answer, and an
// answer with no heading line leave the page as it was, byte for byte.
func TestAnAnswerNamingNoSectionChangesNothing(t *testing.T) {
	for _, answer := range []string{"none", "None.", "", "The task changed nothing that the page describes, so there is nothing to say."} {
		built, path := aReviewedTaskWithAPage(t, thePage, answer)
		built.ask(t, "wire the hazards")
		written, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(written) != thePage {
			t.Errorf("the answer %q changed the page:\n%s", answer, string(written))
		}
	}
}

// TestAPlainTaskWithoutAPageIsNotAskedTheFifthQuestion: a task that belongs to
// no job, in a folder with no page, is asked the four questions and no more.
func TestAPlainTaskWithoutAPageIsNotAskedTheFifthQuestion(t *testing.T) {
	built, path := aReviewedTaskWithAPage(t, "", "## Hazards\nA section nobody asked for.")

	built.ask(t, "wire the hazards")

	for _, request := range built.model.Requests() {
		if strings.Contains(theTextOf(request), loop.TheFifthQuestion) {
			t.Fatalf("a plain task in a folder with no page was asked the fifth question")
		}
	}
	if _, err := os.Stat(path); err == nil {
		t.Errorf("a page was written for a plain task in a folder that had none")
	}
}

// TestALongAnswerIsCutAtTwoHundredWords: a body past two hundred words is cut
// at a sentence end, and the dated line says so.
func TestALongAnswerIsCutAtTwoHundredWords(t *testing.T) {
	sentence := "The engine steps the board once a tick and locks a piece when it lands. "
	long := "## Engine\n" + strings.Repeat(sentence, 30)
	built, path := aReviewedTaskWithAPage(t, thePage, long)

	built.ask(t, "wire the hazards")

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	page := string(written)
	start := strings.Index(page, "## Engine")
	end := strings.Index(page, "## Hazards")
	if start < 0 || end < 0 {
		t.Fatalf("the page lost its sections:\n%s", page)
	}
	body := page[start:end]
	if words := len(strings.Fields(body)); words > loop.MaxSectionWords+12 {
		t.Errorf("the engine section holds %d words, want it cut at about %d", words, loop.MaxSectionWords)
	}
	if !strings.Contains(body, "cut at two hundred words") {
		t.Errorf("the dated line does not say the answer was cut:\n%s", body)
	}
	if !strings.Contains(body, "lands.\n") && !strings.Contains(body, "lands. ") {
		t.Errorf("the cut did not land on a sentence end:\n%s", body)
	}
}

// aReviewedJobTask makes a job whose one task earns its review with a
// correction, in a folder with no page.
func aReviewedJobTask(t *testing.T, fifth string) (*harness, string) {
	t.Helper()
	built, _ := aReviewedTaskWithAPage(t, "", fifth)
	jobID, err := built.jobs.Create(t.Context(), contract.NewJob{Ask: "wire the hazards", Name: "Hazards", Why: "the game needs them"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := built.jobs.AddTask(t.Context(), contract.NewTask{JobID: jobID, Text: "wire the hazards"}); err != nil {
		t.Fatal(err)
	}
	return built, filepath.Join(built.workFolder, loop.ArchitectureFile)
}

// TestAFolderWithoutAnArchitecturePageGetsOneFromAJobTask: a job's task is
// asked the fifth question even when the folder has no page, and an answer that
// names a section starts the page with a title and that section.
func TestAFolderWithoutAnArchitecturePageGetsOneFromAJobTask(t *testing.T) {
	built, path := aReviewedJobTask(t, "## Hazards\nThe states and the machine live in src/hazards.js.")

	runTheJobToTheEnd(t, built.loop, built.channel)

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no page was written for the job's task: %v", err)
	}
	page := string(written)
	if !strings.HasPrefix(page, "# Architecture\n") || !strings.Contains(page, "## Hazards\n\nThe states and the machine live in src/hazards.js.") {
		t.Errorf("the new page reads:\n%s", page)
	}
}

// theTextOf joins every message of a request, so a test can look for a line.
func theTextOf(request contract.Request) string {
	var parts []string
	for _, block := range request.SystemBlocks {
		parts = append(parts, block.Text)
	}
	for _, message := range request.Messages {
		parts = append(parts, message.Text)
	}
	return strings.Join(parts, "\n")
}
