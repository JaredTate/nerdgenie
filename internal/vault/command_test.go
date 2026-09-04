package vault_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/vault"
)

// runVaultCommand runs the slash command over a channel of the given name and
// gives back what the user would see.
func runVaultCommand(t *testing.T, opened *vault.Vault, channel *testkit.FakeChannel, arguments string) string {
	t.Helper()
	command := vault.NewCommand(opened)
	reply, err := command.Run(context.Background(), arguments, contract.CommandContext{Channel: channel})
	if err != nil {
		t.Fatalf("the vault command with the arguments %q failed: %v", arguments, err)
	}
	return reply
}

func TestTheVaultCommandIsNamedAndTerminalOnly(t *testing.T) {
	opened, _, _ := openTestVault(t)
	command := vault.NewCommand(opened)

	if command.Name != "vault" {
		t.Errorf("the command is named %q, want vault", command.Name)
	}
	if command.Help == "" {
		t.Errorf("the command has no help line")
	}
	if !command.TerminalOnly {
		t.Errorf("the command is not marked terminal only, so the registry would let it run over Signal")
	}
	if command.Run == nil {
		t.Errorf("the command has nothing to run")
	}
}

func TestTheVaultCommandIsRefusedOverAChannelThatIsNotTheTerminal(t *testing.T) {
	opened, _, _ := openTestVault(t)
	addThreeEntries(t, opened)
	overSignal := testkit.NewFakeChannel("signal")
	overSignal.AnswerSecretWith("hunter2")

	for _, arguments := range []string{"", "list", "add mail", "remove mail", "test x-account"} {
		reply := runVaultCommand(t, opened, overSignal, arguments)
		if !strings.Contains(reply, "terminal") {
			t.Errorf("the reply to %q over Signal is %q and does not say the vault works only in the terminal", arguments, reply)
		}
		if strings.Contains(reply, "x-account") || strings.Contains(reply, "correct-horse") {
			t.Errorf("the reply to %q over Signal is %q and shows what the vault holds", arguments, reply)
		}
		if len(strings.Split(strings.TrimSpace(reply), "\n")) != 1 {
			t.Errorf("the reply to %q over Signal is %q, and it should be one line", arguments, reply)
		}
	}

	if listed := opened.List(); len(listed) != 3 {
		t.Errorf("the vault holds %d entries after the refused commands, want the 3 it started with", len(listed))
	}
}

func TestTheListingOverTheTerminalShowsNamesAndSitesAndNoValues(t *testing.T) {
	opened, _, _ := openTestVault(t)
	addThreeEntries(t, opened)
	terminal := testkit.NewFakeChannel("terminal")

	reply := runVaultCommand(t, opened, terminal, "list")
	for _, wanted := range []string{"x-account", "X", "mail", "Fastmail", vault.SudoEntryName} {
		if !strings.Contains(reply, wanted) {
			t.Errorf("the listing %q does not hold %q", reply, wanted)
		}
	}
	for _, forbidden := range []string{"correct-horse-battery-staple", "another-long-password", "the-machine-password", rfc6238Secret} {
		if strings.Contains(reply, forbidden) {
			t.Errorf("the listing %q shows a value", reply)
		}
	}
}

func TestTheListingOfAnEmptyVaultSaysSo(t *testing.T) {
	opened, _, _ := openTestVault(t)
	terminal := testkit.NewFakeChannel("terminal")

	reply := runVaultCommand(t, opened, terminal, "")
	if !strings.Contains(strings.ToLower(reply), "empty") {
		t.Errorf("the listing of an empty vault is %q and does not say that it is empty", reply)
	}
}

