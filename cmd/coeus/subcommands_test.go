package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/command"
	"github.com/JaredTate/coeus/internal/config"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// aTemporaryServiceMachine gives a test its own home folder, its own systemd
// folder, and a systemctl that only writes down what it was told, so that
// nothing here installs a real unit or touches the real home folder.
func aTemporaryServiceMachine(t *testing.T) (contract.Home, string) {
	t.Helper()
	home := testkit.NewTempHome(t)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), ".config"))

	folder := t.TempDir()
	written := filepath.Join(folder, "arguments.txt")
	script := "#!/bin/sh\necho \"$@\" >> " + written + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(folder, "systemctl"), []byte(script), 0o755); err != nil {
		t.Fatalf("writing the fake systemctl failed: %v", err)
	}
	t.Setenv("PATH", folder)
	return home, written
}

func TestDoctorSubcommandReportsOnTheHomeFolder(t *testing.T) {
	testkit.NewTempHome(t)
	var output, problems bytes.Buffer

	code := doctorSubcommand.run(nil, &output, &problems)

	if code != contract.ExitOK {
		t.Errorf("coeus doctor left with %d rather than %d on a home folder that is fine: %s", code, contract.ExitOK, problems.String())
	}
	if !strings.Contains(output.String(), "coeus doctor") {
		t.Errorf("coeus doctor printed no report:\n%s", output.String())
	}
}

func TestDoctorSubcommandLeavesWithAFailureWhenSomethingIsBroken(t *testing.T) {
	home := testkit.NewTempHome(t)
	if err := os.WriteFile(home.ConfigFile(), []byte("default_model = 17\n"), contract.DataFileMode); err != nil {
		t.Fatalf("writing the broken configuration failed: %v", err)
	}
	var output, problems bytes.Buffer

	if code := doctorSubcommand.run(nil, &output, &problems); code != contract.ExitFailure {
		t.Errorf("coeus doctor left with %d rather than %d on a configuration that will not load", code, contract.ExitFailure)
	}
}

func TestDoctorSubcommandSaysWhenTheHomeFolderCannotBeWorkedOut(t *testing.T) {
	t.Setenv(config.HomeVariable, "not-a-full-path")
	var output, problems bytes.Buffer

	if code := doctorSubcommand.run(nil, &output, &problems); code != contract.ExitBadConfiguration {
		t.Errorf("coeus doctor left with %d rather than %d when COEUS_HOME is not a full path", code, contract.ExitBadConfiguration)
	}
	if !strings.Contains(problems.String(), config.HomeVariable) {
		t.Errorf("coeus doctor does not name the variable that is wrong:\n%s", problems.String())
	}
}

func TestInitSubcommandRefusesAFlagItDoesNotUnderstand(t *testing.T) {
	testkit.NewTempHome(t)
	var output, problems bytes.Buffer

	if code := initSubcommand.run([]string{"--nothing-like-this"}, &output, &problems); code == contract.ExitOK {
		t.Errorf("coeus init said all was well after a flag it does not understand")
	}
}

func TestInitSubcommandLeavesAHomeThatIsAlreadySetUpAlone(t *testing.T) {
	home := testkit.NewTempHome(t)
	written := []byte("default_model = \"local\"\n")
	if err := os.WriteFile(home.ConfigFile(), written, contract.DataFileMode); err != nil {
		t.Fatalf("writing the configuration failed: %v", err)
	}
	var output, problems bytes.Buffer

	if code := initSubcommand.run(nil, &output, &problems); code != contract.ExitOK {
		t.Errorf("coeus init left with %d on a home folder that is already set up: %s", code, problems.String())
	}
	after, err := os.ReadFile(home.ConfigFile())
	if err != nil {
		t.Fatalf("reading the configuration back failed: %v", err)
	}
	if string(after) != string(written) {
		t.Errorf("coeus init rewrote a configuration that was already there:\n%s", after)
	}
}

func TestInstallSubcommandWritesTheUnitAndStartsTheService(t *testing.T) {
	home, told := aTemporaryServiceMachine(t)
	var output, problems bytes.Buffer

	if code := installSubcommand.run(nil, &output, &problems); code != contract.ExitOK {
		t.Fatalf("coeus install left with %d: %s", code, problems.String())
	}

	unitPath, err := command.UnitPath()
	if err != nil {
		t.Fatalf("working out where the unit goes failed: %v", err)
	}
	unit, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("the unit was not written: %v", err)
	}
	if string(unit) != command.UnitText(home) {
		t.Errorf("the unit that was written is not the one the package renders:\n%s", unit)
	}
	if _, err := os.ReadFile(told); err != nil {
		t.Errorf("the service manager was never told anything: %v", err)
	}
}

func TestInstallSubcommandTakesNoArguments(t *testing.T) {
	aTemporaryServiceMachine(t)
	var output, problems bytes.Buffer

	if code := installSubcommand.run([]string{"now"}, &output, &problems); code != contract.ExitUsage {
		t.Errorf("coeus install left with %d rather than %d when given a word it does not understand", code, contract.ExitUsage)
	}
}

func TestUninstallSubcommandTakesTheUnitAwayAndKeepsTheHome(t *testing.T) {
	home, _ := aTemporaryServiceMachine(t)
	var output, problems bytes.Buffer
	if code := installSubcommand.run(nil, &output, &problems); code != contract.ExitOK {
		t.Fatalf("coeus install left with %d: %s", code, problems.String())
	}

	if code := uninstallSubcommand.run(nil, &output, &problems); code != contract.ExitOK {
		t.Fatalf("coeus uninstall left with %d: %s", code, problems.String())
	}

	unitPath, err := command.UnitPath()
	if err != nil {
		t.Fatalf("working out where the unit goes failed: %v", err)
	}
	if _, err := os.Stat(unitPath); !os.IsNotExist(err) {
		t.Errorf("the unit is still there after coeus uninstall")
	}
	if _, err := os.Stat(home.Root); err != nil {
		t.Errorf("coeus uninstall took the home folder away without being asked to: %v", err)
	}
}

func TestUninstallSubcommandRefusesAFlagItDoesNotUnderstand(t *testing.T) {
	aTemporaryServiceMachine(t)
	var output, problems bytes.Buffer

	if code := uninstallSubcommand.run([]string{"--nothing-like-this"}, &output, &problems); code == contract.ExitOK {
		t.Errorf("coeus uninstall said all was well after a flag it does not understand")
	}
}
