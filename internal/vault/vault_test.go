package vault_test

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/clock"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
	"github.com/JaredTate/coeus/internal/vault"
)

// openTestVault opens a vault in a temporary home with a clock a test controls.
func openTestVault(t testing.TB) (*vault.Vault, contract.Home, *testkit.FakeClock) {
	t.Helper()
	home := testkit.NewTempHome(t)
	testClock := testkit.NewFakeClock(time.Unix(0, 0).UTC())
	opened, err := vault.Open(home, testClock)
	if err != nil {
		t.Fatalf("opening the vault in a temporary home failed: %v", err)
	}
	t.Cleanup(func() { _ = opened.Close() })
	return opened, home, testClock
}

// threeEntries is the set the round-trip tests add.
func threeEntries() []vault.Entry {
	return []vault.Entry{
		{
			Name:       "x-account",
			Site:       "X",
			Domains:    []string{"x.com", "twitter.com"},
			Username:   "jared",
			Password:   "correct-horse-battery-staple",
			TOTPSecret: "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ",
		},
		{
			Name:     "mail",
			Site:     "Fastmail",
			Domains:  []string{"fastmail.com"},
			Username: "someone@example.com",
			Password: "another-long-password",
		},
		{
			Name:     vault.SudoEntryName,
			Password: "the-machine-password",
		},
	}
}

// addThreeEntries puts the three round-trip entries in the vault.
func addThreeEntries(t testing.TB, opened *vault.Vault) {
	t.Helper()
	for _, entry := range threeEntries() {
		if err := opened.Add(entry); err != nil {
			t.Fatalf("adding the entry %q failed: %v", entry.Name, err)
		}
	}
}

func TestAThreeEntryVaultSurvivesCloseAndReopen(t *testing.T) {
	opened, home, testClock := openTestVault(t)
	addThreeEntries(t, opened)
	if err := opened.Close(); err != nil {
		t.Fatalf("closing the vault failed: %v", err)
	}

	reopened, err := vault.Open(home, testClock)
	if err != nil {
		t.Fatalf("reopening the vault failed: %v", err)
	}
	defer func() { _ = reopened.Close() }()

	ctx := context.Background()
	credential, err := reopened.Resolve(ctx, contract.SecretReferencePrefix+"x-account")
	if err != nil {
		t.Fatalf("resolving the entry the reopened vault should hold failed: %v", err)
	}
	if credential.Username != "jared" || credential.Password != "correct-horse-battery-staple" {
		t.Errorf("the reopened vault gave back the wrong login for x-account")
	}
	if len(credential.Domains) != 2 || credential.Domains[0] != "x.com" {
		t.Errorf("the reopened vault gave back the domains %v, want x.com and twitter.com", credential.Domains)
	}

	password, err := reopened.SudoPassword(ctx)
	if err != nil {
		t.Fatalf("reading the sudo password from the reopened vault failed: %v", err)
	}
	if password != "the-machine-password" {
		t.Errorf("the sudo password came back wrong")
	}
}

func TestTheVaultFileOnDiskIsNotPlaintext(t *testing.T) {
	opened, home, _ := openTestVault(t)
	addThreeEntries(t, opened)

	written, err := os.ReadFile(home.VaultFile())
	if err != nil {
		t.Fatalf("reading the vault file failed: %v", err)
	}
	for _, value := range []string{"correct-horse-battery-staple", "the-machine-password", "jared", "x.com"} {
		if bytes.Contains(written, []byte(value)) {
			t.Errorf("the vault file holds %q in the clear, and it must be encrypted", value)
		}
	}
	if !bytes.HasPrefix(written, []byte("age-encryption.org/v1")) {
		t.Errorf("the vault file does not begin with the age header, so it is not an age file")
	}

	info, err := os.Stat(home.VaultFile())
	if err != nil {
		t.Fatalf("reading the mode of the vault file failed: %v", err)
	}
	if info.Mode().Perm() != contract.SecretFileMode {
		t.Errorf("the vault file has mode %v, want %v", info.Mode().Perm(), contract.SecretFileMode)
	}
}

func TestTheListingShowsNamesAndSitesAndNeverAValue(t *testing.T) {
	opened, _, _ := openTestVault(t)
	addThreeEntries(t, opened)

	listed := opened.List()
	if len(listed) != 3 {
		t.Fatalf("the listing has %d lines, want 3", len(listed))
	}
	if listed[0].Name != "mail" || listed[1].Name != vault.SudoEntryName || listed[2].Name != "x-account" {
		t.Errorf("the listing is not in name order: %v", listed)
	}
	if listed[2].Site != "X" {
		t.Errorf("the listing gave the site %q for x-account, want X", listed[2].Site)
	}
	for _, line := range listed {
		if strings.Contains(line.Name+line.Site, "correct-horse") {
			t.Errorf("the listing shows a value, and it must show only names and sites")
		}
	}
}

