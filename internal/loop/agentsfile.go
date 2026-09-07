package loop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/workorder"
)

// MaxRulesInAStandingOrder is how many of a job's rules the standing order
// lists, so that the file stays under sixty lines.
const MaxRulesInAStandingOrder = 20

// theMapFile is the generated map a standing order points at when the folder
// has one.
const theMapFile = "REPO_MAP.md"

// writeTheStandingOrder writes the work folder's AGENTS.md from a job that
// finished done, once: a folder that already has one, whoever wrote it, is
// left alone. The next job on the folder then starts with the rules and the
// commands in front of every task. Nothing here fails the job.
func (theLoop *Loop) writeTheStandingOrder(held contract.Record) {
	folder := theLoop.options.WorkingDirectory
	if folder == "" {
		return
	}
	path := filepath.Join(folder, StandingOrderFile)
	if _, err := os.Stat(path); err == nil {
		return
	}
	_ = os.WriteFile(path, []byte(theStandingOrderOf(held, folder)), 0o644)
}

// theStandingOrderOf writes the file's text: the name, the why, how to run
// and test it from the checked done lines, the rules, and where the other
// documents are.
func theStandingOrderOf(held contract.Record, folder string) string {
	var out strings.Builder
	name := strings.TrimSpace(held.Goal.Name)
	if name == "" {
		name = "This project"
	}
	fmt.Fprintf(&out, "# %s\n\n", name)
	if why := strings.TrimSpace(held.Goal.Why); why != "" {
		out.WriteString(why + "\n\n")
	}
	run, test := theRunAndTestLinesOf(held.Goal.DoneWhen)
	out.WriteString("## Run\n\n" + run + "\n\n## Test\n\n" + test + "\n\n")
	if len(held.Rules.Corrections) > 0 {
		out.WriteString("## Rules\n\n")
		for at, rule := range held.Rules.Corrections {
			if at == MaxRulesInAStandingOrder {
				break
			}
			out.WriteString("- " + strings.TrimSpace(rule.Text) + "\n")
		}
		out.WriteString("\n")
	}
	out.WriteString("## Where things are\n\n" + theDocumentPointersOf(folder) + "\n")
	return out.String()
}

// theRunAndTestLinesOf reads the commands off the done lines' checks: the
// first tests-pass or exit-0 check is how the project is tested, and a shows
// check names the page to open.
func theRunAndTestLinesOf(lines []contract.DoneLine) (string, string) {
	run := "See the project's own files for how to run it."
	test := "See the tests folder."
	testFound := false
	for _, line := range lines {
		check, found := workorder.ReadCheck(line.Text)
		if !found {
			continue
		}
		switch check.Kind {
		case "tests pass", "exit 0":
			if !testFound {
				test = "`" + check.Argument + "`"
				testFound = true
			}
		case "shows":
			run = "Serve the project and open " + check.URL + "."
		}
	}
	return run, test
}

// theDocumentPointersOf names the other two documents the folder holds.
func theDocumentPointersOf(folder string) string {
	var pointers []string
	if _, err := os.Stat(filepath.Join(folder, ArchitectureFile)); err == nil {
		pointers = append(pointers, "`"+ArchitectureFile+"` says how the parts fit; read one section by its heading.")
	}
	if _, err := os.Stat(filepath.Join(folder, theMapFile)); err == nil {
		pointers = append(pointers, "`"+theMapFile+"` says where everything is.")
	}
	if len(pointers) == 0 {
		return "The source, the tests and the assets are in this folder."
	}
	return strings.Join(pointers, "\n")
}
