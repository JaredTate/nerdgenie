package loop_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/loop"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// questionEventsOf reads every question the harness asked with the tools off
// out of the log, each as its purpose, question, answer and outcome, with the
// task it was asked under.
func questionEventsOf(t *testing.T, built *harness) []map[string]string {
	t.Helper()
	events, err := built.store.ByKind(t.Context(), contract.EventQuestion)
	if err != nil {
		t.Fatalf("cannot read the question events: %v", err)
	}
	var read []map[string]string
	for _, event := range events {
		body := map[string]string{}
		if err := json.Unmarshal(event.Body, &body); err != nil {
			t.Fatalf("the question event %d does not unmarshal: %v", event.Sequence, err)
		}
		body["task"] = event.TaskID
		read = append(read, body)
	}
	return read
}

// TestEveryToolsOffQuestionAndAnswerIsLogged: the four questions and the
// section question are each written into the log under the task, with the
// answer and what the harness did with it, so that a page that was not
// written can be traced to the answer that failed to name a section. On run
// eighteen the page was missing after two tasks and nobody could see why.
func TestEveryToolsOffQuestionAndAnswerIsLogged(t *testing.T) {
	built, _ := aReviewedTaskWithAPage(t, thePage, "## Hazards\nThe states live in src/hazards.js.")

	built.ask(t, "wire the hazards")

	asked := questionEventsOf(t, built)
	if len(asked) != 2 {
		t.Fatalf("the log holds %d question events, want the review and the section question:\n%v", len(asked), asked)
	}
	review, section := asked[0], asked[1]
	if review["purpose"] != "review" || review["question"] != loop.TheFourQuestions || !strings.Contains(review["answer"], "Keep the hazard values in the config file.") || review["task"] != "1" {
		t.Errorf("the review's event reads %v", review)
	}
	if !strings.Contains(review["outcome"], "lesson") {
		t.Errorf("the review's event does not say what became of the answer: %v", review)
	}
	if section["purpose"] != "architecture section" || section["question"] != loop.TheFifthQuestion || !strings.HasPrefix(section["answer"], "## Hazards") || section["task"] != "1" {
		t.Errorf("the section question's event reads %v", section)
	}
	if section["outcome"] != "wrote ARCHITECTURE.md, section Hazards" {
		t.Errorf("the section question's outcome reads %q, want the section it wrote", section["outcome"])
	}
}

// TestAnAnswerThatNamesNoSectionIsLoggedWithNoSection: the answer "none" is in
// the log too, with the outcome saying no section was written.
func TestAnAnswerThatNamesNoSectionIsLoggedWithNoSection(t *testing.T) {
	built, _ := aReviewedTaskWithAPage(t, thePage, "none")

	built.ask(t, "wire the hazards")

	asked := questionEventsOf(t, built)
	if len(asked) != 2 || asked[1]["answer"] != "none" || asked[1]["outcome"] != "no section" {
		t.Errorf("the section question's event reads %v", asked)
	}
}

// TestAPlainFirstLineIsTheSectionsHeading: a model that writes the part's
// name on the first line without a hash mark has still named the section, so
// the heading is that line. On run eighteen the first tasks' answers had no
// hash mark and the page stayed unwritten.
func TestAPlainFirstLineIsTheSectionsHeading(t *testing.T) {
	built, path := aReviewedTaskWithAPage(t, thePage, "Hazards\nThe states are NORMAL, DRAGON_WARNING and DRAGON_ATTACK, raised in src/hazards.js.")

	built.ask(t, "wire the hazards")

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	page := string(written)
	if !strings.Contains(page, "## Hazards\n\nThe states are NORMAL, DRAGON_WARNING and DRAGON_ATTACK, raised in src/hazards.js.") {
		t.Errorf("the plain first line did not become the heading; the page reads:\n%s", page)
	}
	if strings.Contains(page, "Nothing yet.") {
		t.Errorf("the old hazards body is still on the page:\n%s", page)
	}
}

// TestASentenceFirstLineGoesUnderTheTasksName: an answer that is a paragraph
// with no heading at all is a section under the task's own name, with the
// whole answer as its body, because a paragraph about the work is worth more
// than an empty page.
func TestASentenceFirstLineGoesUnderTheTasksName(t *testing.T) {
	answer := "The hazards are raised by raise(kind, piece) in src/hazards.js, once a tick.\nTheir values live in src/config.js."
	built, path := aReviewedTaskWithAPage(t, thePage, answer)

	built.ask(t, "wire the hazards")

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	page := string(written)
	if !strings.Contains(page, "## Wire the hazards\n\n"+answer) {
		t.Errorf("the paragraph was not put under the task's name; the page reads:\n%s", page)
	}
	for _, kept := range []string{"## Engine", "## Hazards\n\nNothing yet.", "## Shell"} {
		if !strings.Contains(page, kept) {
			t.Errorf("the page lost %q:\n%s", kept, page)
		}
	}
}

// TestTheReportSaysWhichSectionWasWritten: the task's report carries the line
// "wrote ARCHITECTURE.md, section Hazards" for the person, and the job keeps
// it in the report the next task reads above its record, so the next task
// knows which section exists before it reads anything.
func TestTheReportSaysWhichSectionWasWritten(t *testing.T) {
	steps := []testkit.Step{
		callStep("I will read the notes.", callFor("c1", "read", `{"path":"notes.md"}`)),
		callStep("The user has corrected me.", callFor("c2", "read", `{"path":"brand.md"}`)),
		answerStep("The hazards are wired. What changed: src/hazards.js. What I checked: the tests. What is left: nothing."),
		aReviewReply("Keep the hazard values in the config file."),
		answerStep("## Hazards\nThe states live in src/hazards.js."),
		answerStep("The yeti is wired. What changed: src/yeti.js. What I checked: the tests. What is left: nothing."),
		answerStep("none"),
	}
	built, _ := midTurnHarness(t, steps, "no, keep the values in the config file")
	jobID, err := built.jobs.Create(t.Context(), contract.NewJob{Ask: "wire the hazards", Name: "Hazards", Why: "the game needs them"})
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"wire the hazards", "wire the yeti"} {
		if _, err := built.jobs.AddTask(t.Context(), contract.NewTask{JobID: jobID, Text: text}); err != nil {
			t.Fatal(err)
		}
	}

	runTheJobToTheEnd(t, built.loop, built.channel)

	told := strings.Join(built.channel.Sent(), "\n")
	if !strings.Contains(told, "wrote ARCHITECTURE.md, section Hazards") {
		t.Errorf("the person was not told which section was written; the channel got:\n%s", told)
	}
	secondTask := ""
	for _, request := range built.model.Requests() {
		if text := theTextOf(request); strings.Contains(text, "1 of 2 tasks done") {
			secondTask = text
			break
		}
	}
	if !strings.Contains(secondTask, "wrote ARCHITECTURE.md, section Hazards") {
		t.Errorf("the second task's front does not carry the first task's section line; it reads:\n%s", secondTask)
	}
}