func TestAddingTheSameNameTwiceKeepsOneEntryWithTheNewValue(t *testing.T) {
	opened, _, _ := openTestVault(t)
	first := vault.Entry{Name: "mail", Site: "Fastmail", Username: "jared", Password: "the-old-password"}
	second := vault.Entry{Name: "mail", Site: "Fastmail", Username: "jared", Password: "the-new-password"}
	for _, entry := range []vault.Entry{first, second} {
		if err := opened.Add(entry); err != nil {
			t.Fatalf("adding the entry %q failed: %v", entry.Name, err)
		}
	}

	if listed := opened.List(); len(listed) != 1 {
		t.Fatalf("the listing has %d lines after adding the same name twice, want 1", len(listed))
	}
	credential, err := opened.Resolve(context.Background(), contract.SecretReferencePrefix+"mail")
	if err != nil {
		t.Fatalf("resolving the entry that was added twice failed: %v", err)
	}
	if credential.Password != "the-new-password" {
		t.Errorf("the second add did not replace the first")
	}
}

func TestAddingAnEntryWithNoNameIsRefused(t *testing.T) {
	opened, _, _ := openTestVault(t)
	err := opened.Add(vault.Entry{Password: "a-long-enough-password"})
	if err == nil {
		t.Fatalf("adding an entry with no name was allowed, and a name is what the reference points at")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("the error %q does not say that the name is missing", err)
	}
}

func TestRemovingAnEntryTakesItOutAndRemovingAMissingOneSaysSo(t *testing.T) {
	opened, home, testClock := openTestVault(t)
	if err := opened.Add(vault.Entry{Name: "mail", Site: "Fastmail", Password: "a-long-enough-password"}); err != nil {
		t.Fatalf("adding the entry failed: %v", err)
	}
	if err := opened.Remove("mail"); err != nil {
		t.Fatalf("removing the entry failed: %v", err)
	}
	if listed := opened.List(); len(listed) != 0 {
		t.Errorf("the listing still has %d lines after the only entry was removed", len(listed))
	}

	err := opened.Remove("mail")
	if err == nil {
		t.Fatalf("removing an entry the vault does not hold was allowed")
	}
	if !strings.Contains(err.Error(), "mail") {
		t.Errorf("the error %q does not name the entry that is missing", err)
	}

	if err := opened.Close(); err != nil {
		t.Fatalf("closing the vault failed: %v", err)
	}
	reopened, err := vault.Open(home, testClock)
	if err != nil {
		t.Fatalf("reopening the emptied vault failed: %v", err)
	}
	if listed := reopened.List(); len(listed) != 0 {
		t.Errorf("the reopened vault has %d entries, want none", len(listed))
	}
	if err := reopened.Close(); err != nil {
		t.Errorf("closing the reopened vault failed: %v", err)
	}
}

func TestAClosedVaultRefusesEveryCall(t *testing.T) {
	home := testkit.NewTempHome(t)
	opened, err := vault.Open(home, testkit.NewFakeClock(time.Unix(0, 0).UTC()))
	if err != nil {
		t.Fatalf("opening the vault failed: %v", err)
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("closing the vault failed: %v", err)
	}
	if err := opened.Close(); err != nil {
		t.Errorf("closing an already closed vault should do nothing, but it failed: %v", err)
	}

	if err := opened.Add(vault.Entry{Name: "mail", Password: "a-long-enough-password"}); err == nil {
		t.Errorf("a closed vault accepted an entry")
	}
	if err := opened.Remove("mail"); err == nil {
		t.Errorf("a closed vault removed an entry")
	}
	if _, err := opened.Resolve(context.Background(), contract.SecretReferencePrefix+"mail"); err == nil {
		t.Errorf("a closed vault resolved a reference")
	}
	if _, err := opened.SudoPassword(context.Background()); err == nil {
		t.Errorf("a closed vault gave up the sudo password")
	}
	if listed := opened.List(); len(listed) != 0 {
		t.Errorf("a closed vault listed %d entries, want none", len(listed))
	}
}

func TestTheKeyFileIsMadeOnFirstUseWithOwnerOnlyMode(t *testing.T) {
	_, home, _ := openTestVault(t)
	info, err := os.Stat(home.VaultKeyFile())
	if err != nil {
		t.Fatalf("the key file was not made on first use: %v", err)
	}
	if info.Mode().Perm() != contract.SecretFileMode {
		t.Errorf("the key file has mode %v, want %v", info.Mode().Perm(), contract.SecretFileMode)
	}
}

func TestALooseKeyFileModeIsRefusedWithTheModeToSet(t *testing.T) {
	for _, mode := range []fs.FileMode{0o644, 0o660, 0o604, 0o666} {
		home := testkit.NewTempHome(t)
		testClock := testkit.NewFakeClock(time.Unix(0, 0).UTC())
		makeKeyFile(t, home, testClock)
		if err := os.Chmod(home.VaultKeyFile(), mode); err != nil {
			t.Fatalf("loosening the key file mode failed: %v", err)
		}

		_, err := vault.Open(home, testClock)
		if err == nil {
			t.Fatalf("the vault opened with a key file of mode %v, which others can read", mode)
		}
		if !strings.Contains(err.Error(), "0600") {
			t.Errorf("the error for mode %v is %q and does not say the mode to set", mode, err)
		}
		if !strings.Contains(err.Error(), home.VaultKeyFile()) {
			t.Errorf("the error for mode %v is %q and does not name the key file", mode, err)
		}
	}
}

