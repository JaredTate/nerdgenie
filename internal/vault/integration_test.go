//go:build integration

package vault_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/vault"
)

// openIn opens a vault in a home built on the real filesystem, with the modes
// the layout calls for.
func openIn(t *testing.T, home contract.Home) (*vault.Vault, error) {
	t.Helper()
	return vault.Open(home, testkit.NewFakeClock(time.Unix(0, 0).UTC()))
}

// makeFolderReadOnly takes the write bit off a folder and puts it back when the
// test is over, so that the test framework can still throw the folder away.
func makeFolderReadOnly(t *testing.T, folder string) {
	t.Helper()
	t.Cleanup(func() { _ = os.Chmod(folder, contract.HomeFolderMode) })
	if err := os.Chmod(folder, 0o500); err != nil {
		t.Fatalf("taking the write bit off %s failed: %v", folder, err)
	}
}

func TestAWholeVaultLivesOnTheRealFilesystemWithTheRightModes(t *testing.T) {
	home := testkit.NewTempHome(t)
	opened, err := openIn(t, home)
	if err != nil {
		t.Fatalf("opening the vault failed: %v", err)
	}
	entry := vault.Entry{
		Name:       "x-account",
		Site:       "X",
		Domains:    []string{"x.com"},
		Username:   "jared",
		Password:   "correct-horse-battery-staple",
		TOTPSecret: rfc6238Secret,
	}
	if err := opened.Add(entry); err != nil {
		t.Fatalf("adding the entry failed: %v", err)
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("closing the vault failed: %v", err)
	}

	for _, path := range []string{home.VaultFile(), home.VaultKeyFile()} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("the file %s is not there: %v", path, err)
		}
		if info.Mode().Perm() != contract.SecretFileMode {
			t.Errorf("the file %s has mode %v, want %v", path, info.Mode().Perm(), contract.SecretFileMode)
		}
	}

	reopened, err := openIn(t, home)
	if err != nil {
		t.Fatalf("reopening the vault failed: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	credential, err := reopened.Resolve(context.Background(), contract.SecretReferencePrefix+"x-account")
	if err != nil {
		t.Fatalf("resolving the entry after a real round trip failed: %v", err)
	}
	if credential.Password != entry.Password {
		t.Errorf("the password did not survive the round trip through the real filesystem")
	}
	if left := leftoverTemporaryFiles(t, home.Root); len(left) != 0 {
		t.Errorf("the vault left the half-written files %v behind", left)
	}
}

// leftoverTemporaryFiles lists the files a half-finished write would have left.
func leftoverTemporaryFiles(t *testing.T, folder string) []string {
	t.Helper()
	entries, err := os.ReadDir(folder)
	if err != nil {
		t.Fatalf("reading the home folder failed: %v", err)
	}
	left := []string{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "vault-") {
			left = append(left, entry.Name())
		}
	}
	return left
}

func TestAKeyFileThatIsNotAPlainFileIsRefused(t *testing.T) {
	home := testkit.NewTempHome(t)
	if err := os.Mkdir(home.VaultKeyFile(), contract.HomeFolderMode); err != nil {
		t.Fatalf("putting a folder where the key file belongs failed: %v", err)
	}

	_, err := openIn(t, home)
	if err == nil {
		t.Fatalf("the vault opened with a folder where its key file belongs")
	}
	if !strings.Contains(err.Error(), home.VaultKeyFile()) {
		t.Errorf("the error %q does not name the key file", err)
	}
}

func TestAKeyFileLongerThanAKeyIsRefused(t *testing.T) {
	home := testkit.NewTempHome(t)
	written := []byte(strings.Repeat("a", 8192))
	if err := os.WriteFile(home.VaultKeyFile(), written, contract.SecretFileMode); err != nil {
		t.Fatalf("writing the over-long key file failed: %v", err)
	}

	_, err := openIn(t, home)
	if err == nil {
		t.Fatalf("the vault opened with a key file far longer than a key")
	}
	if !strings.Contains(err.Error(), home.VaultKeyFile()) {
		t.Errorf("the error %q does not name the key file", err)
	}
}

func TestAVaultFileThatIsNotAPlainFileIsRefused(t *testing.T) {
	home := testkit.NewTempHome(t)
	if err := os.Mkdir(home.VaultFile(), contract.HomeFolderMode); err != nil {
		t.Fatalf("putting a folder where the vault file belongs failed: %v", err)
	}

	_, err := openIn(t, home)
	if err == nil {
		t.Fatalf("the vault opened with a folder where its file belongs")
	}
	if !strings.Contains(err.Error(), home.VaultFile()) {
		t.Errorf("the error %q does not name the vault file", err)
	}
}

func TestAHomeFolderThatCannotBeWrittenToIsRefusedWhenTheKeyIsMade(t *testing.T) {
	home := testkit.NewTempHome(t)
	makeFolderReadOnly(t, home.Root)

	_, err := openIn(t, home)
	if err == nil {
		t.Fatalf("the vault made a key file in a folder it cannot write to")
	}
	if !strings.Contains(err.Error(), "key") {
		t.Errorf("the error %q does not say that the key file could not be made", err)
	}
}

func TestAHomeFolderUnderAPlainFileIsRefused(t *testing.T) {
	root := t.TempDir()
	inTheWay := filepath.Join(root, "coeus")
	if err := os.WriteFile(inTheWay, []byte("this is a file, not a folder"), contract.DataFileMode); err != nil {
		t.Fatalf("writing the file in the way failed: %v", err)
	}

	_, err := openIn(t, contract.NewHome(filepath.Join(inTheWay, ".coeus")))
	if err == nil {
		t.Fatalf("the vault made its home folder underneath a plain file")
	}
	if !strings.Contains(err.Error(), "home folder") {
		t.Errorf("the error %q does not say that the home folder could not be made", err)
	}
}

func TestAFailedWriteLeavesTheVaultExactlyAsItWas(t *testing.T) {
	home := testkit.NewTempHome(t)
	opened, err := openIn(t, home)
	if err != nil {
		t.Fatalf("opening the vault failed: %v", err)
	}
	if err := opened.Add(vault.Entry{Name: "mail", Site: "Fastmail", Password: "a-long-enough-password"}); err != nil {
		t.Fatalf("adding the first entry failed: %v", err)
	}
	makeFolderReadOnly(t, home.Root)

	if err := opened.Add(vault.Entry{Name: "x-account", Site: "X", Password: "another-long-password"}); err == nil {
		t.Fatalf("an entry was added even though the folder cannot be written to")
	}
	if listed := opened.List(); len(listed) != 1 || listed[0].Name != "mail" {
		t.Errorf("the vault holds %v after a failed write, want only the entry it had", listed)
	}
	if err := opened.Remove("mail"); err == nil {
		t.Errorf("an entry was removed even though the folder cannot be written to")
	}
	if listed := opened.List(); len(listed) != 1 {
		t.Errorf("the vault holds %d entries after a failed remove, want the 1 it had", len(listed))
	}
}