func TestAddingAnEntryTakesThePlainFieldsFromTheLineAndMasksOnlyTheSecrets(t *testing.T) {
	opened, _, _ := openTestVault(t)
	terminal := testkit.NewFakeChannel("terminal")
	terminal.AnswerSecretWith("hunter2")

	reply := runVaultCommand(t, opened, terminal, "add mail Fastmail fastmail.com,fastmail.fm someone")
	if strings.Contains(reply, "hunter2") {
		t.Errorf("the reply to add is %q and shows what was typed", reply)
	}
	if !strings.Contains(reply, "mail") {
		t.Errorf("the reply to add is %q and does not name the entry that was added", reply)
	}

	credential, err := opened.Resolve(context.Background(), contract.SecretReferencePrefix+"mail")
	if err != nil {
		t.Fatalf("resolving the entry that was just added failed: %v", err)
	}
	if credential.Site != "Fastmail" {
		t.Errorf("the site was stored as %q, want the one typed in the open on the command line", credential.Site)
	}
	if credential.Username != "someone" {
		t.Errorf("the login name was stored as %q, want the one typed in the open", credential.Username)
	}
	if len(credential.Domains) != 2 || credential.Domains[0] != "fastmail.com" || credential.Domains[1] != "fastmail.fm" {
		t.Errorf("the domains were stored as %v, want both of the ones on the command line", credential.Domains)
	}
	if credential.Password != "hunter2" {
		t.Errorf("the password was stored as %q, want the one from the masked prompt", credential.Password)
	}
}

func TestAddingAnEntryWithoutAllFourFieldsPrintsTheForm(t *testing.T) {
	opened, _, _ := openTestVault(t)
	terminal := testkit.NewFakeChannel("terminal")
	terminal.AnswerSecretWith("hunter2")

	tooFew := []string{
		"add",
		"add mail",
		"add mail Fastmail",
		"add mail Fastmail fastmail.com",
		"add " + vault.SudoEntryName + " and something else",
	}
	for _, arguments := range tooFew {
		reply := runVaultCommand(t, opened, terminal, arguments)
		if !strings.Contains(reply, "<site>") || !strings.Contains(reply, "<username>") {
			t.Errorf("the reply to %q is %q and does not print the form to write", arguments, reply)
		}
		if listed := opened.List(); len(listed) != 0 {
			t.Fatalf("%q added an entry even though the line was short", arguments)
		}
	}
}

func TestAddingTheSudoEntryAsksOnlyForThePassword(t *testing.T) {
	opened, _, _ := openTestVault(t)
	terminal := testkit.NewFakeChannel("terminal")
	terminal.AnswerSecretWith("the-machine-password")

	reply := runVaultCommand(t, opened, terminal, "add "+vault.SudoEntryName)
	if strings.Contains(reply, "the-machine-password") {
		t.Errorf("the reply to adding the sudo entry is %q and shows the password", reply)
	}

	listed := opened.List()
	if len(listed) != 1 || listed[0].Name != vault.SudoEntryName || listed[0].Site != "" {
		t.Errorf("the sudo entry came out as %v, want a name on its own with no site to ask for", listed)
	}

	password, err := opened.SudoPassword(context.Background())
	if err != nil {
		t.Fatalf("reading the sudo password that was just added failed: %v", err)
	}
	if password != "the-machine-password" {
		t.Errorf("the sudo password was stored as the wrong value")
	}
}

func TestAddingIsRefusedOverAChannelThatCannotHideTyping(t *testing.T) {
	opened, _, _ := openTestVault(t)
	terminal := testkit.NewFakeChannel("terminal")
	terminal.CannotMaskSecrets()

	reply := runVaultCommand(t, opened, terminal, "add mail Fastmail fastmail.com someone")
	if !strings.Contains(reply, "terminal") {
		t.Errorf("the reply is %q and does not say where a secret can be entered", reply)
	}
	if listed := opened.List(); len(listed) != 0 {
		t.Errorf("an entry was stored even though the channel cannot hide what is typed")
	}
}

