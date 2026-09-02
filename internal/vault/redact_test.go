package vault_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/vault"
)

// privateKeyBlock is the shape of a key file pasted into a message by mistake.
const privateKeyBlock = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtz
c2gtZWQyNTUxOQAAACBqUyN3sVJqXsjKZaOWQaLQ==
-----END OPENSSH PRIVATE KEY-----`

// redactionCase is one before and after pair.
type redactionCase struct {
	what   string
	before string
	after  string
}

// vaultWithSecretsToRedact returns a vault holding the values the redactor must
// black out wherever they appear.
func vaultWithSecretsToRedact(t testing.TB) *vault.Vault {
	t.Helper()
	opened, _, _ := openTestVault(t)
	entries := []vault.Entry{
		{Name: "x-account", Site: "X", Username: "jared", Password: "correct-horse-battery-staple", TOTPSecret: rfc6238Secret},
		{Name: vault.SudoEntryName, Password: "the-machine-password"},
		{Name: "short", Site: "a toy", Password: "abcde"},
	}
	for _, entry := range entries {
		if err := opened.Add(entry); err != nil {
			t.Fatalf("adding the entry %q failed: %v", entry.Name, err)
		}
	}
	return opened
}

func TestRedactionBlacksOutTwelveSecretShapes(t *testing.T) {
	marker := contract.RedactedMarker
	cases := []redactionCase{
		{
			what:   "a password the vault holds",
			before: "the password is correct-horse-battery-staple and it works",
			after:  "the password is " + marker + " and it works",
		},
		{
			what:   "the sudo password the vault holds",
			before: "echo the-machine-password | sudo -S apt update",
			after:  "echo " + marker + " | sudo -S apt update",
		},
		{
			what:   "a two-factor secret the vault holds",
			before: "the seed is " + rfc6238Secret,
			after:  "the seed is " + marker,
		},
		{
			what:   "an Anthropic key",
			before: "ANTHROPIC_API_KEY=sk-ant-api03-AbCdEf0123456789xyz",
			after:  "ANTHROPIC_API_KEY=" + marker,
		},
		{
			what:   "an OpenAI project key",
			before: "the key sk-proj-AbCdEf0123456789 was rotated",
			after:  "the key " + marker + " was rotated",
		},
		{
			what:   "a bare OpenAI key",
			before: "curl -H 'x-api-key: sk-AbCdEf0123456789' https://example.com",
			after:  "curl -H 'x-api-key: " + marker + "' https://example.com",
		},
		{
			what:   "a bearer token in an Authorization header",
			before: "Authorization: Bearer abc123XYZ.token-value",
			after:  "Authorization: Bearer " + marker,
		},
		{
			what:   "a password in a query string",
			before: "https://example.com/login?password=hunter2secret&next=/home",
			after:  "https://example.com/login?password=" + marker + "&next=/home",
		},
		{
			what:   "a token in a query string",
			before: "POST /webhook?token=abc123def456 HTTP/1.1",
			after:  "POST /webhook?token=" + marker + " HTTP/1.1",
		},
		{
			what:   "an AWS access key id",
			before: "aws_access_key_id = AKIAIOSFODNN7EXAMPLE",
			after:  "aws_access_key_id = " + marker,
		},
		{
			what:   "a private key block",
			before: "here is the key:\n" + privateKeyBlock + "\nthat is all",
			after:  "here is the key:\n" + marker + "\nthat is all",
		},
		{
			what:   "a GitHub personal access token",
			before: "git remote add origin https://ghp_0123456789abcdefABCDEF@github.com/x/y",
			after:  "git remote add origin https://" + marker + "@github.com/x/y",
		},
		{
			what:   "a GitHub OAuth token",
			before: "gho_0123456789abcdefABCDEF is the session token",
			after:  marker + " is the session token",
		},
		{
			what:   "a Slack token",
			before: "SLACK_BOT_TOKEN=xoxb-123456789012-abcdefABCDEF",
			after:  "SLACK_BOT_TOKEN=" + marker,
		},
		{
			what:   "a six-digit code on a line that says code",
			before: "Your verification code is 481920.",
			after:  "Your verification code is " + marker + ".",
		},
		{
			what:   "a six-digit code before the word that says what it is",
			before: "481920 is your one-time code",
			after:  marker + " is your one-time code",
		},
		{
			what:   "a six-digit code on a line that says OTP",
			before: "OTP: 123456",
			after:  "OTP: " + marker,
		},
	}

	redactor := vaultWithSecretsToRedact(t)
	for _, one := range cases {
		got := redactor.Redact(one.before)
		if got != one.after {
			t.Errorf("redacting %s gave\n  %q\nwant\n  %q", one.what, got, one.after)
		}
	}
}

func TestRedactionLeavesOrdinaryTextExactlyAsItWas(t *testing.T) {
	ordinary := []string{
		"there is nothing secret in this sentence",
		"write to someone@example.com about the invoice",
		"the commit is a94a8fe5ccb19ba61c4c0873d391e987982fbbd3",
		"Coeus version 1.2.3 is running on this machine",
		"risk-management is a task-oriented discipline",
		"the file has 123456 lines in it",
		"the tokenizer=fast setting is the default",
		"her passcode was accepted at the door",
		"error 500 came back from the server",
		"abcde is short enough to be an ordinary word",
		"",
		"a line\nand another line\n",
	}

	redactor := vaultWithSecretsToRedact(t)
	for _, text := range ordinary {
		if got := redactor.Redact(text); got != text {
			t.Errorf("redacting ordinary text changed\n  %q\ninto\n  %q", text, got)
		}
	}
}

func TestRedactionBlacksOutEveryCopyOfASecretInOneMessage(t *testing.T) {
	redactor := vaultWithSecretsToRedact(t)
	text := "correct-horse-battery-staple, then correct-horse-battery-staple again, and sk-ant-api03-AbCdEf0123456789"
	got := redactor.Redact(text)

	if strings.Contains(got, "correct-horse") {
		t.Errorf("a repeated password was left showing in %q", got)
	}
	if strings.Contains(got, "sk-ant-") {
		t.Errorf("a key beside a stored password was left showing in %q", got)
	}
	if strings.Count(got, contract.RedactedMarker) != 3 {
		t.Errorf("the text %q should hold three markers", got)
	}
}

func TestAnEmptyVaultStillBlacksOutTheSecretShapes(t *testing.T) {
	opened, _, _ := openTestVault(t)
	got := opened.Redact("the key is sk-ant-api03-AbCdEf0123456789 today")
	if strings.Contains(got, "sk-ant-") {
		t.Errorf("an empty vault left a key showing in %q", got)
	}
	if plain := opened.Redact("nothing secret here at all"); plain != "nothing secret here at all" {
		t.Errorf("an empty vault changed ordinary text into %q", plain)
	}
}

func TestAClosedVaultStopsBlackingOutTheValuesItHeld(t *testing.T) {
	redactor := vaultWithSecretsToRedact(t)
	if err := redactor.Close(); err != nil {
		t.Fatalf("closing the vault failed: %v", err)
	}
	got := redactor.Redact("the key is sk-ant-api03-AbCdEf0123456789 today")
	if strings.Contains(got, "sk-ant-") {
		t.Errorf("a closed vault left a key showing in %q", got)
	}
}

func FuzzRedactNeverPanicsAndNeverLeavesAStoredValue(f *testing.F) {
	seeds := []string{
		"",
		"correct-horse-battery-staple",
		"Authorization: Bearer abc123XYZ",
		"OTP: 123456",
		privateKeyBlock,
		"\x00\xff\xfe not utf-8 at all",
		strings.Repeat("sk-abcdefgh ", 40),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	opened, _, _ := openTestVault(f)
	if err := opened.Add(vault.Entry{Name: "x-account", Site: "X", Password: "correct-horse-battery-staple"}); err != nil {
		f.Fatalf("adding the entry failed: %v", err)
	}

	f.Fuzz(func(t *testing.T, text string) {
		got := opened.Redact(text)
		if strings.Contains(got, "correct-horse-battery-staple") {
			t.Errorf("redacting %q left the stored password showing in %q", text, got)
		}
	})
}
