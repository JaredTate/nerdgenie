package workorder_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/workorder"
)

func TestReadCheckReadsTheFourKindsOffTheEndOfADoneLine(t *testing.T) {
	cases := []struct {
		line     string
		kind     string
		argument string
		text     string
		url      string
	}{
		{"Every test passes and none is skipped. [tests pass: npm test]", "tests pass", "npm test", "", ""},
		{"The build is clean. [exit 0: node build.js]", "exit 0", "node build.js", "", ""},
		{`The game loads. [shows: "Tater Tots Tetris" at http://127.0.0.1:8091]`, "shows", `"Tater Tots Tetris" at http://127.0.0.1:8091`, "Tater Tots Tetris", "http://127.0.0.1:8091"},
		{"The bundle is written. [exists: dist/index.html]", "exists", "dist/index.html", "", ""},
		{"1. Every test passes. [Tests Pass:  npm test ]  ", "tests pass", "npm test", "", ""},
	}
	for _, one := range cases {
		check, found := workorder.ReadCheck(one.line)
		if !found {
			t.Errorf("ReadCheck(%q) found no check", one.line)
			continue
		}
		if check.Kind != one.kind || check.Argument != one.argument || check.Text != one.text || check.URL != one.url {
			t.Errorf("ReadCheck(%q) = %+v, want kind %q argument %q text %q url %q", one.line, check, one.kind, one.argument, one.text, one.url)
		}
	}
}

func TestALineWithoutACheckOrWithAKindItDoesNotKnowIsNotACheck(t *testing.T) {
	for _, line := range []string{
		"A whole game has been played to game over.",
		"The layout was looked at [at five sizes].",
		"The tests pass [passes: npm test]",
		"Nothing here [tests pass]",
		"Nothing here [: npm test]",
		`Shows without a place [shows: "board"]`,
		"",
	} {
		if check, found := workorder.ReadCheck(line); found {
			t.Errorf("ReadCheck(%q) read a check %+v, want none", line, check)
		}
	}
}

func TestReadCheckHandsBackTheLineWithoutItsCheck(t *testing.T) {
	check, found := workorder.ReadCheck("Every test passes. [tests pass: npm test]")
	if !found || check.Line != "Every test passes." {
		t.Errorf("the line without its check is %q, want the words before the bracket", check.Line)
	}
}

func FuzzReadCheck(f *testing.F) {
	f.Add("Every test passes. [tests pass: npm test]")
	f.Add(`The game loads. [shows: "Tater Tots" at http://x]`)
	f.Add("[exists: ]")
	f.Add("no check")
	f.Fuzz(func(t *testing.T, line string) {
		check, found := workorder.ReadCheck(line)
		if !found {
			return
		}
		if !strings.Contains(strings.ToLower(line), check.Kind) {
			t.Errorf("the kind %q is not in the line %q", check.Kind, line)
		}
		if strings.TrimSpace(check.Argument) == "" {
			t.Errorf("a check with an empty argument was read from %q", line)
		}
	})
}
