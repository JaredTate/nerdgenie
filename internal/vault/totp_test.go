package vault_test

import (
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/testkit"
	"github.com/JaredTate/nerdgenie/internal/vault"
)

// rfc6238Secret is the twenty-byte seed the RFC 6238 test vectors use, written
// in the base32 form every site hands out, and it is "12345678901234567890".
const rfc6238Secret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

func TestTheCodeMatchesTheRFC6238VectorsForSHA1(t *testing.T) {
	// The RFC prints eight digits; a six-digit code is the last six of them,
	// because the truncation is the same number taken modulo a smaller power of
	// ten.
	vectors := []struct {
		second int64
		code   string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
		{20000000000, "353130"},
	}

	for _, vector := range vectors {
		clock := testkit.NewFakeClock(time.Unix(vector.second, 0).UTC())
		home := testkit.NewTempHome(t)
		opened, err := vault.Open(home, clock)
		if err != nil {
			t.Fatalf("opening the vault failed: %v", err)
		}
		if err := opened.Add(vault.Entry{Name: "x-account", Site: "X", TOTPSecret: rfc6238Secret}); err != nil {
			t.Fatalf("adding the entry failed: %v", err)
		}

		code, _, err := opened.Code("x-account")
		if err != nil {
			t.Fatalf("making the code at second %d failed: %v", vector.second, err)
		}
		if code != vector.code {
			t.Errorf("the code at second %d is %q, want %q", vector.second, code, vector.code)
		}
		if err := opened.Close(); err != nil {
			t.Errorf("closing the vault failed: %v", err)
		}
	}
}

func TestTheSecondsLeftCountDownWithTheClock(t *testing.T) {
	clock := testkit.NewFakeClock(time.Unix(0, 0).UTC())
	home := testkit.NewTempHome(t)
	opened, err := vault.Open(home, clock)
	if err != nil {
		t.Fatalf("opening the vault failed: %v", err)
	}
	defer func() { _ = opened.Close() }()
	if err := opened.Add(vault.Entry{Name: "x-account", Site: "X", TOTPSecret: rfc6238Secret}); err != nil {
		t.Fatalf("adding the entry failed: %v", err)
	}

	steps := []struct {
		advance     time.Duration
		secondsLeft int
	}{
		{0, 30},
		{time.Second, 29},
		{14 * time.Second, 15},
		{14 * time.Second, 1},
		{time.Second, 30},
	}
	for _, step := range steps {
		clock.Advance(step.advance)
		_, secondsLeft, err := opened.Code("x-account")
		if err != nil {
			t.Fatalf("making the code failed: %v", err)
		}
		if secondsLeft != step.secondsLeft {
			t.Errorf("at %s the code has %d seconds left, want %d", clock.Now().UTC(), secondsLeft, step.secondsLeft)
		}
	}
}

func TestTheCodeChangesWhenTheThirtySecondWindowTurnsOver(t *testing.T) {
	clock := testkit.NewFakeClock(time.Unix(0, 0).UTC())
	home := testkit.NewTempHome(t)
	opened, err := vault.Open(home, clock)
	if err != nil {
		t.Fatalf("opening the vault failed: %v", err)
	}
	defer func() { _ = opened.Close() }()
	if err := opened.Add(vault.Entry{Name: "x-account", Site: "X", TOTPSecret: rfc6238Secret}); err != nil {
		t.Fatalf("adding the entry failed: %v", err)
	}

	first, _, err := opened.Code("x-account")
	if err != nil {
		t.Fatalf("making the first code failed: %v", err)
	}
	clock.Advance(29 * time.Second)
	same, _, err := opened.Code("x-account")
	if err != nil {
		t.Fatalf("making the code inside the same window failed: %v", err)
	}
	if same != first {
		t.Errorf("the code changed inside its own thirty-second window, from %q to %q", first, same)
	}
	clock.Advance(time.Second)
	next, _, err := opened.Code("x-account")
	if err != nil {
		t.Fatalf("making the code in the next window failed: %v", err)
	}
	if next == first {
		t.Errorf("the code did not change when the window turned over")
	}
}

func TestAskingForACodeWithoutATwoFactorSecretIsRefused(t *testing.T) {
	opened, _, _ := openTestVault(t)
	if err := opened.Add(vault.Entry{Name: "mail", Site: "Fastmail", Password: "a-long-enough-password"}); err != nil {
		t.Fatalf("adding the entry failed: %v", err)
	}

	_, _, err := opened.Code("mail")
	if err == nil {
		t.Fatalf("an entry with no two-factor secret still made a code")
	}
	if !strings.Contains(err.Error(), "mail") {
		t.Errorf("the error %q does not name the entry", err)
	}

	if _, _, err := opened.Code("no-such-login"); err == nil {
		t.Errorf("an entry the vault does not hold still made a code")
	}
}

func TestATwoFactorSecretThatIsNotBase32IsRefusedByName(t *testing.T) {
	opened, _, _ := openTestVault(t)
	if err := opened.Add(vault.Entry{Name: "x-account", Site: "X", TOTPSecret: "not base32 at all !!"}); err != nil {
		t.Fatalf("adding the entry failed: %v", err)
	}

	_, _, err := opened.Code("x-account")
	if err == nil {
		t.Fatalf("a two-factor secret that is not base32 still made a code")
	}
	if !strings.Contains(err.Error(), "x-account") {
		t.Errorf("the error %q does not name the entry whose secret is wrong", err)
	}
}
