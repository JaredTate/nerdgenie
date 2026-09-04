package memory

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// aFactAt returns a fact recorded at a fixed moment, so that every line these
// tests build has the same date in it.
func aFactAt(text string, source string) contract.Fact {
	return contract.Fact{
		ID:       "m7",
		Text:     text,
		Source:   source,
		Recorded: time.Date(2026, 9, 2, 14, 0, 0, 0, time.UTC),
	}
}

func TestAFactLineHoldsTheIDTheDateTheSourceAndTheText(t *testing.T) {
	line := formatFactLine(aFactAt("the user leads with the date", "task 17"))
	want := "- m7 [2026-09-02T14:00:00Z | task 17] the user leads with the date"
	if line != want {
		t.Fatalf("the fact line is %q, want %q", line, want)
	}
}

func TestAFactLineNamesTheFactItSupersedes(t *testing.T) {
	fact := aFactAt("the user leads with the features", "task 17")
	fact.Supersedes = "m3"
	line := formatFactLine(fact)
	if !strings.Contains(line, "| supersedes m3]") {
		t.Fatalf("the fact line is %q, and it must name the fact it supersedes", line)
	}
	parsed, ok := parseFactLine(line)
	if !ok {
		t.Fatalf("the line %q did not parse back", line)
	}
	if parsed.Supersedes != "m3" {
		t.Errorf("the parsed fact supersedes %q, want %q", parsed.Supersedes, "m3")
	}
}

func TestAFactLineParsesBackIntoTheSameFact(t *testing.T) {
	fact := aFactAt("the DigiByte anniversary is on the tenth of January", "browser x.com")
	parsed, ok := parseFactLine(formatFactLine(fact))
	if !ok {
		t.Fatalf("the line for %+v did not parse back", fact)
	}
	if parsed.ID != fact.ID || parsed.Text != fact.Text || parsed.Source != fact.Source {
		t.Errorf("the fact came back as %+v, want %+v", parsed, fact)
	}
	if !parsed.Recorded.Equal(fact.Recorded) {
		t.Errorf("the date came back as %s, want %s", parsed.Recorded, fact.Recorded)
	}
}

func TestALineThatIsNotAFactDoesNotParse(t *testing.T) {
	notFacts := []string{
		"",
		"# a heading somebody wrote by hand",
		"- m7 the brackets are missing",
		"- m7 [not a date | task 17] the text",
		"- m7 [2026-09-02T14:00:00Z] the source is missing",
		"- m7 [2026-09-02T14:00:00Z | task 17 | replaces m3] the third field is wrong",
		"- m7 [2026-09-02T14:00:00Z | task 17 | supersedes ] the superseded id is missing",
		"- m7 [2026-09-02T14:00:00Z | task 17]",
		"- [2026-09-02T14:00:00Z | task 17] the id is missing",
		"- m 7 [2026-09-02T14:00:00Z | task 17] the id has a space in it",
		"m7 [2026-09-02T14:00:00Z | task 17] the dash is missing",
	}
	for _, line := range notFacts {
		if _, ok := parseFactLine(line); ok {
			t.Errorf("the line %q parsed as a fact, and it is not one", line)
		}
	}
}

func TestAFactLineIsWrittenOnOneLineWithNoSeparatorsInItsFields(t *testing.T) {
	fact := aFactAt("first line\nsecond line\ttabbed", "a | source] with separators")
	line := formatFactLine(fact)
	if strings.ContainsAny(line, "\n\t") {
		t.Fatalf("the fact line %q carries a line break or a tab, and it has to fit on one line", line)
	}
	parsed, ok := parseFactLine(line)
	if !ok {
		t.Fatalf("the line %q did not parse back", line)
	}
	if strings.Contains(parsed.Source, "|") || strings.Contains(parsed.Source, "]") {
		t.Errorf("the source came back as %q, and the separators must be taken out of it", parsed.Source)
	}
	if parsed.Text != "first line second line tabbed" {
		t.Errorf("the text came back as %q, want the three parts joined by spaces", parsed.Text)
	}
}

func TestAnIDIsRefusedWhenItCouldNotBeWrittenOnAFactLine(t *testing.T) {
	good := []string{"m7", "u12", "contract-check-1", "c17-9", "a.b_c"}
	for _, id := range good {
		if !validFactID(id) {
			t.Errorf("the id %q was refused, and it is a name a fact line can hold", id)
		}
	}
	bad := []string{"", "m 7", "note:x", "m|7", "m]7", "m\n7", strings.Repeat("m", maxFactIDRunes+1)}
	for _, id := range bad {
		if validFactID(id) {
			t.Errorf("the id %q was accepted, and a fact line could not hold it", id)
		}
	}
}
