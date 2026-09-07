package workorder_test

import (
	"os"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/workorder"
)

const aSmallOrder = `# Notes app

## Goal
A notes app for one person, in the browser. It is for Jared. Because he wants his notes in one place.

## Where
A new, empty folder: /tmp/notes.

## Done when
1. Every test passes. [tests pass: npm test]
2. The page shows the list. [shows: "Notes" at http://127.0.0.1:8091]
3. A note survives a reload.
4. The build is green. [exit 0: npm run build]
5. The bundle exists. [exists: dist/app.js]
6. It feels fast. [speed: under a second]

## Rules
- Plain JavaScript, no framework.
- Serve on 8091; 8090 is in use.

## Tasks
1. Scaffold: package.json, a test runner, one smoke test. Done when the smoke
   test passes. (Details: Storage)
2. The list and the editor. (Details: List, Editor)
3. Reload and polish.

## Details
### Storage
Notes live in local storage under one key.

### List
One line per note, newest first.

### Editor
A textarea and a save button.
`

func TestTheSixHeadingsAreReadWhateverTheirCase(t *testing.T) {
	order := workorder.Parse(strings.ReplaceAll(aSmallOrder, "## Done when", "## DONE WHEN"))
	if !order.IsWorkOrder {
		t.Fatalf("a text with a Goal and a Done when heading is not read as a work order")
	}
	if order.Name != "Notes app" {
		t.Errorf("the name is %q, want the title", order.Name)
	}
	if !strings.HasPrefix(order.Goal, "A notes app for one person") || strings.Contains(order.Goal, "##") {
		t.Errorf("the goal is %q, want the paragraph under Goal", order.Goal)
	}
	if order.Where != "A new, empty folder: /tmp/notes." {
		t.Errorf("where is %q", order.Where)
	}
	if len(order.DoneWhen) != 6 || len(order.Rules) != 2 || len(order.Tasks) != 3 || len(order.Sections) != 3 {
		t.Errorf("read %d done lines, %d rules, %d tasks, %d sections; want 6, 2, 3, 3",
			len(order.DoneWhen), len(order.Rules), len(order.Tasks), len(order.Sections))
	}
}

func TestAPlainAskIsNotAWorkOrder(t *testing.T) {
	for _, text := range []string{
		"read the notes and tell me what they say",
		"# A title\n\n## Goal\nsomething with a goal but no done list\n",
		"## Done when\n1. it works\n",
		"",
	} {
		if order := workorder.Parse(text); order.IsWorkOrder {
			t.Errorf("%q was read as a work order", text)
		}
	}
}

func TestADoneLineKeepsItsCheck(t *testing.T) {
	order := workorder.Parse(aSmallOrder)
	wants := []struct {
		text     string
		kind     string
		argument string
		unknown  string
	}{
		{"Every test passes. [tests pass: npm test]", workorder.CheckTestsPass, "npm test", ""},
		{"The page shows the list. [shows: \"Notes\" at http://127.0.0.1:8091]", workorder.CheckShows, "\"Notes\" at http://127.0.0.1:8091", ""},
		{"A note survives a reload.", "", "", ""},
		{"The build is green. [exit 0: npm run build]", workorder.CheckExitZero, "npm run build", ""},
		{"The bundle exists. [exists: dist/app.js]", workorder.CheckExists, "dist/app.js", ""},
		{"It feels fast. [speed: under a second]", "", "", "speed"},
	}
	for at, want := range wants {
		line := order.DoneWhen[at]
		if line.Text != want.text || line.Check.Kind != want.kind || line.Check.Argument != want.argument || line.UnknownCheck != want.unknown {
			t.Errorf("done line %d reads %+v, want text %q kind %q argument %q unknown %q", at+1, line, want.text, want.kind, want.argument, want.unknown)
		}
	}
}

func TestATaskNamesItsDetailsAndKeepsItsLine(t *testing.T) {
	order := workorder.Parse(aSmallOrder)
	first := order.Tasks[0]
	if first.Text != "Scaffold: package.json, a test runner, one smoke test. Done when the smoke test passes." {
		t.Errorf("the first task's text is %q, want the wrapped line joined and the details taken off", first.Text)
	}
	if strings.Join(first.Details, ",") != "Storage" {
		t.Errorf("the first task names %v, want Storage", first.Details)
	}
	if !strings.HasSuffix(first.Line, "(Details: Storage)") {
		t.Errorf("the first task's line %q lost its details suffix", first.Line)
	}
	if got := strings.Join(order.Tasks[1].Details, ","); got != "List,Editor" {
		t.Errorf("the second task names %q, want List,Editor", got)
	}
	if len(order.Tasks[2].Details) != 0 {
		t.Errorf("the third task names %v, want nothing", order.Tasks[2].Details)
	}
	if got := workorder.DetailsNamedBy("Reload and polish. (details: Editor, Storage)"); strings.Join(got, ",") != "Editor,Storage" {
		t.Errorf("DetailsNamedBy read %v", got)
	}
}

