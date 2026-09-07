package loop

import "testing"

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
	} {
		if got := theFallbackHeading(shape.name, shape.ask); got != shape.want {
			t.Errorf("the fallback heading of name %q and ask %q reads %q, want %q", shape.name, shape.ask, got, shape.want)
		}
	}
}
