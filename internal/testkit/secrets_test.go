package testkit_test

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

func TestTheFakeVaultResolvesAReferenceIntoACredential(t *testing.T) {
	secrets := testkit.NewFakeSecrets()
	secrets.Add("x-account", contract.Credential{
		Site:     "x-account",
		Domains:  []string{"x.com"},
		Username: "digibyte",
		Password: "correct horse battery staple",
	})

	found, err := secrets.Resolve(context.Background(), "secret://x-account")
	if err != nil {
		t.Fatalf("resolving a reference that is in the vault failed: %v", err)
	}
	if found.Username != "digibyte" {
		t.Errorf("the credential came back with username %q, want digibyte", found.Username)
	}
}

func TestTheFakeVaultRefusesAReferenceItDoesNotHold(t *testing.T) {
	secrets := testkit.NewFakeSecrets()

	for _, reference := range []string{"secret://nothing", "x-account", ""} {
		if _, err := secrets.Resolve(context.Background(), reference); err == nil {
			t.Errorf("resolving %q was reported as a success, want an error", reference)
		}
	}
}

func TestTheFakeVaultRedactsEveryValueItHolds(t *testing.T) {
	secrets := testkit.NewFakeSecrets()
	secrets.Add("x-account", contract.Credential{
		Username:   "digibyte",
		Password:   "correct horse battery staple",
		TOTPSecret: "JBSWY3DPEHPK3PXP",
	})
	secrets.SetSudoPassword("open sesame please")

	redacted := secrets.Redact("logging in as digibyte with correct horse battery staple and code JBSWY3DPEHPK3PXP, sudo open sesame please")

	for _, value := range []string{"correct horse battery staple", "JBSWY3DPEHPK3PXP", "open sesame please"} {
		if strings.Contains(redacted, value) {
			t.Errorf("the redacted text still holds %q: %s", value, redacted)
		}
	}
	if !strings.Contains(redacted, contract.RedactedMarker) {
		t.Errorf("the redacted text has no %s marker in it: %s", contract.RedactedMarker, redacted)
	}
	if !strings.Contains(redacted, "digibyte") {
		t.Error("the redactor removed the username, and only secret values should go")
	}
}

func TestRedactingDoesNotCorruptTextItHasAlreadyRedacted(t *testing.T) {
	secrets := testkit.NewFakeSecrets()
	secrets.Add("long", contract.Credential{Password: "hunter2hunter2"})
	secrets.Add("short", contract.Credential{Password: "act"})

	redacted := secrets.Redact("logging in with hunter2hunter2 now")

	if redacted != "logging in with "+contract.RedactedMarker+" now" {
		t.Errorf("the redacted text is %q, and a short secret matched inside the marker a longer one had already written", redacted)
	}
}

func TestTheLongestSecretGoesFirstSoNoneIsLeftHalfVisible(t *testing.T) {
	secrets := testkit.NewFakeSecrets()
	secrets.Add("part", contract.Credential{Password: "hunter2"})
	secrets.Add("whole", contract.Credential{Password: "hunter2hunter2"})

	redacted := secrets.Redact("the password is hunter2hunter2 today")

	if redacted != "the password is "+contract.RedactedMarker+" today" {
		t.Errorf("the redacted text is %q, and the whole secret should have gone in one piece", redacted)
	}
}

func TestRedactingTwiceChangesNothingTheSecondTime(t *testing.T) {
	secrets := testkit.NewFakeSecrets()
	secrets.Add("x-account", contract.Credential{Password: "correct horse battery staple"})

	once := secrets.Redact("logging in with correct horse battery staple now")
	twice := secrets.Redact(once)

	if once != twice {
		t.Errorf("redacting twice gave %q then %q, and the second pass should change nothing", once, twice)
	}
}

func FuzzRedact(f *testing.F) {
	for _, seed := range []string{
		"", "nothing secret here", "hunter2hunter2", "[redacted]",
		"logging in with hunter2hunter2 now", "actactact",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		secrets := testkit.NewFakeSecrets()
		secrets.Add("long", contract.Credential{Password: "hunter2hunter2"})
		secrets.Add("short", contract.Credential{Password: "act"})
		secrets.SetSudoPassword("open sesame")

		redacted := secrets.Redact(text)

		// The marker itself spells "act" inside "redacted", so the markers are
		// taken out before the search: nothing secret may survive anywhere else.
		outsideTheMarkers := strings.ReplaceAll(redacted, contract.RedactedMarker, "\x00")
		for _, value := range []string{"hunter2hunter2", "act", "open sesame"} {
			if strings.Contains(outsideTheMarkers, value) {
				t.Fatalf("the redacted text still holds %q: %q came out as %q", value, text, redacted)
			}
		}
		if again := secrets.Redact(redacted); again != redacted {
			t.Fatalf("redacting twice changed the text: %q became %q then %q", text, redacted, again)
		}
	})
}

func TestTheFakeVaultServesTheSudoPassword(t *testing.T) {
	secrets := testkit.NewFakeSecrets()
	secrets.SetSudoPassword("open sesame please")

	password, err := secrets.SudoPassword(context.Background())
	if err != nil {
		t.Fatalf("getting the sudo password failed: %v", err)
	}
	if password != "open sesame please" {
		t.Errorf("the sudo password came back as %q, want the one that was set", password)
	}

	empty := testkit.NewFakeSecrets()
	if _, err := empty.SudoPassword(context.Background()); err == nil {
		t.Error("getting a sudo password nobody set was reported as a success, want an error")
	}
}

func TestTheFakeVaultKeepsTheSecretsContract(t *testing.T) {
	if err := testkit.CheckSecrets(context.Background(), testkit.NewFakeSecrets()); err != nil {
		t.Fatalf("the fake vault does not keep the secrets contract: %v", err)
	}
}
