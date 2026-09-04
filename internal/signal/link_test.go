package signal

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/nerdgenie/internal/testkit"
)

func TestLinkDrawsTheCodeWaitsForThePhoneAndSavesTheAccount(t *testing.T) {
	const address = "sgnl://linkdevice?uuid=abcd-1234&pub_key=OTk5OTk5OTk"
	folder := testkit.WriteFakeSignalProgram(t, address)
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))

	shown := &strings.Builder{}
	saved := ""
	account, err := Link(context.Background(), LinkOptions{
		Program:     filepath.Join(folder, "signal-cli"),
		Out:         shown,
		Clock:       clock,
		SaveAccount: func(number string) error { saved = number; return nil },
	})
	if err != nil {
		t.Fatalf("linking failed: %v", err)
	}

	if account != "+15555550123" {
		t.Errorf("the account is %q, want the one signal-cli said it was linked as", account)
	}
	if saved != account {
		t.Errorf("the configuration was given %q, want the account %q", saved, account)
	}
	printed := shown.String()
	if !strings.Contains(printed, address) {
		t.Errorf("the linking address was never printed, so somebody whose terminal cannot draw a code has nothing to copy")
	}
	if !strings.ContainsAny(printed, "█▀▄") {
		t.Errorf("nothing that looks like a drawn code was printed:\n%s", printed)
	}
	if strings.Count(printed, "\n") < 20 {
		t.Errorf("the drawn code is only %d lines, which is too small to be a real one", strings.Count(printed, "\n"))
	}
}

func TestLinkSaysSoWhenThePhoneNeverScans(t *testing.T) {
	program, _ := writeProgram(t, "echo 'sgnl://linkdevice?uuid=abcd'\nwhile true; do sleep 0.1; done")
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))

	failed := make(chan error, 1)
	go func() {
		_, err := Link(context.Background(), LinkOptions{
			Program:     program,
			Out:         &strings.Builder{},
			Clock:       clock,
			SaveAccount: func(string) error { return nil },
		})
		failed <- err
	}()

	waitFor(t, "the link flow waits for the phone", func() bool { return clock.Sleepers() > 0 })
	clock.Advance(LinkTimeout)

	select {
	case err := <-failed:
		if err == nil {
			t.Fatalf("linking said it worked when no phone ever scanned the code")
		}
		if !strings.Contains(err.Error(), "scan") {
			t.Errorf("the error is %q, want it to say the phone never scanned the code", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("the link flow never gave up waiting for the phone")
	}
}

func TestLinkSaysSoWhenSignalCliPrintsNoLinkingAddress(t *testing.T) {
	program, _ := writeProgram(t, "echo 'something else entirely'\nexit 1")
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))

	_, err := Link(context.Background(), LinkOptions{
		Program:     program,
		Out:         &strings.Builder{},
		Clock:       clock,
		SaveAccount: func(string) error { return nil },
	})
	if err == nil {
		t.Fatalf("linking said it worked when signal-cli printed no linking address")
	}
	if !strings.Contains(err.Error(), "signal-cli") {
		t.Errorf("the error is %q, want it to name signal-cli and say what to check", err)
	}
}

func TestLinkReportsAConfigurationItCannotWrite(t *testing.T) {
	folder := testkit.WriteFakeSignalProgram(t, "sgnl://linkdevice?uuid=abcd")
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))

	_, err := Link(context.Background(), LinkOptions{
		Program:     filepath.Join(folder, "signal-cli"),
		Out:         &strings.Builder{},
		Clock:       clock,
		SaveAccount: func(string) error { return errStubRefused },
	})
	if err == nil {
		t.Fatalf("linking said it worked when the account could not be written down")
	}
}

func TestLinkRefusesOptionsItCannotWorkWith(t *testing.T) {
	clock := testkit.NewFakeClock(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
	cases := []struct {
		name    string
		options LinkOptions
	}{
		{"no program", LinkOptions{Out: &strings.Builder{}, Clock: clock, SaveAccount: func(string) error { return nil }}},
		{"nowhere to draw the code", LinkOptions{Program: "signal-cli", Clock: clock, SaveAccount: func(string) error { return nil }}},
		{"no clock", LinkOptions{Program: "signal-cli", Out: &strings.Builder{}, SaveAccount: func(string) error { return nil }}},
		{"nowhere to write the account", LinkOptions{Program: "signal-cli", Out: &strings.Builder{}, Clock: clock}},
	}
	for _, oneCase := range cases {
		t.Run(oneCase.name, func(t *testing.T) {
			if _, err := Link(context.Background(), oneCase.options); err == nil {
				t.Errorf("linking ran with %s", oneCase.name)
			}
		})
	}
}
