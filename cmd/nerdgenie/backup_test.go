package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
	"github.com/JaredTate/nerdgenie/internal/config"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/reliability"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// aHomeWithSomethingToBackUp gives the test its own home folder with a vault
// key, a vault, and a browser profile in it, which is what an archive holds.
func aHomeWithSomethingToBackUp(t *testing.T) contract.Home {
	t.Helper()
	home := testkit.NewTempHome(t)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("making the age key failed: %v", err)
	}
	write(t, home.VaultKeyFile(), identity.String()+"\n")
	write(t, home.VaultFile(), "the encrypted vault")
	write(t, filepath.Join(home.BrowserProfile("default"), "Preferences"), "what the browser remembers")
	return home
}

// write writes one file and the folders above it.
func write(t *testing.T, path string, written string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), contract.HomeFolderMode); err != nil {
		t.Fatalf("making the folder for %s failed: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(written), contract.SecretFileMode); err != nil {
		t.Fatalf("writing %s failed: %v", path, err)
	}
}

// theOneArchiveIn returns the single archive in a folder, failing the test when
// there is not exactly one.
func theOneArchiveIn(t *testing.T, folder string) string {
	t.Helper()
	entries, err := os.ReadDir(folder)
	if err != nil {
		t.Fatalf("reading %s failed: %v", folder, err)
	}
	if len(entries) != 1 {
		t.Fatalf("%s holds %d files, want the one archive", folder, len(entries))
	}
	return filepath.Join(folder, entries[0].Name())
}

func TestTheBackupSubcommandWritesAnArchiveAndSaysWhereItIs(t *testing.T) {
	home := aHomeWithSomethingToBackUp(t)
	var output, problems bytes.Buffer

	if code := backupSubcommand.run(nil, &output, &problems); code != contract.ExitOK {
		t.Fatalf("coeus backup left with %d: %s", code, problems.String())
	}

	archive := theOneArchiveIn(t, home.BackupsFolder())
	if !strings.Contains(output.String(), archive) {
		t.Errorf("coeus backup does not say where it put the archive:\n%s", output.String())
	}
	if !strings.Contains(output.String(), home.VaultKeyFile()) {
		t.Errorf("coeus backup does not say which key is needed to open the archive again:\n%s", output.String())
	}
}

func TestTheBackupSubcommandTakesNoArguments(t *testing.T) {
	aHomeWithSomethingToBackUp(t)
	var output, problems bytes.Buffer

	if code := backupSubcommand.run([]string{"now"}, &output, &problems); code != contract.ExitUsage {
		t.Errorf("coeus backup left with %d rather than %d when given a word it does not understand", code, contract.ExitUsage)
	}
}

func TestTheBackupSubcommandSaysWhenThereIsNoKeyToLockTheArchiveWith(t *testing.T) {
	testkit.NewTempHome(t)
	var output, problems bytes.Buffer

	if code := backupSubcommand.run(nil, &output, &problems); code != contract.ExitFailure {
		t.Errorf("coeus backup left with %d on a home with no vault key", code)
	}
	if !strings.Contains(problems.String(), "coeus backup") {
		t.Errorf("the failure does not say which command failed:\n%s", problems.String())
	}
}

func TestTheRestoreSubcommandPutsTheArchiveBackIntoAnEmptyHome(t *testing.T) {
	home := aHomeWithSomethingToBackUp(t)
	var output, problems bytes.Buffer
	if code := backupSubcommand.run(nil, &output, &problems); code != contract.ExitOK {
		t.Fatalf("coeus backup left with %d: %s", code, problems.String())
	}
	archive := theOneArchiveIn(t, home.BackupsFolder())

	// The home is emptied the way a person would before putting a backup back,
	// and the key is kept, because the key is what opens the archive.
	if err := os.Remove(home.VaultFile()); err != nil {
		t.Fatalf("emptying the home failed: %v", err)
	}
	if err := os.RemoveAll(home.BrowserFolder()); err != nil {
		t.Fatalf("emptying the browser folder failed: %v", err)
	}

	output.Reset()
	if code := restoreSubcommand.run([]string{archive}, &output, &problems); code != contract.ExitOK {
		t.Fatalf("coeus restore left with %d: %s", code, problems.String())
	}

	written, err := os.ReadFile(home.VaultFile())
	if err != nil {
		t.Fatalf("the vault did not come back: %v", err)
	}
	if string(written) != "the encrypted vault" {
		t.Errorf("the vault came back as %q", written)
	}
	if !strings.Contains(output.String(), archive) {
		t.Errorf("coeus restore does not say what it put back:\n%s", output.String())
	}
}

