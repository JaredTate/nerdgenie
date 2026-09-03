package reliability_test

import (
	"archive/tar"
	"bytes"
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

func TestAKeyThatIsNotAKeyIsRefused(t *testing.T) {
	home := anEmptyHome(t)
	writeFile(t, home.VaultKeyFile(), "this is not an age key", contract.SecretFileMode)

	_, err := reliability.Backup(context.Background(), reliability.BackupSettings{Home: home, Clock: testkit.NewFakeClock(startOfTime)})

	if err == nil {
		t.Fatalf("a backup was locked with something that is not a key")
	}
	if !strings.Contains(err.Error(), "age private key") {
		t.Errorf("the failure does not say what the file should have held: %v", err)
	}
}

func TestAKeyFileTooLongToBeAKeyIsRefused(t *testing.T) {
	home := anEmptyHome(t)
	writeFile(t, home.VaultKeyFile(), strings.Repeat("x", 8192), contract.SecretFileMode)

	_, err := reliability.Backup(context.Background(), reliability.BackupSettings{Home: home, Clock: testkit.NewFakeClock(startOfTime)})

	if err == nil || !strings.Contains(err.Error(), "longer than") {
		t.Errorf("a key file of 8192 bytes came back as %v, want a refusal saying it is too long", err)
	}
}

func TestABackupNeedsAClockToNameTheArchiveAfter(t *testing.T) {
	if _, err := reliability.Backup(context.Background(), reliability.BackupSettings{Home: anEmptyHome(t)}); err == nil {
		t.Errorf("a backup was written with no clock to name it after")
	}
}

func TestABackupSaysWhenTheFolderCannotBeMade(t *testing.T) {
	home := aHomeWorthBackingUp(t)
	blocked := filepath.Join(t.TempDir(), "cannot-be-written-in")
	if err := os.Mkdir(blocked, 0o500); err != nil {
		t.Fatalf("making the read-only folder failed: %v", err)
	}

	_, err := reliability.Backup(context.Background(), reliability.BackupSettings{
		Home:   home,
		Clock:  testkit.NewFakeClock(startOfTime),
		Folder: filepath.Join(blocked, "backups"),
	})

	if err == nil {
		t.Errorf("a backup was written into a folder that could not be made")
	}
}

func TestABackupSkipsWhatIsNeitherAFileNorAFolderInTheProfile(t *testing.T) {
	home := aHomeWorthBackingUp(t)
	if err := os.Symlink(home.VaultFile(), filepath.Join(home.BrowserProfile("default"), "a-link")); err != nil {
		t.Fatalf("making the link failed: %v", err)
	}

	archive, err := reliability.Backup(context.Background(), reliability.BackupSettings{Home: home, Clock: testkit.NewFakeClock(startOfTime)})
	if err != nil {
		t.Fatalf("writing the backup failed: %v", err)
	}

	restored := anEmptyHome(t)
	writeFile(t, restored.VaultKeyFile(), readFile(t, home.VaultKeyFile()), contract.SecretFileMode)
	if err := reliability.Restore(context.Background(), reliability.RestoreSettings{Home: restored, Archive: archive}); err != nil {
		t.Fatalf("putting the backup back failed: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(restored.BrowserProfile("default"), "a-link")); !os.IsNotExist(err) {
		t.Errorf("the link was carried into the archive, and a backup holds only plain files and folders")
	}
}

func TestABackupSaysWhenTheProfileCannotBeRead(t *testing.T) {
	home := aHomeWorthBackingUp(t)
	closed := filepath.Join(home.BrowserProfile("default"), "closed")
	if err := os.Mkdir(closed, 0o000); err != nil {
		t.Fatalf("making the folder nobody can read failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(closed, contract.HomeFolderMode) })

	_, err := reliability.Backup(context.Background(), reliability.BackupSettings{Home: home, Clock: testkit.NewFakeClock(startOfTime)})

	if err == nil {
		t.Errorf("a browser folder that could not be read was backed up in silence")
	}
}

func TestARestoreRefusesAnythingInTheArchiveThatIsNotAPlainFileOrFolder(t *testing.T) {
	home := aHomeWorthBackingUp(t)
	restored := anEmptyHome(t)
	writeFile(t, restored.VaultKeyFile(), readFile(t, home.VaultKeyFile()), contract.SecretFileMode)
	identity, err := age.ParseX25519Identity(strings.TrimSpace(readFile(t, home.VaultKeyFile())))
	if err != nil {
		t.Fatalf("reading the key back failed: %v", err)
	}
	for name, header := range map[string]*tar.Header{
		"a link":                        {Typeflag: tar.TypeSymlink, Name: "vault.age", Linkname: "/etc/passwd"},
		"a file bigger than any backup": {Typeflag: tar.TypeReg, Name: "coeus.db", Size: int64(reliability.MaxArchiveBytes) + 1},
		"a name from outside the home":  {Typeflag: tar.TypeReg, Name: "../../escaped", Size: 0},
	} {
		t.Run(name, func(t *testing.T) {
			archive := filepath.Join(t.TempDir(), "archive.tar.age")
			writeArchiveHolding(t, archive, identity.Recipient(), header)

			err := reliability.Restore(context.Background(), reliability.RestoreSettings{
				Home: restored, Archive: archive, Force: true,
			})

			if err == nil {
				t.Errorf("%s was put back out of an archive", name)
			}
		})
	}
}

func TestARestoreSaysWhenTheBrowserFolderCannotBeLookedAt(t *testing.T) {
	home := aHomeWorthBackingUp(t)
	archive, err := reliability.Backup(context.Background(), reliability.BackupSettings{Home: home, Clock: testkit.NewFakeClock(startOfTime)})
	if err != nil {
		t.Fatalf("writing the backup failed: %v", err)
	}
	restored := anEmptyHome(t)
	writeFile(t, restored.VaultKeyFile(), readFile(t, home.VaultKeyFile()), contract.SecretFileMode)
	if err := os.Chmod(restored.BrowserFolder(), 0o000); err != nil {
		t.Fatalf("closing the browser folder failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(restored.BrowserFolder(), contract.HomeFolderMode) })

	err = reliability.Restore(context.Background(), reliability.RestoreSettings{Home: restored, Archive: archive})

	if err == nil {
		t.Errorf("a restore went ahead into a home it could not look at")
	}
}

// writeArchiveHolding writes a locked archive holding exactly one header, which
// is how a test builds the archives nobody should ever be sent. The archive is
// left unfinished on purpose: the header is all the reader has to refuse, and a
// header saying it is followed by two gigabytes cannot be given a body.
func writeArchiveHolding(t *testing.T, path string, recipient age.Recipient, header *tar.Header) {
	t.Helper()
	held := &bytes.Buffer{}
	archive := tar.NewWriter(held)
	if err := archive.WriteHeader(header); err != nil {
		t.Fatalf("writing the header failed: %v", err)
	}
	writeFile(t, path, string(locked(t, held.Bytes(), recipient)), contract.SecretFileMode)
}
func TestABackupKeepsAsManyArchivesAsItWasAskedTo(t *testing.T) {
	home := aHomeWorthBackingUp(t)
	clock := testkit.NewFakeClock(startOfTime)

	for range 4 {
		if _, err := reliability.Backup(context.Background(), reliability.BackupSettings{Home: home, Clock: clock, Keep: 2}); err != nil {
			t.Fatalf("writing a backup failed: %v", err)
		}
		clock.Advance(24 * time.Hour)
	}

	left, err := os.ReadDir(home.BackupsFolder())
	if err != nil {
		t.Fatalf("reading the backups folder failed: %v", err)
	}
	if len(left) != 2 {
		t.Errorf("%d archives were kept, want the 2 the caller asked for", len(left))
	}
}

func TestTheNewestArchiveInAFolderThatIsNotThereSaysSo(t *testing.T) {
	_, err := reliability.LatestArchive(filepath.Join(t.TempDir(), "nothing-like-this"))

	if err == nil {
		t.Errorf("a folder that is not there was read for archives")
	}
}
func TestABackupSaysWhenAFileInItCannotBeRead(t *testing.T) {
	home := aHomeWorthBackingUp(t)
	if err := os.Chmod(home.VaultFile(), 0o000); err != nil {
		t.Fatalf("closing the vault file failed: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(home.VaultFile(), contract.SecretFileMode) })

	_, err := reliability.Backup(context.Background(), reliability.BackupSettings{Home: home, Clock: testkit.NewFakeClock(startOfTime)})

	if err == nil {
		t.Errorf("a backup was written that quietly left the vault out")
	}
}

func TestARestoreRefusesAHomeThatAlreadyHoldsABrowserProfile(t *testing.T) {
	home := aHomeWorthBackingUp(t)
	archive, err := reliability.Backup(context.Background(), reliability.BackupSettings{Home: home, Clock: testkit.NewFakeClock(startOfTime)})
	if err != nil {
		t.Fatalf("writing the backup failed: %v", err)
	}
	restored := anEmptyHome(t)
	writeFile(t, restored.VaultKeyFile(), readFile(t, home.VaultKeyFile()), contract.SecretFileMode)
	writeFile(t, filepath.Join(restored.BrowserProfile("default"), "Preferences"), "a profile in use", contract.DataFileMode)

	err = reliability.Restore(context.Background(), reliability.RestoreSettings{Home: restored, Archive: archive})

	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Errorf("a restore over a browser profile in use came back as %v, want a refusal", err)
	}
}

func TestARestoreStopsWhenItIsToldTo(t *testing.T) {
	home := aHomeWorthBackingUp(t)
	archive, err := reliability.Backup(context.Background(), reliability.BackupSettings{Home: home, Clock: testkit.NewFakeClock(startOfTime)})
	if err != nil {
		t.Fatalf("writing the backup failed: %v", err)
	}
	restored := anEmptyHome(t)
	writeFile(t, restored.VaultKeyFile(), readFile(t, home.VaultKeyFile()), contract.SecretFileMode)
	stopped, stop := context.WithCancel(context.Background())
	stop()

	err = reliability.Restore(stopped, reliability.RestoreSettings{Home: restored, Archive: archive})

	if err == nil {
		t.Errorf("a restore that was told to stop carried on to the end")
	}
}

func TestAnArchiveCutShortSaysSo(t *testing.T) {
	home := aHomeWorthBackingUp(t)
	restored := anEmptyHome(t)
	writeFile(t, restored.VaultKeyFile(), readFile(t, home.VaultKeyFile()), contract.SecretFileMode)
	identity, err := age.ParseX25519Identity(strings.TrimSpace(readFile(t, home.VaultKeyFile())))
	if err != nil {
		t.Fatalf("reading the key back failed: %v", err)
	}
	archive := filepath.Join(t.TempDir(), "archive.tar.age")
	writeArchiveHolding(t, archive, identity.Recipient(), &tar.Header{Typeflag: tar.TypeReg, Name: "vault.age", Size: 100})

	err = reliability.Restore(context.Background(), reliability.RestoreSettings{Home: restored, Archive: archive, Force: true})

	if err == nil {
		t.Errorf("an archive that stops in the middle of a file was put back as if it were whole")
	}
}
