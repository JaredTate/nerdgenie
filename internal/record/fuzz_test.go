package record

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// FuzzParse holds the parser to two promises, whatever it is handed. Any text at
// all reads either as a record or as an error, and never as a panic. And any text
// that does read as a record prints back and reads again as the same record, so
// that a checkpoint can be written and read as many times as a task is put down
// and picked up.
func FuzzParse(f *testing.F) {
	for _, name := range []string{"task.txt", "job.txt"} {
		content, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			f.Fatalf("cannot read the golden record %s to seed the fuzzing: %v", name, err)
		}
		f.Add(string(content))
	}
	for _, seed := range fuzzSeeds() {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, text string) {
		parsed, err := Parse([]byte(text))
		if err != nil {
			return
		}
		printed := Print(parsed)
		again, err := Parse(printed)
		if err != nil {
			t.Fatalf("a record that read once does not read after printing: %v\n--- printed ---\n%s", err, printed)
		}
		if !reflect.DeepEqual(parsed, again) {
			t.Fatalf("a record changed on its way through the text form.\nwant %+v\ngot  %+v\n--- printed ---\n%s",
				parsed, again, printed)
		}
	})
}

// fuzzSeeds are the small awkward records the fuzzer starts from, so that it
// spends its time on the shapes that are easy to get wrong rather than on
// finding the header.
func fuzzSeeds() []string {
	bare := "# task 1   running   budget left: 0 rounds, 0 minutes\n" +
		"this turn: 0.0k tokens in, 0.0k of them cached, 0.0k out\n\n" +
		"## Goal\nAsk: \"do the thing\"\n\n## Rules\n\n## Work\n\n## Lessons\n"
	return []string{
		"",
		"\n",
		"# job 1   done   0 of 0 tasks done\n\n## Goal\nAsk: \"run it\"\n\n## Rules\n\n## Work\n\n## Lessons\n",
		bare,
		insertIntoGoal(bare, "Done when:\n- [ ] a line with an arrow in it -> and no result\n"),
		insertIntoGoal(bare, "Done when:\n- [ ] a line ->\n- [x] a proven line -> r1\n"),
		insertIntoGoal(bare, "Done when:\n- [ ] a -> ->\n"),
		insertIntoGoal(bare, "Ask: \"a \\n b \\\\ c \\q d\"\n"),
		insertIntoGoal(bare, "Ask: \"\"\"\n"),
	}
}

// insertIntoGoal puts some lines under the goal heading of a bare record.
func insertIntoGoal(record string, lines string) string {
	return strings.Replace(record, headingGoal+"\n", headingGoal+"\n"+lines, 1)
}
