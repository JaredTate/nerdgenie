package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/config"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

func TestTheSignalSubcommandSaysHowToUseItWhenTheActionIsMissingOrWrong(t *testing.T) {
	for _, arguments := range [][]string{nil, {"wibble"}} {
		var output, problems bytes.Buffer
		code := signalSubcommand.run(arguments, &output, &problems)
		if code != contract.ExitUsage {
			t.Errorf("running \"coeus signal %v\" returned %d, want %d", arguments, code, contract.ExitUsage)
		}
		if !strings.Contains(problems.String(), "coeus signal link") {
			t.Errorf("running \"coeus signal %v\" said %q, want it to say what to type", arguments, problems.String())
		}
	}
}

func TestTheSignalSubcommandLinksAndWritesTheAccountIntoTheConfiguration(t *testing.T) {
	home := testkit.NewTempHome(t)
	folder := testkit.WriteFakeSignalProgram(t, "sgnl://linkdevice?uuid=abcd-1234")
	t.Setenv("PATH", folder)

	var output, problems bytes.Buffer
	code := signalSubcommand.run([]string{"link"}, &output, &problems)
	if code != contract.ExitOK {
		t.Fatalf("linking returned %d, want %d; it said: %s", code, contract.ExitOK, problems.String())
	}

	written, err := os.ReadFile(home.ConfigFile())
	if err != nil {
		t.Fatalf("cannot read the configuration file after linking: %v", err)
	}
	if !strings.Contains(string(written), `signal_account = "+15555550123"`) {
		t.Errorf("the configuration file says %q, want it to hold the linked account", written)
	}
	settings, err := config.Load(home)
	if err != nil {
		t.Fatalf("the configuration written after linking will not load: %v", err)
	}
	if settings.SignalAccount != "+15555550123" {
		t.Errorf("the configuration reads the account as %q, want the linked one", settings.SignalAccount)
	}
}

func TestTheSignalSubcommandKeepsTheRestOfTheConfiguration(t *testing.T) {
	home := testkit.NewTempHome(t)
	existing := "default_model = \"local\"\nsignal_account = \"+15125550999\"\n"
	if err := os.WriteFile(home.ConfigFile(), []byte(existing), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the configuration the test starts from: %v", err)
	}
	folder := testkit.WriteFakeSignalProgram(t, "sgnl://linkdevice?uuid=abcd-1234")
	t.Setenv("PATH", folder)

	var output, problems bytes.Buffer
	if code := signalSubcommand.run([]string{"link"}, &output, &problems); code != contract.ExitOK {
		t.Fatalf("linking returned %d; it said: %s", code, problems.String())
	}

	written, err := os.ReadFile(home.ConfigFile())
	if err != nil {
		t.Fatalf("cannot read the configuration file after linking: %v", err)
	}
	if !strings.Contains(string(written), `default_model = "local"`) {
		t.Errorf("linking threw away the rest of the configuration:\n%s", written)
	}
	if strings.Contains(string(written), "+15125550999") {
		t.Errorf("linking left the old account behind:\n%s", written)
	}
	if strings.Count(string(written), "signal_account") != 1 {
		t.Errorf("the configuration names the account %d times, want once:\n%s", strings.Count(string(written), "signal_account"), written)
	}
}

func TestTheSignalSubcommandSaysWhereToGetSignalCli(t *testing.T) {
	testkit.NewTempHome(t)
	t.Setenv("PATH", filepath.Join(t.TempDir(), "nothing-here"))

	var output, problems bytes.Buffer
	code := signalSubcommand.run([]string{"link"}, &output, &problems)
	if code == contract.ExitOK {
		t.Fatalf("linking said it worked with no signal-cli anywhere")
	}
	if !strings.Contains(problems.String(), "signal-cli") {
		t.Errorf("linking said %q, want it to name signal-cli and say where to get it", problems.String())
	}
}
