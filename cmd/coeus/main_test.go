package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

// theRealTable is the table the binary itself uses.
func theRealTable() []subcommand {
	return subcommands()
}

// aSubcommandReturning is a subcommand that does nothing and quits with the code
// it was built for, so that a test can prove run gives the code back unchanged.
func aSubcommandReturning(name string, code int) subcommand {
	return subcommand{
		name: name,
		help: "Quits with a fixed code.",
		run: func(_ []string, _ io.Writer, _ io.Writer) int {
			return code
		},
	}
}

func TestRunningWithNoArgumentsOpensTheTerminalScreen(t *testing.T) {
	opened := false
	table := []subcommand{
		versionSubcommand,
		{
			name: tuiName,
			help: "Opens the terminal screen.",
			run: func(_ []string, _ io.Writer, _ io.Writer) int {
				opened = true
				return contract.ExitOK
			},
		},
	}
	var output, problems bytes.Buffer

	code := run(table, nil, &output, &problems)

	if !opened {
		t.Errorf("typing coeus on its own did not open the terminal screen; it printed:\n%s", output.String())
	}
	if code != contract.ExitOK {
		t.Errorf("typing coeus on its own returned %d, want %d", code, contract.ExitOK)
	}
}

func TestTheBareCommandNamesASubcommandThatIsReallyInTheTable(t *testing.T) {
	for _, command := range theRealTable() {
		if command.name == tuiName {
			return
		}
	}
	t.Errorf("the bare coeus command runs %q, and no subcommand in the table has that name", tuiName)
}

func TestTheTableHoldsEverySubcommandThisFolderWrote(t *testing.T) {
	wanted := []string{
		"version", helpName, "init", "doctor", "serve", "run", tuiName,
		"install", "uninstall", "signal", "askpass", "sandbox-entry",
		"backup", "restore", "replay", "update",
	}
	inTheTable := map[string]bool{}
	for _, command := range theRealTable() {
		inTheTable[command.name] = true
	}

	for _, name := range wanted {
		if !inTheTable[name] {
			t.Errorf("the subcommand table has no %q, so a subcommand somebody wrote can never be typed", name)
		}
	}
}

func TestAHiddenSubcommandStillRunsButIsNotInTheListing(t *testing.T) {
	hiddenNames := []string{}
	for _, command := range theRealTable() {
		if command.hidden {
			hiddenNames = append(hiddenNames, command.name)
		}
	}
	if len(hiddenNames) == 0 {
		t.Fatal("no subcommand is hidden, and the sandbox helper is not one a person types")
	}

	var output, problems bytes.Buffer
	run(theRealTable(), []string{helpName}, &output, &problems)
	for _, name := range hiddenNames {
		if strings.Contains(output.String(), name) {
			t.Errorf("the listing offers %q, which nobody is meant to type:\n%s", name, output.String())
		}
		if _, found := lookUp(theRealTable(), name); !found {
			t.Errorf("the hidden subcommand %q cannot be run at all", name)
		}
	}
}

func TestTheHelpSubcommandPrintsEverySubcommandWithItsHelpLine(t *testing.T) {
	var output, problems bytes.Buffer

	code := run(theRealTable(), []string{"help"}, &output, &problems)

	if code != contract.ExitOK {
		t.Errorf("the help subcommand returned %d, want %d", code, contract.ExitOK)
	}
	for _, command := range theRealTable() {
		if command.hidden {
			continue
		}
		if !strings.Contains(output.String(), command.name) {
			t.Errorf("the list does not hold %q:\n%s", command.name, output.String())
		}
		if !strings.Contains(output.String(), command.help) {
			t.Errorf("the list does not hold the help line for %q:\n%s", command.name, output.String())
		}
	}
}

func TestHelpIsARowOfTheTableLikeEveryOtherSubcommand(t *testing.T) {
	found := false
	for _, command := range theRealTable() {
		if command.name == "help" {
			found = true
		}
	}

	if !found {
		t.Error("help is not in the subcommand table, so the table's own rules never apply to it")
	}
	if len(theRealTable()) < 2 {
		t.Error("the table holds fewer than two subcommands, so the check for a duplicate name can never fire")
	}
}

func TestTheShortAndLongHelpFlagsPrintTheSameList(t *testing.T) {
	for _, flag := range []string{"-h", "--help"} {
		t.Run(flag, func(t *testing.T) {
			var output, problems bytes.Buffer

			code := run(theRealTable(), []string{flag}, &output, &problems)

			if code != contract.ExitOK {
				t.Errorf("%s returned %d, want %d", flag, code, contract.ExitOK)
			}
			if !strings.Contains(output.String(), "Usage: coeus") {
				t.Errorf("%s printed no list:\n%s", flag, output.String())
			}
		})
	}
}

