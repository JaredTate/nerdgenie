package tool_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// describingTools puts count programs in the tools folder, each of which
// describes itself under its own name and writes one line into a tally file
// every time it is run, so that a test can count how many were asked at all.
func describingTools(t *testing.T, home contract.Home, count int, describe bool) string {
	t.Helper()
	tally := filepath.Join(t.TempDir(), "asked.txt")
	for at := 0; at < count; at++ {
		name := fmt.Sprintf("tool-%03d", at)
		answer := `echo 'this is not a tool description at all'`
		if describe {
			answer = fmt.Sprintf(`echo '{"name":"%s","description":"Answers with the name of this program and nothing else."}'`, name)
		}
		writeUserTool(t, home, name, fmt.Sprintf("echo %s >> %s\n%s\n", name, tally, answer))
	}
	return tally
}

// timesAsked is how many of the programs in the tools folder were run.
func timesAsked(t *testing.T, tally string) int {
	t.Helper()
	written, err := os.ReadFile(tally)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatalf("cannot read the tally of the programs that were asked: %v", err)
	}
	return len(strings.Fields(string(written)))
}

func TestTheToolsFolderIsReadNoFurtherThanTheSixtyFourTheRegistryHolds(t *testing.T) {
	home := testkit.NewTempHome(t)
	tally := describingTools(t, home, 84, true)
	lines := []string{}
	registry := registryOver(t, home, &lines)

	if err := registry.AddUserTools(context.Background()); err != nil {
		t.Fatalf("loading the user's own tools failed: %v", err)
	}
	if asked := timesAsked(t, tally); asked != 64 {
		t.Errorf("%d of the 84 programs in the tools folder were run, want the 64 the registry can hold", asked)
	}
	if held := len(registry.Specs()); held != 64 {
		t.Errorf("the registry holds %d tools, want the 64 it caps at", held)
	}
}

func TestAFolderOfProgramsThatDescribeNothingIsStillOnlyReadSixtyFourDeep(t *testing.T) {
	home := testkit.NewTempHome(t)
	tally := describingTools(t, home, 84, false)
	lines := []string{}
	registry := registryOver(t, home, &lines)

	if err := registry.AddUserTools(context.Background()); err != nil {
		t.Fatalf("loading the user's own tools failed: %v", err)
	}
	if asked := timesAsked(t, tally); asked != 64 {
		t.Errorf("%d of the 84 programs in the tools folder were run, want the 64 the cap allows", asked)
	}
	if len(registry.Specs()) != 0 {
		t.Errorf("the registry took a program that described nothing")
	}
	if !strings.Contains(strings.Join(lines, "\n"), "84") {
		t.Errorf("nothing logged says how many programs the folder holds: %v", lines)
	}
}
