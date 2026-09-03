package command_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/command"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// fakeSystemctl puts a program called systemctl on the PATH that writes down
// every argument it was given and then exits with the code asked for, so that a
// test can say exactly what "coeus install" told the service manager without a
// real unit ever being installed.
func fakeSystemctl(t *testing.T, exitCode int) string {
	t.Helper()
	folder := t.TempDir()
	written := filepath.Join(folder, "arguments.txt")
	script := "#!/bin/sh\n" +
		"echo \"$@\" >> " + written + "\n" +
		"if [ " + itoa(exitCode) + " -ne 0 ]; then echo 'the fake systemctl was told to fail' >&2; fi\n" +
		"exit " + itoa(exitCode) + "\n"
	if err := os.WriteFile(filepath.Join(folder, "systemctl"), []byte(script), 0o755); err != nil {
		t.Fatalf("writing the fake systemctl failed: %v", err)
	}
	t.Setenv("PATH", folder)
	return written
}

// itoa writes a small number, because the fake systemctl is built as text.
func itoa(number int) string {
	if number == 0 {
		return "0"
	}
	return "1"
}

// whatSystemctlWasTold reads back every line the fake systemctl wrote.
func whatSystemctlWasTold(t *testing.T, path string) string {
	t.Helper()
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the fake systemctl was never called: %v", err)
	}
	return string(written)
}

// aServiceIn builds the install and uninstall work against a home folder, with
// a program standing in for the coeus binary the current link points at.
func aServiceIn(t *testing.T, home contract.Home, answers string, written *strings.Builder) command.Service {
	t.Helper()
	program := filepath.Join(t.TempDir(), "coeus")
	if err := os.WriteFile(program, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("writing the stand-in binary failed: %v", err)
	}
	return command.Service{Home: home, Program: program, Input: strings.NewReader(answers), Output: written}
}

func TestTheUnitFileIsTheOneTheDesignAsksFor(t *testing.T) {
	unit := command.UnitText(contract.NewHome("/home/tester/.coeus"))

	testkit.Golden(t, "coeus.service.golden", []byte(unit))
}

func TestTheThreeUnitFilesAreTheOnesTheDesignAsksFor(t *testing.T) {
	units := command.Units(contract.NewHome("/home/tester/.coeus"))

	if len(units) != 3 {
		t.Fatalf("coeus install writes %d units, want the service and the two that run the nightly backup", len(units))
	}
	for _, unit := range units {
		testkit.Golden(t, unit.Name+".golden", []byte(unit.Text))
	}
}

func TestInstallWritesTheUnitLinksTheCurrentReleaseAndStartsTheService(t *testing.T) {
	home := testkit.NewTempHome(t)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), ".config"))
	told := fakeSystemctl(t, 0)
	written := &strings.Builder{}
	service := aServiceIn(t, home, "", written)

	if err := service.Install(context.Background()); err != nil {
		t.Fatalf("coeus install failed: %v", err)
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

	pointsAt, err := os.Readlink(home.CurrentReleaseLink())
	if err != nil {
		t.Fatalf("the current link was not made: %v", err)
	}
	if pointsAt != service.Program {
		t.Errorf("the current link points at %q rather than at the running binary", pointsAt)
	}

	said := whatSystemctlWasTold(t, told)
	for _, wanted := range []string{
		"--user daemon-reload",
		"--user enable " + command.ServiceName,
		"--user start " + command.ServiceName,
		"--user enable " + command.BackupTimerName,
		"--user start " + command.BackupTimerName,
	} {
		if !strings.Contains(said, wanted) {
			t.Errorf("coeus install never told systemctl %q, only:\n%s", wanted, said)
		}
	}
	if !strings.Contains(written.String(), unitPath) {
		t.Errorf("coeus install does not say where it wrote the unit:\n%s", written)
	}
}

func TestInstallWritesTheNightlyBackupTimerBesideTheService(t *testing.T) {
	home := testkit.NewTempHome(t)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), ".config"))
	fakeSystemctl(t, 0)
	written := &strings.Builder{}

	if err := aServiceIn(t, home, "", written).Install(context.Background()); err != nil {
		t.Fatalf("coeus install failed: %v", err)
	}

	for _, unit := range command.Units(home) {
		path, err := command.UnitPathFor(unit.Name)
		if err != nil {
			t.Fatalf("working out where %s goes failed: %v", unit.Name, err)
		}
		onDisk, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s was not written: %v", unit.Name, err)
		}
		if string(onDisk) != unit.Text {
			t.Errorf("%s on disk is not the one the package renders:\n%s", unit.Name, onDisk)
		}
	}
	if !strings.Contains(written.String(), command.BackupTimerName) {
		t.Errorf("coeus install does not say that it set up the nightly backup:\n%s", written)
	}
}

