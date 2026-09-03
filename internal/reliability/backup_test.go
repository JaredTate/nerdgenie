package reliability_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/reliability"
	"github.com/JaredTate/coeus/internal/testkit"
)

// aHomeWorthBackingUp fills a temporary home with a vault, a vault key, and a
// browser profile, which is everything a backup holds except the database.
func aHomeWorthBackingUp(t *testing.T) contract.Home {
	t.Helper()
	home := testkit.NewTempHome(t)
	writeVaultKey(t, home)
	writeFile(t, home.VaultFile(), "the encrypted vault", contract.SecretFileMode)
	writeFile(t, filepath.Join(home.BrowserProfile("default"), "Preferences"), "what the browser remembers", contract.DataFileMode)
	writeFile(t, filepath.Join(home.BrowserProfile("default"), "Cache", "one"), "a cached page", contract.DataFileMode)
	return home
}

// writeVaultKey puts an age private key where the vault keeps one, because the
// backup is encrypted to that key and only it can open the archive again.
func writeVaultKey(t *testing.T, home contract.Home) {
	t.Helper()
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("making the age key failed: %v", err)
	}
	writeFile(t, home.VaultKeyFile(), identity.String()+"\n", contract.SecretFileMode)
}

// writeFile writes one file and the folders above it.
func writeFile(t *testing.T, path string, written string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), contract.HomeFolderMode); err != nil {
		t.Fatalf("making the folder for %s failed: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(written), mode); err != nil {
		t.Fatalf("writing %s failed: %v", path, err)
	}
}

// readFile reads a file back, failing the test when it is not there.
func readFile(t *testing.T, path string) string {
	t.Helper()
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s back failed: %v", path, err)
	}
	return string(written)
}

// anEmptyHome is a home folder with the layout made and nothing in it, which is
// what a restore is meant to be run into.
func anEmptyHome(t *testing.T) contract.Home {
	t.Helper()
	home := contract.NewHome(filepath.Join(t.TempDir(), ".coeus"))
	for _, folder := range home.Folders() {
		if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
			t.Fatalf("making %s failed: %v", folder, err)
		}
	}
	return home
}

func TestAnArchiveIsNamedForTheDayItWasWritten(t *testing.T) {
	name := reliability.ArchiveName(startOfTime)

	if !strings.Contains(name, "2026-09-02") {
		t.Errorf("the archive is called %q, which does not say when it was written", name)
	}
	if !strings.HasSuffix(name, reliability.ArchiveSuffix) {
		t.Errorf("the archive is called %q, which does not end in %q", name, reliability.ArchiveSuffix)
	}
	if reliability.ArchiveName(startOfTime.Add(time.Hour)) == name {
		t.Errorf("two archives written an hour apart are both called %q, so one would replace the other", name)
	}
}

func TestABackupHoldsTheVaultAndTheBrowserProfileAndARestoreBringsThemBack(t *testing.T) {
	home := aHomeWorthBackingUp(t)
	clock := testkit.NewFakeClock(startOfTime)

	archive, err := reliability.Backup(context.Background(), reliability.BackupSettings{Home: home, Clock: clock})
	if err != nil {
		t.Fatalf("writing the backup failed: %v", err)
	}
	if filepath.Dir(archive) != home.BackupsFolder() {
		t.Errorf("the archive went to %s, want the backups folder %s", archive, home.BackupsFolder())
	}
	info, err := os.Stat(archive)
	if err != nil {
		t.Fatalf("the archive was not written: %v", err)
	}
	if info.Mode().Perm() != contract.SecretFileMode {
		t.Errorf("the archive has mode %#o, want %#o, because it holds everything Coeus knows", info.Mode().Perm(), contract.SecretFileMode)
	}

	restored := anEmptyHome(t)
	writeFile(t, restored.VaultKeyFile(), readFile(t, home.VaultKeyFile()), contract.SecretFileMode)

	if err := reliability.Restore(context.Background(), reliability.RestoreSettings{Home: restored, Archive: archive}); err != nil {
		t.Fatalf("putting the backup back failed: %v", err)
	}

	for original, brought := range map[string]string{
		home.VaultFile(): restored.VaultFile(),
		filepath.Join(home.BrowserProfile("default"), "Preferences"):  filepath.Join(restored.BrowserProfile("default"), "Preferences"),
		filepath.Join(home.BrowserProfile("default"), "Cache", "one"): filepath.Join(restored.BrowserProfile("default"), "Cache", "one"),
	} {
		if readFile(t, brought) != readFile(t, original) {
			t.Errorf("%s did not come back the same as %s", brought, original)
		}
	}
}