func TestRunGivesBackWhateverExitCodeTheSubcommandReturned(t *testing.T) {
	for _, code := range []int{
		contract.ExitOK,
		contract.ExitFailure,
		contract.ExitUsage,
		contract.ExitRestartMe,
		contract.ExitBadConfiguration,
	} {
		var output, problems bytes.Buffer
		table := []subcommand{aSubcommandReturning("quit", code)}

		if given := run(table, []string{"quit"}, &output, &problems); given != code {
			t.Errorf("the subcommand returned %d and run gave back %d", code, given)
		}
	}
}

// theChildMarker tells the test binary, when it runs itself again, that it is
// the child and should call main rather than run the test.
const theChildMarker = "COEUS_MAIN_EXIT_TEST"

func TestMainQuitsWithTheCodeTheSubcommandReturned(t *testing.T) {
	if os.Getenv(theChildMarker) == "1" {
		os.Args = []string{"coeus", "a-subcommand-nobody-wrote"}
		main()
		return
	}

	command := exec.Command(os.Args[0], "-test.run=^TestMainQuitsWithTheCodeTheSubcommandReturned$")
	command.Env = append(os.Environ(), theChildMarker+"=1")
	printed, err := command.CombinedOutput()

	if err == nil {
		t.Fatalf("the program quit cleanly on a subcommand nobody wrote:\n%s", printed)
	}
	var quit *exec.ExitError
	if !asExitError(err, &quit) {
		t.Fatalf("cannot run the program: %v\n%s", err, printed)
	}
	if quit.ExitCode() != contract.ExitUsage {
		t.Errorf("the program quit with %d, want the %d its subcommand table returned", quit.ExitCode(), contract.ExitUsage)
	}
}

// asExitError says whether the error is a program that ran and quit with a code.
func asExitError(err error, into **exec.ExitError) bool {
	quit, isExit := err.(*exec.ExitError)
	if !isExit {
		return false
	}
	*into = quit
	return true
}

func TestTheVersionSubcommandPrintsTheVersion(t *testing.T) {
	var output, problems bytes.Buffer

	code := run(theRealTable(), []string{"version"}, &output, &problems)

	if code != contract.ExitOK {
		t.Errorf("the version subcommand returned %d, want %d", code, contract.ExitOK)
	}
	if strings.TrimSpace(output.String()) != version {
		t.Errorf("the version printed as %q, want %q", strings.TrimSpace(output.String()), version)
	}
	if version != "dev" {
		t.Errorf("the version defaults to %q, want dev until a release sets it with -ldflags", version)
	}
}

func TestASubcommandNobodyWroteSaysSoAndPointsAtTheList(t *testing.T) {
	var output, problems bytes.Buffer

	code := run(theRealTable(), []string{"fly"}, &output, &problems)

	if code != contract.ExitUsage {
		t.Errorf("an unknown subcommand returned %d, want %d", code, contract.ExitUsage)
	}
	if !strings.Contains(problems.String(), "fly") {
		t.Errorf("the message %q does not name the subcommand that was asked for", problems.String())
	}
	if !strings.Contains(problems.String(), "coeus help") {
		t.Errorf("the message %q does not say how to see the list", problems.String())
	}
}

func TestEverySubcommandInTheTableHasANameAHelpLineAndSomethingToRun(t *testing.T) {
	seen := map[string]bool{}
	for _, command := range theRealTable() {
		if command.name == "" || command.help == "" || command.run == nil {
			t.Errorf("the subcommand %+v is missing its name, its help line, or its function", command)
		}
		if seen[command.name] {
			t.Errorf("the subcommand %q is in the table twice", command.name)
		}
		seen[command.name] = true
	}
}

func TestTheHelpListingGivesEachSubcommandOneRowAndNoMore(t *testing.T) {
	var output, problems bytes.Buffer

	run(theRealTable(), []string{"help"}, &output, &problems)

	for _, command := range theRealTable() {
		if command.hidden {
			continue
		}
		if rows := rowsNaming(output.String(), command.name); rows != 1 {
			t.Errorf("the listing gives %q %d rows, want one:\n%s", command.name, rows, output.String())
		}
	}
	if rows := rowsNaming(output.String(), "help"); rows != 1 {
		t.Errorf("the listing gives help %d rows, want one:\n%s", rows, output.String())
	}
}

// rowsNaming counts the rows of the listing whose first word is the name, so
// that a name appearing inside another row's help line is not counted twice.
func rowsNaming(listing string, name string) int {
	rows := 0
	for _, line := range strings.Split(listing, "\n") {
		first, _, _ := strings.Cut(strings.TrimSpace(line), " ")
		if first == name {
			rows++
		}
	}
	return rows
}
