package sandbox

import (
	"bytes"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestTheHelperReadsTheFoldersItMayReachAndTheCommandToRun(t *testing.T) {
	request, err := parseEntryArguments([]string{
		"--read", "/usr", "--read", "/etc",
		"--write", "/home/example/work", "--write", "/tmp",
		"--", "/bin/sh", "-c", "echo hello",
	})
	if err != nil {
		t.Fatalf("the helper refused arguments the fence would really give it: %v", err)
	}

	if !slices.Equal(request.readable, []string{"/usr", "/etc"}) {
		t.Errorf("the readable folders are %v, want /usr and /etc", request.readable)
	}
	if !slices.Equal(request.writable, []string{"/home/example/work", "/tmp"}) {
		t.Errorf("the writable folders are %v, want the work folder and /tmp", request.writable)
	}
	if request.program != "/bin/sh" {
		t.Errorf("the program is %q, want /bin/sh", request.program)
	}
	if !slices.Equal(request.arguments, []string{"-c", "echo hello"}) {
		t.Errorf("the arguments are %v, want the two after the program", request.arguments)
	}
}

func TestTheHelperAcceptsACommandWithNoArgumentsOfItsOwn(t *testing.T) {
	request, err := parseEntryArguments([]string{"--write", "/tmp", "--", "/bin/true"})
	if err != nil {
		t.Fatalf("a command with no arguments was refused: %v", err)
	}
	if len(request.arguments) != 0 {
		t.Errorf("the arguments are %v, want none", request.arguments)
	}
}

func TestTheHelperRefusesArgumentsThatNameNoCommand(t *testing.T) {
	cases := map[string][]string{
		"nothing at all":            {},
		"no double dash":            {"--read", "/usr"},
		"nothing after the dashes":  {"--read", "/usr", "--"},
		"an option with no folder":  {"--read"},
		"an option this helper has": {"--write"},
	}

	for what, arguments := range cases {
		if _, err := parseEntryArguments(arguments); err == nil {
			t.Errorf("the helper accepted %s: %v", what, arguments)
		}
	}
}

func TestTheHelperRefusesAFolderThatIsNotAFullPath(t *testing.T) {
	if _, err := parseEntryArguments([]string{"--read", "usr", "--", "/bin/true"}); err == nil {
		t.Fatal("a folder named without a full path was accepted, and the helper has no working directory to read it against")
	}
}

func TestTheHelperRefusesAnOptionItDoesNotKnow(t *testing.T) {
	_, err := parseEntryArguments([]string{"--allow-everything", "/", "--", "/bin/true"})
	if err == nil {
		t.Fatal("an option the helper does not know was accepted")
	}
	if !strings.Contains(err.Error(), "--allow-everything") {
		t.Errorf("the refusal says %q, and it must name the option it did not know", err)
	}
}

func TestTheHelperRefusesBeingGivenNoFoldersAtAll(t *testing.T) {
	if _, err := parseEntryArguments([]string{"--", "/bin/true"}); err == nil {
		t.Fatal("the helper accepted a ruleset that allows nothing, which would deny the command its own program")
	}
}

func TestTheHelperRefusesMoreFoldersThanTheCap(t *testing.T) {
	arguments := []string{}
	for index := 0; index <= MaxHelperFolders; index++ {
		arguments = append(arguments, "--read", "/usr")
	}
	arguments = append(arguments, "--", "/bin/true")

	if _, err := parseEntryArguments(arguments); err == nil {
		t.Fatalf("more than %d folders were accepted", MaxHelperFolders)
	}
}

func TestTheHelperRefusesToRunOutsideTheFence(t *testing.T) {
	t.Setenv(FenceMarkerVariable, "")
	progress := &bytes.Buffer{}

	err := Entry([]string{"--write", "/tmp", "--", "/bin/true"}, progress)
	if err == nil {
		t.Fatal("the helper ran outside the fence, and only the sandbox may start it")
	}
	if !strings.Contains(err.Error(), EntrySubcommandName) {
		t.Errorf("the refusal says %q, and it must name the subcommand a person should not be typing", err)
	}
	if progress.Len() != 0 {
		t.Errorf("the helper wrote %q before refusing, and it must say nothing until it is inside the fence", progress)
	}
}

func TestTheHelperRefusesBadArgumentsBeforeItLooksAtAnythingElse(t *testing.T) {
	t.Setenv(FenceMarkerVariable, "1")

	if err := Entry([]string{"--read"}, &bytes.Buffer{}); err == nil {
		t.Fatal("the helper accepted an option with no folder after it")
	}
}

func TestTheEnvironmentPassedOnHoldsEverythingButTheFenceMarker(t *testing.T) {
	t.Setenv(FenceMarkerVariable, "1")
	t.Setenv("A_NAME_THE_COMMAND_KEEPS", "kept")

	passedOn := environmentWithoutMarker()

	for _, entry := range passedOn {
		if strings.HasPrefix(entry, FenceMarkerVariable+"=") {
			t.Errorf("the command would see %q, and the fence marker must not reach it", entry)
		}
	}
	if !slices.Contains(passedOn, "A_NAME_THE_COMMAND_KEEPS=kept") {
		t.Error("the environment passed on lost a name the fence set for the command")
	}
	if len(passedOn) != len(os.Environ())-1 {
		t.Errorf("the environment passed on has %d entries and this program has %d, want one fewer", len(passedOn), len(os.Environ()))
	}
}

// FuzzTheHelpersArgumentsAreNeverAPanic throws any command line at the helper's
// argument reader and asserts it either returns a request whose parts are all
// full paths, or an error, and never panics.
func FuzzTheHelpersArgumentsAreNeverAPanic(f *testing.F) {
	f.Add("--read /usr --write /tmp -- /bin/sh -c echo")
	f.Add("--")
	f.Add("")
	f.Add("--read --write -- --")
	f.Add("--read /usr --read /usr --read /usr -- /bin/true")

	f.Fuzz(func(t *testing.T, line string) {
		request, err := parseEntryArguments(strings.Fields(line))
		if err != nil {
			return
		}
		if request.program == "" {
			t.Errorf("the helper accepted %q and came back with no program to run", line)
		}
		for _, folder := range append(append([]string{}, request.readable...), request.writable...) {
			if !strings.HasPrefix(folder, "/") {
				t.Errorf("the helper accepted %q, which names the folder %q without a full path", line, folder)
			}
		}
	})
}
