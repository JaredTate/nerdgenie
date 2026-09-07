package loop

import (
	"strings"
	"testing"
)

// TestTheSectionAnswerIsReadInEveryShapeAModelWrites: the heading is a hash
// line, a plain short line, or a bold line; a paragraph with no heading goes
// under the fallback name; and "none", a numbered list, a question and a
// one-line sentence saying nothing changed are no section at all.
func TestTheSectionAnswerIsReadInEveryShapeAModelWrites(t *testing.T) {
	for _, shape := range []struct {
		name, answer, heading, body string
		found                       bool
	}{
		{"a hash heading", "## Hazards\nThe states live in src/hazards.js.", "Hazards", "The states live in src/hazards.js.", true},
		{"a plain short line", "Hazards\nThe states live in src/hazards.js.", "Hazards", "The states live in src/hazards.js.", true},
		{"a bold line with a colon", "**Board and rules:**\nThe board is a nine-cell array in game.js.", "Board and rules", "The board is a nine-cell array in game.js.", true},
		{"a sentence first line", "The hazards are raised by raise(kind, piece) in src/hazards.js, once a tick.\nTheir values live in src/config.js.", "Wire the hazards", "The hazards are raised by raise(kind, piece) in src/hazards.js, once a tick.\nTheir values live in src/config.js.", true},
		{"a one-line paragraph", "The server in server.js serves the folder on port 8096 and answers /health with ok.", "Wire the hazards", "The server in server.js serves the folder on port 8096 and answers /health with ok.", true},
		{"none", "none", "", "", false},
		{"none with a stop", "None.", "", "", false},
		{"none in a sentence", "None of the sections changed.", "", "", false},
		{"nothing to say", "The task changed nothing that the page describes, so there is nothing to say.", "", "", false},
		{"empty", "", "", "", false},
		{"a heading with no body", "## Hazards", "", "", false},
		{"a numbered list", "1. The notes were to be read.\n2. They were read.", "", "", false},
		{"a question", "Which section do you mean?\nI cannot tell.", "", "", false},
		// Run 19, task 1: the model wanted the tools and wrote their markup.
		{"tool markup", "I'll start by looking at what was built to write an accurate architecture section.\n\n<tool_call>\n<function=read>\n/home/user/Desktop/Tic Tac Toe/package.json\n</function>\n</tool_call>", "", "", false},
		{"narration first", "I'll start by looking at what was built.\nThe scaffold holds package.json and server.js.", "", "", false},
		{"narration about the questions", "The user wants me to answer the question about the architecture section. Let me review the record.", "", "", false},
		{"let me first", "Let me look at the files first.", "", "", false},
		// Run 20, task 1: a sentence of preamble, then the section itself.
		{"a preamble then a hash heading", "Scaffold done — all three tests pass. Here's the first section for ARCHITECTURE.md:\n\n## Scaffold\n\nThe scaffold is the project's runnable shell.", "Scaffold", "The scaffold is the project's runnable shell.", true},
		{"narration then a hash heading", "Let me write the section.\n\n## Scaffold\nThe scaffold is the shell.\nIt lives in server.js.", "Scaffold", "The scaffold is the shell.\nIt lives in server.js.", true},
		{"a preamble then a heading with no body", "Here is the section:\n## Scaffold", "", "", false},
	} {
		heading, body, found := readTheSectionAnswer(shape.answer, "Wire the hazards")
		if found != shape.found || heading != shape.heading || body != shape.body {
			t.Errorf("%s: the answer %q reads heading %q, body %q, found %v; want %q, %q, %v", shape.name, shape.answer, heading, body, found, shape.heading, shape.body, shape.found)
		}
	}
}

// TestAParagraphWithNoFallbackNameIsNoSection: a paragraph with no heading
// and no task name to put it under has nowhere to go.
func TestAParagraphWithNoFallbackNameIsNoSection(t *testing.T) {
	if _, _, found := readTheSectionAnswer("The server in server.js serves the folder on port 8096 and answers /health with ok.", ""); found {
		t.Error("a paragraph with no name to go under was read as a section")
	}
}

// TestTheFallbackHeadingIsTheTasksName: the record's name when it has one,
// else the first words of the ask with a capital letter, cut to eight words.
func TestTheFallbackHeadingIsTheTasksName(t *testing.T) {
	for _, shape := range []struct{ name, ask, want string }{
		{"Hazards", "wire the hazards", "Hazards"},
		{"", "wire the hazards", "Wire the hazards"},
		{"", "make the board, the pieces, the rules, the scoring, the page and the tests", "Make the board, the pieces, the rules, the"},
		{"", "", ""},
		// Run 19, task 1: a job task's name is its whole line, so the heading
		// is what comes before the colon, without the backticks, cut to the cap.
		{"Scaffold: `package.json`, a test runner, `index.html`, one smoke test", "", "Scaffold"},
		{"Make the `board`, the pieces, the rules, the scoring, the page and the tests", "", "Make the board, the pieces, the rules, the"},
		{": nothing before the colon", "", "Nothing before the colon"},
	} {
		if got := theFallbackHeading(shape.name, shape.ask); got != shape.want {
			t.Errorf("the fallback heading of name %q and ask %q reads %q, want %q", shape.name, shape.ask, got, shape.want)
		}
	}
}

// TestTheArchitectureQuestionsSayTheToolsAreOff: run 19's first task
// answered the section question by asking for the read tool, because nothing
// told it the tools were off; both questions now say so and say to answer
// from the record.
func TestTheArchitectureQuestionsSayTheToolsAreOff(t *testing.T) {
	// Run 20, task 3: the review's four questions got tool markup back too.
	for _, question := range []string{TheFifthQuestion, TheFirstSectionQuestion, TheFourQuestions} {
		if !strings.Contains(question, TheToolsAreOffLine) {
			t.Errorf("the question %q does not say the tools are off", question)
		}
	}
}

// TestTheLogSaysWhyAnAnswerGaveNoSection: the outcome names the refusal, so
// a run's log shows whether the model declined or wrote something else.
func TestTheLogSaysWhyAnAnswerGaveNoSection(t *testing.T) {
	for _, shape := range []struct{ answer, want string }{
		{"none", "no section"},
		{"I'll read the files.\n<tool_call>\n<function=read>\n</function>\n</tool_call>", "no section: the answer was tool markup"},
		{"Let me look at the files first.", "no section: the answer narrated instead of answering"},
	} {
		if got := theReasonForNoSection(shape.answer); got != shape.want {
			t.Errorf("the reason for %q reads %q, want %q", shape.answer, got, shape.want)
		}
	}
}