func TestTheRulesAlwaysBeginWithTestsFirst(t *testing.T) {
	order := workorder.Parse(aSmallOrder)
	rules := order.RulesWithTestsFirst()
	if len(rules) != 3 || rules[0] != workorder.TestsFirst || rules[1] != "Plain JavaScript, no framework." {
		t.Errorf("the rules read %q, want tests first and then the two written", rules)
	}
	written := workorder.Parse(strings.Replace(aSmallOrder, "- Plain JavaScript", "- Tests first, always, no exceptions.\n- Plain JavaScript", 1))
	rules = written.RulesWithTestsFirst()
	if len(rules) != 3 || rules[0] != "Tests first, always, no exceptions." {
		t.Errorf("a written tests-first rule was not kept as the first: %q", rules)
	}
}

func TestSectionsAreTheDetailsByHeading(t *testing.T) {
	order := workorder.Parse(aSmallOrder)
	if order.Sections[1].Heading != "List" || !strings.Contains(order.Sections[1].Body, "newest first") {
		t.Errorf("the second section is %+v", order.Sections[1])
	}
	if got := strings.Join(order.Headings(), ", "); got != "Storage, List, Editor" {
		t.Errorf("the headings are %q", got)
	}
}

func TestTheTetrisAskReadsWhole(t *testing.T) {
	text, err := os.ReadFile("testdata/tetris.md")
	if err != nil {
		t.Fatal(err)
	}
	order := workorder.Parse(string(text))
	if !order.IsWorkOrder || order.Name != "Tater Tots Tetris" {
		t.Fatalf("the Tetris ask reads as work order %v named %q", order.IsWorkOrder, order.Name)
	}
	if len(order.Tasks) != 14 || len(order.DoneWhen) != 7 || len(order.Rules) != 8 || len(order.Sections) != 12 {
		t.Errorf("read %d tasks, %d done lines, %d rules, %d sections; want 14, 7, 8, 12",
			len(order.Tasks), len(order.DoneWhen), len(order.Rules), len(order.Sections))
	}
	if order.DoneWhen[0].Check.Kind != workorder.CheckTestsPass || order.DoneWhen[1].Check.Kind != workorder.CheckShows {
		t.Errorf("the first two done lines carry checks %+v and %+v", order.DoneWhen[0].Check, order.DoneWhen[1].Check)
	}
	if got := strings.Join(order.Tasks[5].Details, ","); got != "Dragon,Tests required" {
		t.Errorf("task six names %q, want Dragon,Tests required", got)
	}
	if !strings.HasPrefix(order.Where, "A new, empty folder") {
		t.Errorf("where reads %q", order.Where)
	}
}

func FuzzParse(f *testing.F) {
	f.Add(aSmallOrder)
	f.Add("## Goal\nx\n## Done when\n1. y [tests pass: z]\n## Tasks\n1. a (Details: b)\n")
	f.Add("no order here")
	f.Fuzz(func(t *testing.T, text string) {
		order := workorder.Parse(text)
		for _, heading := range order.Headings() {
			if !strings.Contains(text, heading) {
				t.Errorf("the heading %q is not in the text", heading)
			}
		}
		// A wrapped task line is joined with one space, so a name is checked
		// word by word rather than whole.
		for _, task := range order.Tasks {
			for _, name := range task.Details {
				for _, word := range strings.Fields(name) {
					if !strings.Contains(text, word) {
						t.Errorf("the details word %q is not in the text", word)
					}
				}
			}
		}
	})
}

func TestWhereNamesTheProjectFolder(t *testing.T) {
	order := workorder.Parse("# Tic Tac Toe\n\n## Goal\nA game.\n\n## Where\nCreate a new folder on the Desktop named exactly `Tic Tac Toe`, so the project lives at `~/Desktop/Tic Tac Toe`. Serve it on port 8096.\n\n## Done when\n1. It works.\n")
	if order.Folder != "~/Desktop/Tic Tac Toe" {
		t.Errorf("the folder reads %q, want the first backticked path with a slash in Where", order.Folder)
	}
	bare := workorder.Parse("# Notes\n\n## Goal\nNotes.\n\n## Where\nA new, empty folder.\n\n## Done when\n1. It works.\n")
	if bare.Folder != "" {
		t.Errorf("a Where that names no path gave the folder %q, want none", bare.Folder)
	}
}

func TestWhereOnTheDesktopNamedExactlyIsTheDesktopFolder(t *testing.T) {
	order := workorder.Parse("# Game\n\n## Goal\nA game.\n\n## Where\nCreate a folder on the Desktop named exactly `Solar System Viewer` and put everything inside it.\n\n## Done when\n1. It works.\n")
	if order.Folder != "~/Desktop/Solar System Viewer" {
		t.Errorf("the folder reads %q, want the Desktop folder the name makes", order.Folder)
	}
}
