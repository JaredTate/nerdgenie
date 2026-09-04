package vault_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/vault"
)

func TestTheResolverGivesTheHarnessTheLoginBehindAGoodReference(t *testing.T) {
	opened, _, _ := openTestVault(t)
	addThreeEntries(t, opened)

	credential, err := opened.Resolve(context.Background(), contract.SecretReferencePrefix+"x-account")
	if err != nil {
		t.Fatalf("resolving a good reference failed: %v", err)
	}
	if credential.Site != "X" || credential.Username != "jared" {
		t.Errorf("the resolver gave back the wrong entry: site %q, username %q", credential.Site, credential.Username)
	}
	if credential.Password != "correct-horse-battery-staple" {
		t.Errorf("the resolver gave back the wrong password")
	}
	if credential.TOTPSecret != "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ" {
		t.Errorf("the resolver gave back the wrong two-factor secret")
	}
}

func TestTheResolverRefusesAnythingThatIsNotAReferenceAndSaysTheForm(t *testing.T) {
	opened, _, _ := openTestVault(t)
	addThreeEntries(t, opened)

	wrongForms := []string{
		"",
		"x-account",
		"secret:/x-account",
		"https://x.com",
		contract.SecretReferencePrefix,
		contract.SecretReferencePrefix + "two words",
	}
	for _, reference := range wrongForms {
		_, err := opened.Resolve(context.Background(), reference)
		if err == nil {
			t.Errorf("the resolver accepted %q, which is not a secret reference", reference)
			continue
		}
		if !strings.Contains(err.Error(), contract.SecretReferencePrefix+"name") {
			t.Errorf("the error for %q is %q and does not say the form to write", reference, err)
		}
	}
}

func TestTheResolverRefusesANameTheVaultDoesNotHoldAndSaysWhichName(t *testing.T) {
	opened, _, _ := openTestVault(t)
	addThreeEntries(t, opened)

	_, err := opened.Resolve(context.Background(), contract.SecretReferencePrefix+"no-such-login")
	if err == nil {
		t.Fatalf("the resolver answered a name the vault does not hold")
	}
	if !strings.Contains(err.Error(), "no-such-login") {
		t.Errorf("the error %q does not name the secret that is missing", err)
	}
}

func TestWhatTheResolverGivesBackPrintsAsSecret(t *testing.T) {
	opened, _, _ := openTestVault(t)
	addThreeEntries(t, opened)

	credential, err := opened.Resolve(context.Background(), contract.SecretReferencePrefix+"x-account")
	if err != nil {
		t.Fatalf("resolving a good reference failed: %v", err)
	}

	printedForms := []string{fmt.Sprint(credential)}
	for _, verb := range []string{"%v", "%s", "the login is %v"} {
		printedForms = append(printedForms, fmt.Sprintf(verb, credential))
	}
	for _, printed := range printedForms {
		if strings.Contains(printed, "correct-horse") || strings.Contains(printed, "jared") {
			t.Errorf("what the resolver gave back printed as %q, which shows what it holds", printed)
		}
		if !strings.Contains(printed, contract.SecretMarker) {
			t.Errorf("what the resolver gave back printed as %q, want %q", printed, contract.SecretMarker)
		}
	}

	written, err := json.Marshal(struct {
		Login contract.Credential `json:"login"`
	}{Login: credential})
	if err != nil {
		t.Fatalf("turning what the resolver gave back into JSON failed: %v", err)
	}
	if strings.Contains(string(written), "correct-horse") {
		t.Errorf("what the resolver gave back leaked its password into %s", written)
	}
}

func TestAnEntryPrintsAsSecretAndTurnsIntoTheSameJSON(t *testing.T) {
	entry := vault.Entry{
		Name:       "x-account",
		Site:       "X",
		Domains:    []string{"x.com"},
		Username:   "jared",
		Password:   "correct-horse-battery-staple",
		TOTPSecret: "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ",
	}

	printedForms := []string{fmt.Sprint(entry), entry.String()}
	for _, verb := range []string{"%v", "%s", "%q", "the login is %v"} {
		printedForms = append(printedForms, fmt.Sprintf(verb, entry))
	}
	for _, printed := range printedForms {
		if strings.Contains(printed, "correct-horse") || strings.Contains(printed, "jared") {
			t.Errorf("an entry printed as %q, which shows what it holds", printed)
		}
		if !strings.Contains(printed, contract.SecretMarker) {
			t.Errorf("an entry printed as %q, want %q", printed, contract.SecretMarker)
		}
	}

	written, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("turning an entry into JSON failed: %v", err)
	}
	if string(written) != `"`+contract.SecretMarker+`"` {
		t.Errorf("an entry turned into the JSON %s, and it must never carry a value into a tool result", written)
	}

	inside := struct {
		Login vault.Entry `json:"login"`
	}{Login: entry}
	nested, err := json.Marshal(inside)
	if err != nil {
		t.Fatalf("turning a struct holding an entry into JSON failed: %v", err)
	}
	if strings.Contains(string(nested), "correct-horse") {
		t.Errorf("an entry inside another value leaked its password into %s", nested)
	}
}

func TestTheHarnessGetsACopyItCannotChangeTheVaultWith(t *testing.T) {
	opened, _, _ := openTestVault(t)
	addThreeEntries(t, opened)

	credential, err := opened.Resolve(context.Background(), contract.SecretReferencePrefix+"x-account")
	if err != nil {
		t.Fatalf("resolving a good reference failed: %v", err)
	}
	credential.Domains[0] = "evil.example"

	again, err := opened.Resolve(context.Background(), contract.SecretReferencePrefix+"x-account")
	if err != nil {
		t.Fatalf("resolving the same reference again failed: %v", err)
	}
	if again.Domains[0] != "x.com" {
		t.Errorf("changing the domains the resolver gave back changed the vault to %v", again.Domains)
	}
}

func TestTheSudoPasswordIsThereWhenTheEntryIsAndRefusedWhenItIsNot(t *testing.T) {
	opened, _, _ := openTestVault(t)
	ctx := context.Background()

	_, err := opened.SudoPassword(ctx)
	if err == nil {
		t.Fatalf("an empty vault gave up a sudo password")
	}
	if !strings.Contains(err.Error(), "/vault add sudo") {
		t.Errorf("the error %q does not say how to add the sudo password", err)
	}

	if err := opened.Add(vault.Entry{Name: vault.SudoEntryName, Password: "the-machine-password"}); err != nil {
		t.Fatalf("adding the sudo entry failed: %v", err)
	}
	password, err := opened.SudoPassword(ctx)
	if err != nil {
		t.Fatalf("reading the sudo password failed: %v", err)
	}
	if password != "the-machine-password" {
		t.Errorf("the sudo password came back as the wrong value")
	}
}

func TestASudoEntryWithNoPasswordIsTreatedAsNoSudoPassword(t *testing.T) {
	opened, _, _ := openTestVault(t)
	if err := opened.Add(vault.Entry{Name: vault.SudoEntryName, Site: "this machine"}); err != nil {
		t.Fatalf("adding the sudo entry with no password failed: %v", err)
	}
	if _, err := opened.SudoPassword(context.Background()); err == nil {
		t.Errorf("a sudo entry with an empty password was treated as a password")
	}
}