func TestOnlyTheLastSevenArchivesAreKept(t *testing.T) {
	home := aHomeWorthBackingUp(t)
	clock := testkit.NewFakeClock(startOfTime)

	written := []string{}
	for range reliability.KeptBackups + 1 {
		archive, err := reliability.Backup(context.Background(), reliability.BackupSettings{Home: home, Clock: clock})
		if err != nil {
			t.Fatalf("writing a backup failed: %v", err)
		}
		written = append(written, archive)
		clock.Advance(24 * time.Hour)
	}

	left, err := os.ReadDir(home.BackupsFolder())
	if err != nil {
		t.Fatalf("reading the backups folder failed: %v", err)
	}
	if len(left) != reliability.KeptBackups {
		t.Errorf("%d archives are in the folder, want the last %d", len(left), reliability.KeptBackups)
	}
	if _, err := os.Stat(written[0]); !os.IsNotExist(err) {
		t.Errorf("the oldest archive is still there after the eighth backup: %v", err)
	}
	if _, err := os.Stat(written[len(written)-1]); err != nil {
		t.Errorf("the newest archive was removed: %v", err)
	}
}

func TestTheNewestArchiveIsTheOneARecoveryReachesFor(t *testing.T) {
	home := aHomeWorthBackingUp(t)
	clock := testkit.NewFakeClock(startOfTime)
	if _, err := reliability.LatestArchive(home.BackupsFolder()); err == nil {
		t.Errorf("a folder with no archives in it named one anyway")
	}

	first, err := reliability.Backup(context.Background(), reliability.BackupSettings{Home: home, Clock: clock})
	if err != nil {
		t.Fatalf("writing the first backup failed: %v", err)
	}
	clock.Advance(24 * time.Hour)
	second, err := reliability.Backup(context.Background(), reliability.BackupSettings{Home: home, Clock: clock})
	if err != nil {
		t.Fatalf("writing the second backup failed: %v", err)
	}

	latest, err := reliability.LatestArchive(home.BackupsFolder())
	if err != nil {
		t.Fatalf("finding the newest archive failed: %v", err)
	}
	if latest != second || latest == first {
		t.Errorf("the newest archive is %s, want %s", latest, second)
	}
}

func TestABackupGoesWhereTheConfigurationSaysToPutIt(t *testing.T) {
	home := aHomeWorthBackingUp(t)
	folder := filepath.Join(t.TempDir(), "somewhere", "else")

	archive, err := reliability.Backup(context.Background(), reliability.BackupSettings{
		Home:   home,
		Clock:  testkit.NewFakeClock(startOfTime),
		Folder: folder,
	})
	if err != nil {
		t.Fatalf("writing the backup failed: %v", err)
	}

	if filepath.Dir(archive) != folder {
		t.Errorf("the archive went to %s, want the folder the configuration named, %s", archive, folder)
	}
}

func TestABackupWithoutAVaultKeyToLockItSaysSo(t *testing.T) {
	home := testkit.NewTempHome(t)

	_, err := reliability.Backup(context.Background(), reliability.BackupSettings{Home: home, Clock: testkit.NewFakeClock(startOfTime)})

	if err == nil {
		t.Fatalf("a backup was written with nothing to lock it")
	}
	if !strings.Contains(err.Error(), home.VaultKeyFile()) {
		t.Errorf("the failure does not name the key it needed: %v", err)
	}
}