func TestInstallLeavesACurrentLinkThatIsAlreadyThereAlone(t *testing.T) {
	home := testkit.NewTempHome(t)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), ".config"))
	fakeSystemctl(t, 0)
	released := filepath.Join(home.ReleaseFolder("1.2.3"), "coeus")
	if err := os.MkdirAll(filepath.Dir(released), contract.HomeFolderMode); err != nil {
		t.Fatalf("making the release folder failed: %v", err)
	}
	if err := os.Symlink(released, home.CurrentReleaseLink()); err != nil {
		t.Fatalf("making the current link failed: %v", err)
	}

	service := aServiceIn(t, home, "", &strings.Builder{})
	if err := service.Install(context.Background()); err != nil {
		t.Fatalf("coeus install failed: %v", err)
	}

	pointsAt, err := os.Readlink(home.CurrentReleaseLink())
	if err != nil {
		t.Fatalf("reading the current link failed: %v", err)
	}
	if pointsAt != released {
		t.Errorf("coeus install moved the current link from the installed release to %q", pointsAt)
	}
}

func TestInstallSaysWhatTheServiceManagerFailedWith(t *testing.T) {
	home := testkit.NewTempHome(t)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), ".config"))
	fakeSystemctl(t, 1)

	err := aServiceIn(t, home, "", &strings.Builder{}).Install(context.Background())
	if err == nil {
		t.Fatalf("coeus install said nothing when systemctl refused")
	}
	if !strings.Contains(err.Error(), "systemctl") {
		t.Errorf("the failure does not name the program that refused: %v", err)
	}
}

func TestUninstallStopsTheServiceRemovesTheUnitAndLeavesTheHome(t *testing.T) {
	home := testkit.NewTempHome(t)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), ".config"))
	told := fakeSystemctl(t, 0)
	service := aServiceIn(t, home, "", &strings.Builder{})
	if err := service.Install(context.Background()); err != nil {
		t.Fatalf("coeus install failed: %v", err)
	}

	if err := service.Uninstall(context.Background(), nil); err != nil {
		t.Fatalf("coeus uninstall failed: %v", err)
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

	said := whatSystemctlWasTold(t, told)
	for _, wanted := range []string{
		"--user stop " + command.ServiceName,
		"--user disable " + command.ServiceName,
		"--user stop " + command.BackupTimerName,
		"--user disable " + command.BackupTimerName,
	} {
		if !strings.Contains(said, wanted) {
			t.Errorf("coeus uninstall never told systemctl %q, only:\n%s", wanted, said)
		}
	}
	for _, unit := range command.Units(home) {
		path, err := command.UnitPathFor(unit.Name)
		if err != nil {
			t.Fatalf("working out where %s goes failed: %v", unit.Name, err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s is still there after coeus uninstall", unit.Name)
		}
	}
}

func TestUninstallPurgeTakesTheHomeAwayOnlyWhenTheWordIsTypedExactly(t *testing.T) {
	for _, one := range []struct {
		typed string
		gone  bool
	}{
		{command.PurgeConfirmation + "\n", true},
		{" " + command.PurgeConfirmation + " \n", false},
		{"DELETE\n", false},
		{"\n", false},
	} {
		t.Run(strings.TrimSpace(one.typed)+"/", func(t *testing.T) {
			home := testkit.NewTempHome(t)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), ".config"))
			fakeSystemctl(t, 0)
			written := &strings.Builder{}
			service := aServiceIn(t, home, one.typed, written)

			if err := service.Uninstall(context.Background(), []string{"--purge"}); err != nil {
				t.Fatalf("coeus uninstall --purge failed: %v", err)
			}

			_, err := os.Stat(home.Root)
			if one.gone && !os.IsNotExist(err) {
				t.Errorf("the home folder is still there after the word was typed exactly")
			}
			if !one.gone && err != nil {
				t.Errorf("the home folder was taken away after %q was typed: %v", one.typed, err)
			}
			if !one.gone && !strings.Contains(written.String(), "kept") {
				t.Errorf("coeus uninstall does not say that it kept the home folder:\n%s", written)
			}
		})
	}
}

func TestUninstallRefusesAFlagItDoesNotUnderstand(t *testing.T) {
	home := testkit.NewTempHome(t)
	fakeSystemctl(t, 0)

	err := aServiceIn(t, home, "", &strings.Builder{}).Uninstall(context.Background(), []string{"--nothing-like-this"})
	if err == nil {
		t.Fatalf("coeus uninstall took a flag it does not understand")
	}
	if _, statErr := os.Stat(home.Root); statErr != nil {
		t.Errorf("coeus uninstall touched the home folder before reading its own flags: %v", statErr)
	}
}
