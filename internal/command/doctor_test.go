package command_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/command"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/sandbox"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

func TestDoctorPrintsTheReportAndSaysNothingIsBroken(t *testing.T) {
	home := testkit.NewTempHome(t)
	written := &strings.Builder{}

	if !command.Doctor(context.Background(), home, written) {
		t.Errorf("the doctor called a fresh temporary home broken:\n%s", written)
	}
	for _, wanted := range []string{"nerdgenie doctor", home.Root, "config.toml"} {
		if !strings.Contains(written.String(), wanted) {
			t.Errorf("the doctor's report leaves out %q:\n%s", wanted, written)
		}
	}
}

func TestDoctorSaysWhenTheConfigurationWillNotLoad(t *testing.T) {
	home := testkit.NewTempHome(t)
	if err := os.WriteFile(home.ConfigFile(), []byte("default_model = 17\n"), contract.DataFileMode); err != nil {
		t.Fatalf("writing the broken configuration failed: %v", err)
	}
	written := &strings.Builder{}

	if command.Doctor(context.Background(), home, written) {
		t.Errorf("the doctor called a configuration that will not load fine:\n%s", written)
	}
	if !strings.Contains(written.String(), "default_model") {
		t.Errorf("the doctor's report does not name the key that is wrong:\n%s", written)
	}
}

func TestDoctorSaysWhenTheHomeFolderIsNotThere(t *testing.T) {
	home := emptyHome(t)
	written := &strings.Builder{}

	if command.Doctor(context.Background(), home, written) {
		t.Errorf("the doctor called a home folder that is not there fine:\n%s", written)
	}
	if !strings.Contains(written.String(), "nerdgenie init") {
		t.Errorf("the doctor's report does not say to run nerdgenie init:\n%s", written)
	}
}

// sandboxFindingName is what the doctor's report calls the line the sandbox's
// user-namespace probe fills in.
const sandboxFindingName = "the sandbox fence"

// TestDoctorSaysWhichWayTheSandboxSettingIsTurned holds what a person runs
// "nerdgenie doctor" for after changing the setting: the printed report says whether
// commands are running on this machine as them or inside the fence.
func TestDoctorSaysWhichWayTheSandboxSettingIsTurned(t *testing.T) {
	for _, written := range []struct {
		document string
		wanted   string
	}{
		{"", "straight on this machine as you"},
		{"sandbox = \"fence\"\n", "only the sandbox roots"},
	} {
		home := testkit.NewTempHome(t)
		if err := os.WriteFile(home.ConfigFile(), []byte(written.document), contract.DataFileMode); err != nil {
			t.Fatalf("writing the configuration failed: %v", err)
		}
		printed := &strings.Builder{}

		command.Doctor(context.Background(), home, printed)

		if !strings.Contains(printed.String(), "the sandbox setting") {
			t.Fatalf("the doctor's report has no line about the sandbox setting:\n%s", printed)
		}
		if !strings.Contains(printed.String(), written.wanted) {
			t.Errorf("the configuration %q is reported without %q:\n%s", written.document, written.wanted, printed)
		}
	}
}

// aFolderEveryMachineHas is the folder this test builds its own throwaway fence
// over, the same one the doctor uses. The probe makes an empty fence and quits
// without ever looking at the folders, so which folder it is does not matter as
// long as it is there and the sandbox rules allow it.
const aFolderEveryMachineHas = "/usr"

func TestDoctorSaysWhatIsMissingWhenTheSandboxCannotFence(t *testing.T) {
	home := testkit.NewTempHome(t)
	noProgramsOnThePath(t)
	written := &strings.Builder{}

	if !command.Doctor(context.Background(), home, written) {
		t.Errorf("the doctor called a machine without bubblewrap broken, and Coeus runs with the shell tool off:\n%s", written)
	}
	for _, wanted := range []string{sandboxFindingName, "bubblewrap"} {
		if !strings.Contains(written.String(), wanted) {
			t.Errorf("the doctor's report leaves out %q:\n%s", wanted, written)
		}
	}
}

func TestDoctorPassesOnWhatTheSandboxSaidAboutMakingAUserNamespace(t *testing.T) {
	home := testkit.NewTempHome(t)
	written := &strings.Builder{}

	command.Doctor(context.Background(), home, written)

	if !strings.Contains(written.String(), sandboxFindingName) {
		t.Fatalf("the doctor's report has no line about the sandbox fence:\n%s", written)
	}
	// The sandbox's own words are what name the AppArmor fix on a machine that
	// refuses bwrap a user namespace, so the doctor has to pass them on rather
	// than sum them up. Whichever way this machine answers, the doctor's report
	// has to carry that answer.
	said := whatTheSandboxSays(t, home)
	if said != nil && !strings.Contains(written.String(), said.Error()) {
		t.Errorf("the doctor does not pass on what the sandbox said:\nsandbox: %v\nreport:\n%s", said, written)
	}
	if said == nil && !strings.Contains(written.String(), "user namespace") {
		t.Errorf("the doctor does not say that the sandbox can make a user namespace here:\n%s", written)
	}
}

// whatTheSandboxSays asks the sandbox package itself whether this machine can
// build a fence, so that a test can hold the doctor's report against the answer
// the doctor is meant to be passing on.
func whatTheSandboxSays(t *testing.T, home contract.Home) error {
	t.Helper()
	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("reading the home directory failed: %v", err)
	}
	fence, err := sandbox.New(sandbox.Settings{
		Roots:     []string{aFolderEveryMachineHas},
		UserHome:  userHome,
		AgentHome: home.Root,
	})
	if err != nil {
		t.Fatalf("building the fence the doctor builds failed: %v", err)
	}
	return fence.Available()
}

// TestDoctorDoesNotSayCommandsRunInsideTheFenceWhenTheSandboxIsOff holds the
// fence line honest on a fresh install, which runs with the sandbox off: on a
// machine that can build a fence, the line says the fence is ready for the
// setting that turns it on, rather than that commands are running inside it.
func TestDoctorDoesNotSayCommandsRunInsideTheFenceWhenTheSandboxIsOff(t *testing.T) {
	home := testkit.NewTempHome(t)
	written := &strings.Builder{}

	command.Doctor(context.Background(), home, written)

	if strings.Contains(written.String(), "shell commands run inside the fence") {
		t.Errorf("the sandbox is off, and the report says commands run inside the fence:\n%s", written)
	}
	if whatTheSandboxSays(t, home) == nil && !strings.Contains(written.String(), `"fence"`) {
		t.Errorf("the fence can be built here, and the report does not name the setting that turns it on:\n%s", written)
	}
}