func TestARestoreRefusesAHomeThatIsNotEmptyUnlessItIsForced(t *testing.T) {
	home := aHomeWorthBackingUp(t)
	archive, err := reliability.Backup(context.Background(), reliability.BackupSettings{Home: home, Clock: testkit.NewFakeClock(startOfTime)})
	if err != nil {
		t.Fatalf("writing the backup failed: %v", err)
	}

	err = reliability.Restore(context.Background(), reliability.RestoreSettings{Home: home, Archive: archive})

	if err == nil {
		t.Fatalf("a restore wrote over a home that already holds a vault")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("the refusal does not say how to go ahead anyway: %v", err)
	}
	if err := reliability.Restore(context.Background(), reliability.RestoreSettings{Home: home, Archive: archive, Force: true}); err != nil {
		t.Errorf("a forced restore over the same home failed: %v", err)
	}
	if readFile(t, home.VaultFile()) != "the encrypted vault" {
		t.Errorf("the forced restore did not put the vault back")
	}
}

func TestARestoreWithoutTheKeyThatLockedTheArchiveSaysSo(t *testing.T) {
	home := aHomeWorthBackingUp(t)
	archive, err := reliability.Backup(context.Background(), reliability.BackupSettings{Home: home, Clock: testkit.NewFakeClock(startOfTime)})
	if err != nil {
		t.Fatalf("writing the backup failed: %v", err)
	}
	restored := anEmptyHome(t)

	err = reliability.Restore(context.Background(), reliability.RestoreSettings{Home: restored, Archive: archive})

	if err == nil {
		t.Fatalf("an archive was opened with no key at all")
	}
	if !strings.Contains(err.Error(), restored.VaultKeyFile()) {
		t.Errorf("the failure does not name the key it looked for: %v", err)
	}
}

func TestARestoreOfAnArchiveThatIsNotThereSaysSo(t *testing.T) {
	restored := anEmptyHome(t)
	missing := filepath.Join(t.TempDir(), "nothing-like-this.tar.age")

	err := reliability.Restore(context.Background(), reliability.RestoreSettings{Home: restored, Archive: missing})

	if err == nil {
		t.Fatalf("a restore from an archive that is not there was called a success")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("the failure does not name the archive it looked for: %v", err)
	}
}

func TestARestoreOfSomethingThatIsNotAnArchiveSaysSo(t *testing.T) {
	home := aHomeWorthBackingUp(t)
	restored := anEmptyHome(t)
	writeFile(t, restored.VaultKeyFile(), readFile(t, home.VaultKeyFile()), contract.SecretFileMode)
	notAnArchive := filepath.Join(t.TempDir(), "not-an-archive.tar.age")
	writeFile(t, notAnArchive, "these are not the bytes you are looking for", contract.SecretFileMode)

	err := reliability.Restore(context.Background(), reliability.RestoreSettings{Home: restored, Archive: notAnArchive})

	if err == nil {
		t.Fatalf("bytes that are not an archive were restored")
	}
	if !strings.Contains(err.Error(), notAnArchive) {
		t.Errorf("the failure does not name the file it could not read: %v", err)
	}
}

func TestARestoreCanBeGivenTheKeyToUse(t *testing.T) {
	home := aHomeWorthBackingUp(t)
	archive, err := reliability.Backup(context.Background(), reliability.BackupSettings{Home: home, Clock: testkit.NewFakeClock(startOfTime)})
	if err != nil {
		t.Fatalf("writing the backup failed: %v", err)
	}
	restored := anEmptyHome(t)

	err = reliability.Restore(context.Background(), reliability.RestoreSettings{
		Home:    restored,
		Archive: archive,
		KeyFile: home.VaultKeyFile(),
	})

	if err != nil {
		t.Fatalf("a restore with the key given on the command line failed: %v", err)
	}
	if readFile(t, restored.VaultFile()) != "the encrypted vault" {
		t.Errorf("the vault did not come back")
	}
}
