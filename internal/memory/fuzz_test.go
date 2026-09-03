package memory

import (
	"strings"
	"testing"
)

func FuzzParseFactLine(f *testing.F) {
	f.Add("- m7 [2026-09-02T14:00:00Z | task 17] the user leads with the date")
	f.Add("- m7 [2026-09-02T14:00:00Z | task 17 | supersedes m3] the newer fact")
	f.Add("- u1 [1970-01-01T00:00:01Z | ] a fact with no source at all")
	f.Add("# a heading somebody wrote by hand")
	f.Add("- m7 [not a date | task 17] the date is wrong")
	f.Add("- m7 [2026-09-02T14:00:00.5+02:00 | task 17] the date has a fraction and an offset")
	f.Add("")
	f.Add("- ")
	f.Add("- \x00 [\x00 | \x00] \x00")

	f.Fuzz(func(t *testing.T, line string) {
		fact, isFact := parseFactLine(line)
		if !isFact {
			return
		}
		if !validFactID(fact.ID) {
			t.Fatalf("the line %q parsed into a fact whose id %q could not be written back", line, fact.ID)
		}
		if fact.Text == "" {
			t.Fatalf("the line %q parsed into a fact with no text", line)
		}
		written := formatFactLine(fact)
		if strings.ContainsAny(written, "\n\r") {
			t.Fatalf("the fact from %q is written as %q, which is more than one line", line, written)
		}
		again, isFactAgain := parseFactLine(written)
		if !isFactAgain {
			t.Fatalf("the fact from %q is written as %q, which does not parse back", line, written)
		}
		if again.ID != fact.ID || again.Text != fact.Text || again.Source != fact.Source ||
			again.Supersedes != fact.Supersedes || !again.Recorded.Equal(fact.Recorded) {
			t.Fatalf("the fact from %q came back as %+v the second time, want %+v", line, again, fact)
		}
	})
}

func FuzzMatchExpression(f *testing.F) {
	f.Add("the anniversary is on the tenth of January")
	f.Add("")
	f.Add("   ")
	f.Add(`"quoted" AND NOT NEAR(a b) OR *`)
	f.Add(strings.Repeat("word ", 200))
	f.Add("\x00�")

	f.Fuzz(func(t *testing.T, query string) {
		expression := matchExpression(query)
		if expression == "" {
			return
		}
		terms := strings.Split(expression, " OR ")
		if len(terms) > maxQueryTokens {
			t.Fatalf("the query %q became %d terms, and the cap is %d", query, len(terms), maxQueryTokens)
		}
		for _, term := range terms {
			if !strings.HasPrefix(term, `"`) || !strings.HasSuffix(term, `"`) {
				t.Fatalf("the term %q from the query %q is not quoted, so the search table would read it as its own", term, query)
			}
			inside := term[1 : len(term)-1]
			if strings.ContainsRune(inside, '"') {
				t.Fatalf("the term %q from the query %q holds a quotation mark of its own", term, query)
			}
			if len([]rune(inside)) > maxTokenRunes {
				t.Fatalf("the term %q from the query %q is %d runes, and the cap is %d", term, query, len([]rune(inside)), maxTokenRunes)
			}
		}
	})
}

func FuzzOneLine(f *testing.F) {
	f.Add("first line\nsecond line\ttabbed")
	f.Add("")
	f.Add("   \n\t  ")
	f.Add("\x00 a fact with a zero byte in it")

	f.Fuzz(func(t *testing.T, text string) {
		flattened := oneLine(text)
		if strings.ContainsAny(flattened, "\n\r\t") {
			t.Fatalf("the text %q flattened to %q, which is more than one line", text, flattened)
		}
		if oneLine(flattened) != flattened {
			t.Fatalf("flattening %q twice gave %q and then %q", text, flattened, oneLine(flattened))
		}
		field := withoutSeparators(text)
		if strings.ContainsAny(field, "|][\n\r\t") {
			t.Fatalf("the field %q still holds one of the line's own marks", field)
		}
	})
}
