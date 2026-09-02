package functional

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
	"github.com/JaredTate/coeus/internal/vault"
)

// TestASecretGoesInThroughTheTerminalAndNeverComesBackOut is the whole vault
// feature in one test: the user adds a login in the terminal, the listing shows
// it without its values, the harness resolves the reference to fill a login
// form, the sudo password reaches the askpass path, and a message on its way out
// of the program has every value blacked out.
func TestASecretGoesInThroughTheTerminalAndNeverComesBackOut(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	home := testkit.NewTempHome(t)
	store, err := vault.Open(home, testkit.NewFakeClock(time.Unix(0, 0).UTC()))
	if err != nil {
		t.Fatalf("opening the vault in a temporary home failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	terminal := testkit.NewFakeChannel("terminal")
	command := vault.NewCommand(store)
	where := contract.CommandContext{Channel: terminal}

	// The fake channel answers every masked prompt with the same text, so the
	// login with five values is added the way the command adds it, and the sudo
	// entry, which is one prompt and one value, goes the whole way through the
	// command.
	login := vault.Entry{
		Name:     "x-account",
		Site:     "X",
		Domains:  []string{"x.com"},
		Username: "jared",
		Password: "correct-horse-battery-staple",
	}
	if err := store.Add(login); err != nil {
		t.Fatalf("adding the login failed: %v", err)
	}
	terminal.AnswerSecretWith("the-machine-password")
	if _, err := command.Run(ctx, "add "+vault.SudoEntryName, where); err != nil {
		t.Fatalf("adding the sudo password in the terminal failed: %v", err)
	}

	listing, err := command.Run(ctx, "list", where)
	if err != nil {
		t.Fatalf("listing the vault failed: %v", err)
	}
	for _, wanted := range []string{"x-account", "X", vault.SudoEntryName} {
		if !strings.Contains(listing, wanted) {
			t.Errorf("the listing %q does not hold %q", listing, wanted)
		}
	}
	for _, value := range []string{"correct-horse-battery-staple", "the-machine-password"} {
		if strings.Contains(listing, value) {
			t.Errorf("the listing %q shows a value", listing)
		}
	}

	checkTheHarnessGetsTheValues(ctx, t, store)
	checkNothingLeavesTheProgram(t, store)
}

// checkTheHarnessGetsTheValues is the only way a value leaves the vault: the
// login tool resolves the reference the model wrote, and the askpass path reads
// the machine password.
func checkTheHarnessGetsTheValues(ctx context.Context, t *testing.T, store *vault.Vault) {
	t.Helper()
	credential, err := store.Resolve(ctx, contract.SecretReferencePrefix+"x-account")
	if err != nil {
		t.Fatalf("the harness could not resolve the reference the model would write: %v", err)
	}
	if credential.Password != "correct-horse-battery-staple" {
		t.Errorf("the harness was handed the wrong password to type into the form")
	}
	password, err := store.SudoPassword(ctx)
	if err != nil {
		t.Fatalf("the askpass path could not read the sudo password: %v", err)
	}
	if password != "the-machine-password" {
		t.Errorf("the askpass path was handed the wrong machine password")
	}
}

// checkNothingLeavesTheProgram runs a message that holds both stored values and
// a key the vault never saw through the one redaction pass.
func checkNothingLeavesTheProgram(t *testing.T, store *vault.Vault) {
	t.Helper()
	onTheWayOut := "I logged in with correct-horse-battery-staple and ran sudo with the-machine-password, using the key sk-ant-api03-AbCdEf0123456789"
	redacted := store.Redact(onTheWayOut)
	for _, value := range []string{"correct-horse-battery-staple", "the-machine-password", "sk-ant-"} {
		if strings.Contains(redacted, value) {
			t.Errorf("the message on its way out still holds %q: %q", value, redacted)
		}
	}
	if strings.Count(redacted, contract.RedactedMarker) != 3 {
		t.Errorf("the message on its way out is %q, want three things blacked out", redacted)
	}
}

func TestTheVaultAnswersOneLineOverAChannelThatIsNotTheTerminal(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	home := testkit.NewTempHome(t)
	store, err := vault.Open(home, testkit.NewFakeClock(time.Unix(0, 0).UTC()))
	if err != nil {
		t.Fatalf("opening the vault in a temporary home failed: %v", err)
	}
	defer func() { _ = store.Close() }()

	overSignal := testkit.NewFakeChannel("signal")
	overSignal.CannotMaskSecrets()
	reply, err := vault.NewCommand(store).Run(ctx, "add x-account", contract.CommandContext{Channel: overSignal})
	if err != nil {
		t.Fatalf("the vault command over Signal failed rather than answering: %v", err)
	}
	if !strings.Contains(reply, "terminal") {
		t.Errorf("the reply over Signal is %q and does not say where the vault works", reply)
	}
	if listed := store.List(); len(listed) != 0 {
		t.Errorf("the vault holds %d entries after a command sent over Signal, want none", len(listed))
	}
}