func TestRemovingAnEntryOverTheTerminalTakesItOut(t *testing.T) {
	opened, _, _ := openTestVault(t)
	addThreeEntries(t, opened)
	terminal := testkit.NewFakeChannel("terminal")

	reply := runVaultCommand(t, opened, terminal, "remove mail")
	if !strings.Contains(reply, "mail") {
		t.Errorf("the reply to remove is %q and does not name the entry", reply)
	}
	for _, line := range opened.List() {
		if line.Name == "mail" {
			t.Errorf("the entry is still in the vault after it was removed")
		}
	}

	command := vault.NewCommand(opened)
	if _, err := command.Run(context.Background(), "remove mail", contract.CommandContext{Channel: terminal}); err == nil {
		t.Errorf("removing an entry the vault does not hold was answered as though it worked")
	}
}

func TestTestingAnEntrySaysItDecryptsWithoutShowingAnything(t *testing.T) {
	opened, _, _ := openTestVault(t)
	addThreeEntries(t, opened)
	terminal := testkit.NewFakeChannel("terminal")

	withCode := runVaultCommand(t, opened, terminal, "test x-account")
	if !strings.Contains(withCode, "x-account") {
		t.Errorf("the reply %q does not name the entry", withCode)
	}
	if !strings.Contains(withCode, "code") {
		t.Errorf("the reply %q does not say that a code was generated", withCode)
	}
	for _, forbidden := range []string{"correct-horse-battery-staple", rfc6238Secret, "jared"} {
		if strings.Contains(withCode, forbidden) {
			t.Errorf("the reply %q shows a value", withCode)
		}
	}
	for _, digit := range []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"} {
		if strings.Contains(withCode, digit) {
			t.Errorf("the reply %q holds a digit, and the code itself must never be printed", withCode)
			break
		}
	}

	withoutCode := runVaultCommand(t, opened, terminal, "test mail")
	if strings.Contains(withoutCode, "another-long-password") {
		t.Errorf("the reply %q shows a value", withoutCode)
	}
}

func TestTestingAnEntryTheVaultDoesNotHoldIsAnError(t *testing.T) {
	opened, _, _ := openTestVault(t)
	terminal := testkit.NewFakeChannel("terminal")
	command := vault.NewCommand(opened)

	if _, err := command.Run(context.Background(), "test no-such-login", contract.CommandContext{Channel: terminal}); err == nil {
		t.Errorf("testing an entry the vault does not hold was answered as though it worked")
	}
}

func TestTestingAnEntryInAClosedVaultIsAnError(t *testing.T) {
	opened, _, _ := openTestVault(t)
	addThreeEntries(t, opened)
	if err := opened.Close(); err != nil {
		t.Fatalf("closing the vault failed: %v", err)
	}

	if _, err := opened.Check("x-account"); err == nil {
		t.Errorf("a closed vault tested an entry")
	}
}

func TestAddingAnEntryTheVaultWillNotTakeIsAnError(t *testing.T) {
	opened, _, _ := openTestVault(t)
	terminal := testkit.NewFakeChannel("terminal")
	terminal.AnswerSecretWith("hunter2")
	command := vault.NewCommand(opened)

	tooLong := strings.Repeat("n", 200)
	if _, err := command.Run(context.Background(), "add "+tooLong+" Fastmail fastmail.com someone", contract.CommandContext{Channel: terminal}); err == nil {
		t.Errorf("an entry with a name far over the limit was added")
	}
	if listed := opened.List(); len(listed) != 0 {
		t.Errorf("the vault holds %d entries after a refused add, want none", len(listed))
	}
}

func TestAWordTheVaultCommandDoesNotKnowPrintsTheFourItDoes(t *testing.T) {
	opened, _, _ := openTestVault(t)
	terminal := testkit.NewFakeChannel("terminal")

	for _, arguments := range []string{"frobnicate", "remove", "test"} {
		reply := runVaultCommand(t, opened, terminal, arguments)
		for _, wanted := range []string{"list", "add", "remove", "test"} {
			if !strings.Contains(reply, wanted) {
				t.Errorf("the reply to %q is %q and does not offer %q", arguments, reply, wanted)
			}
		}
	}
}