func TestTheRestoreSubcommandRefusesAHomeInUseUntilItIsForced(t *testing.T) {
	home := aHomeWithSomethingToBackUp(t)
	var output, problems bytes.Buffer
	if code := backupSubcommand.run(nil, &output, &problems); code != contract.ExitOK {
		t.Fatalf("coeus backup left with %d: %s", code, problems.String())
	}
	archive := theOneArchiveIn(t, home.BackupsFolder())

	if code := restoreSubcommand.run([]string{archive}, &output, &problems); code != contract.ExitFailure {
		t.Errorf("coeus restore left with %d over a home that is in use", code)
	}
	if !strings.Contains(problems.String(), "--force") {
		t.Errorf("the refusal does not say how to go ahead anyway:\n%s", problems.String())
	}

	if code := restoreSubcommand.run([]string{"--force", archive}, &output, &problems); code != contract.ExitOK {
		t.Errorf("coeus restore --force left with %d: %s", code, problems.String())
	}
}

func TestTheRestoreSubcommandNeedsExactlyOneArchive(t *testing.T) {
	aHomeWithSomethingToBackUp(t)
	for _, arguments := range [][]string{nil, {"one.tar.age", "two.tar.age"}} {
		var output, problems bytes.Buffer

		if code := restoreSubcommand.run(arguments, &output, &problems); code != contract.ExitUsage {
			t.Errorf("coeus restore %v left with %d rather than %d", arguments, code, contract.ExitUsage)
		}
	}
}

func TestTheRestoreSubcommandCanBeGivenTheKeyToOpenTheArchiveWith(t *testing.T) {
	home := aHomeWithSomethingToBackUp(t)
	var output, problems bytes.Buffer
	if code := backupSubcommand.run(nil, &output, &problems); code != contract.ExitOK {
		t.Fatalf("coeus backup left with %d: %s", code, problems.String())
	}
	archive := theOneArchiveIn(t, home.BackupsFolder())
	elsewhere := filepath.Join(t.TempDir(), "vault.key")
	written, err := os.ReadFile(home.VaultKeyFile())
	if err != nil {
		t.Fatalf("reading the key failed: %v", err)
	}
	write(t, elsewhere, string(written))

	code := restoreSubcommand.run([]string{"--key", elsewhere, "--force", archive}, &output, &problems)

	if code != contract.ExitOK {
		t.Errorf("coeus restore --key left with %d: %s", code, problems.String())
	}
}

func TestTheTwoSubcommandsSayWhenTheHomeFolderCannotBeWorkedOut(t *testing.T) {
	for _, one := range []struct {
		name       string
		subcommand subcommand
		arguments  []string
	}{
		{"backup", backupSubcommand, nil},
		{"restore", restoreSubcommand, []string{"an-archive.tar.age"}},
	} {
		t.Run(one.name, func(t *testing.T) {
			t.Setenv(config.HomeVariable, "not-a-full-path")
			var output, problems bytes.Buffer

			if code := one.subcommand.run(one.arguments, &output, &problems); code != contract.ExitBadConfiguration {
				t.Errorf("coeus %s left with %d rather than %d when COEUS_HOME is not a full path", one.name, code, contract.ExitBadConfiguration)
			}
		})
	}
}

func TestTheBackupSubcommandKeepsOnlyTheLastSevenArchives(t *testing.T) {
	home := aHomeWithSomethingToBackUp(t)

	for range reliability.KeptBackups + 2 {
		var output, problems bytes.Buffer
		if code := backupSubcommand.run(nil, &output, &problems); code != contract.ExitOK {
			t.Fatalf("coeus backup left with %d: %s", code, problems.String())
		}
	}

	left, err := os.ReadDir(home.BackupsFolder())
	if err != nil {
		t.Fatalf("reading the backups folder failed: %v", err)
	}
	if len(left) > reliability.KeptBackups {
		t.Errorf("%d archives are kept, want at most %d", len(left), reliability.KeptBackups)
	}
}
