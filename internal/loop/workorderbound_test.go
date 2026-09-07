package loop_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/loop"
)

// The sections a task names ride under the job summary on every call of the
// task, so they are bounded: a section past MaxInlinedDetailWords, and any
// named section past the third, is shown by heading with the way to read it
// instead, and the job record's task line keeps its pointer exactly as
// written.

// aWorkOrderWithSections is the notes work order with its Details replaced
// by the sections given, as heading and body pairs, and its first task
// naming the headings given.
func aWorkOrderWithSections(named string, sections ...string) string {
	var details strings.Builder
	for at := 0; at+1 < len(sections); at += 2 {
		details.WriteString("### " + sections[at] + "\n" + sections[at+1] + "\n\n")
	}
	ask := strings.Replace(aWorkOrderAsk, "(Details: Storage)", "(Details: "+named+")", 1)
	from := strings.Index(ask, "## Details")
	return ask[:from] + "## Details\n" + details.String()
}

func aBodyOfWords(count int) string {
	words := make([]string, count)
	for at := range words {
		words[at] = "word"
	}
	return strings.Join(words, " ") + "."
}

func firstRequestOfTheJobTask(t *testing.T, ask string) (*harness, string) {
	t.Helper()
	built := newHarness(t, closingScript("the notes are read"), scriptedTool("read", "the notes"))
	built.ask(t, ask)
	if _, err := built.loop.RunNextJobTask(t.Context(), built.channel); err != nil {
		t.Fatalf("the job's first task did not run: %v", err)
	}
	return built, wholeRequestText(built.model.Requests()[0])
}

func TestAShortNamedSectionRidesInFullAndALongOneByHeading(t *testing.T) {
	short := aBodyOfWords(60)
	long := aBodyOfWords(loop.MaxInlinedDetailWords + 150)
	built, first := firstRequestOfTheJobTask(t, aWorkOrderWithSections("Storage, List", "Storage", short, "List", long))
	if !strings.Contains(first, "### Storage\n"+short) {
		t.Errorf("the sixty-word section the task names does not ride in full under the task")
	}
	if strings.Contains(first, long) {
		t.Errorf("a section past %d words rides in full; it must be shown by heading", loop.MaxInlinedDetailWords)
	}
	if !strings.Contains(first, loop.TheLongSectionsLine+" List") {
		t.Errorf("the long section the task names is not listed with the way to read it; the request reads:\n%s", first)
	}
	if text := built.jobs.Tasks("1")[0].Text; !strings.HasSuffix(text, "(Details: Storage, List)") {
		t.Errorf("the job record's task line reads %q, want its pointer kept exactly as written", text)
	}
}

func TestAtMostThreeNamedSectionsRideInFull(t *testing.T) {
	_, first := firstRequestOfTheJobTask(t, aWorkOrderWithSections("One, Two, Three, Four",
		"One", "The first body.", "Two", "The second body.", "Three", "The third body.", "Four", "The fourth body."))
	for _, body := range []string{"The first body.", "The second body.", "The third body."} {
		if !strings.Contains(first, body) {
			t.Errorf("the request lacks the named section %q, which is within the first %d", body, loop.MaxInlinedDetails)
		}
	}
	if strings.Contains(first, "The fourth body.") || !strings.Contains(first, loop.TheLongSectionsLine+" Four") {
		t.Errorf("the fourth named section is not shown by heading alone; the request reads:\n%s", first)
	}
}

func TestAnUnknownNamedHeadingIsLeftAsWritten(t *testing.T) {
	built, first := firstRequestOfTheJobTask(t, strings.Replace(aWorkOrderAsk, "(Details: Storage)", "(Details: Storage, Nowhere)", 1))
	if !strings.Contains(first, "### Storage\nNotes live in local storage under one key.") {
		t.Errorf("the named section that exists does not ride in full")
	}
	if strings.Contains(first, "### Nowhere") || strings.Contains(first, "Nowhere\n") || strings.Contains(first, ", Nowhere\n") {
		t.Errorf("a heading the ask does not have was shown as a section or listed; the request reads:\n%s", first)
	}
	if text := built.jobs.Tasks("1")[0].Text; !strings.HasSuffix(text, "(Details: Storage, Nowhere)") {
		t.Errorf("the job record's task line reads %q, want the pointer left exactly as written", text)
	}
}