func TestAnOwnerOnlyKeyFileIsAccepted(t *testing.T) {
	home := testkit.NewTempHome(t)
	testClock := testkit.NewFakeClock(time.Unix(0, 0).UTC())
	makeKeyFile(t, home, testClock)
	if err := os.Chmod(home.VaultKeyFile(), contract.SecretFileMode); err != nil {
		t.Fatalf("setting the key file mode failed: %v", err)
	}

	reopened, err := vault.Open(home, testClock)
	if err != nil {
		t.Fatalf("the vault refused a key file of mode 0600, which is the right mode: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Errorf("closing the reopened vault failed: %v", err)
	}
}

// makeKeyFile opens and closes a vault once, so that its key file exists.
func makeKeyFile(t *testing.T, home contract.Home, testClock contract.Clock) {
	t.Helper()
	opened, err := vault.Open(home, testClock)
	if err != nil {
		t.Fatalf("opening the vault to make its key failed: %v", err)
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("closing the vault failed: %v", err)
	}
}

func TestAKeyFileThatIsNotAnIdentityIsRefusedByName(t *testing.T) {
	home := testkit.NewTempHome(t)
	if err := os.WriteFile(home.VaultKeyFile(), []byte("this is not an age identity\n"), contract.SecretFileMode); err != nil {
		t.Fatalf("writing the broken key file failed: %v", err)
	}
	_, err := vault.Open(home, testkit.NewFakeClock(time.Unix(0, 0).UTC()))
	if err == nil {
		t.Fatalf("the vault opened with a key file that is not an age identity")
	}
	if !strings.Contains(err.Error(), home.VaultKeyFile()) {
		t.Errorf("the error %q does not name the key file", err)
	}
}

func TestAVaultFileThatWillNotDecryptIsRefusedByName(t *testing.T) {
	opened, home, testClock := openTestVault(t)
	if err := opened.Add(vault.Entry{Name: "mail", Password: "a-long-enough-password"}); err != nil {
		t.Fatalf("adding the entry failed: %v", err)
	}
	if err := opened.Close(); err != nil {
		t.Fatalf("closing the vault failed: %v", err)
	}
	if err := os.WriteFile(home.VaultFile(), []byte("not an age file at all"), contract.SecretFileMode); err != nil {
		t.Fatalf("overwriting the vault file failed: %v", err)
	}

	_, err := vault.Open(home, testClock)
	if err == nil {
		t.Fatalf("the vault opened on a file that is not an age file")
	}
	if !strings.Contains(err.Error(), home.VaultFile()) {
		t.Errorf("the error %q does not name the vault file", err)
	}
}

func TestAVaultWithoutAClockIsRefused(t *testing.T) {
	home := testkit.NewTempHome(t)
	opened, err := vault.Open(home, nil)
	if err == nil {
		_ = opened.Close()
		t.Fatalf("the vault opened with no clock, and it needs one to say when a code runs out")
	}
	if !strings.Contains(err.Error(), "clock") {
		t.Errorf("the error %q does not say that a clock is missing", err)
	}
}

func TestAVaultOnTheRealClockCountsDownAgainstTheMachine(t *testing.T) {
	home := testkit.NewTempHome(t)
	opened, err := vault.Open(home, clock.System())
	if err != nil {
		t.Fatalf("opening the vault on the real clock failed: %v", err)
	}
	defer func() { _ = opened.Close() }()
	if err := opened.Add(vault.Entry{Name: "x-account", Site: "X", TOTPSecret: rfc6238Secret}); err != nil {
		t.Fatalf("adding the entry failed: %v", err)
	}

	_, secondsLeft, err := opened.Code("x-account")
	if err != nil {
		t.Fatalf("making a code on the machine's own clock failed: %v", err)
	}
	wanted := vault.CodeSeconds - int(time.Now().Unix()%vault.CodeSeconds)
	if secondsLeft < 1 || secondsLeft > vault.CodeSeconds {
		t.Errorf("the code has %d seconds left, which is outside the window of %d", secondsLeft, vault.CodeSeconds)
	}
	if secondsLeft != wanted && secondsLeft != wanted-1 {
		t.Errorf("the code has %d seconds left, want about %d from the machine's own clock", secondsLeft, wanted)
	}
}

func TestTheVaultKeepsTheSecretsContract(t *testing.T) {
	opened, _, _ := openTestVault(t)
	if err := testkit.CheckSecrets(context.Background(), opened); err != nil {
		t.Errorf("the real vault breaks the secrets contract: %v", err)
	}
}
