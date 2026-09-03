package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/config"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/update"
)

// TestTheStatusIsSafeBeforeTheRegistryAndTheLoopExist holds finding 56. The
// event stream sends a status every five seconds from the moment it is built,
// and it is built before the command registry and the turn loop are. One slow
// program in the tools folder is enough to make the walk of that folder outlast
// the first heartbeat, and a panic in that goroutine cannot be recovered, so
// "coeus serve" dies five seconds into starting up.
func TestTheStatusIsSafeBeforeTheRegistryAndTheLoopExist(t *testing.T) {
	halfBuilt := &agent{settings: contract.DefaultConfig()}

	defer func() {
		if fell := recover(); fell != nil {
			t.Fatalf("asking for the status before the agent is built panicked with %v, and a panic in the "+
				"heartbeat goroutine cannot be recovered, so the whole program dies", fell)
		}
	}()
	fields := halfBuilt.statusForAScreen()

	if fields[contract.StatusFieldState] == "" {
		t.Errorf("the status of a half-built agent says nothing about its state: %v", fields)
	}
}

// TestTheStartupToolWalkHasATimeLimit holds finding 68: the registry built at
// startup is handed the program's own context, so a folder of slow programs can
// hold startup open for ten seconds each.
func TestTheStartupToolWalkHasATimeLimit(t *testing.T) {
	if buildingToolsTakes <= 0 || buildingToolsTakes > time.Minute {
		t.Errorf("the tool walk is bounded at %s, which is not a bound a person would wait through", buildingToolsTakes)
	}
	found := readTheSourceOf(t, "wiring.go")
	if strings.Count(found, "running.toolSettings(\"\", nil)") == 0 {
		t.Fatal("the startup registry is not built through toolSettings any more, so this test is looking at the wrong line")
	}
	if !strings.Contains(found, "withinTheToolWalkLimit") {
		t.Error("the registry built at startup is not bounded by a deadline of its own, so a slow tools folder holds startup open")
	}
}

// TestTheReadyCommandIsTheOneTheUpdaterAsksFor holds finding 65: the updater
// waits sixty seconds for this exact word and rolls the release back without it.
func TestTheReadyCommandIsTheOneTheUpdaterAsksFor(t *testing.T) {
	if wanted := strings.TrimPrefix(update.ReadyCommand, "/"); readyName != wanted {
		t.Errorf("the readiness command is %q and the updater asks for %q, so every update would roll back after a minute",
			readyName, wanted)
	}
}

// TestTheBoundsInTheWiringAreTheOnesWritten holds the rest of finding 65: every
// one of these survived being mutated, because nothing read them.
func TestTheBoundsInTheWiringAreTheOnesWritten(t *testing.T) {
	for _, bound := range []struct {
		name  string
		held  int
		wants int
	}{
		{"maxMessagesInOnePass", maxMessagesInOnePass, 64},
		{"maxPreviewsWaiting", maxPreviewsWaiting, 32},
		{"maxRunLineBytes", maxRunLineBytes, 1 << 20},
	} {
		if bound.held != bound.wants {
			t.Errorf("%s is %d, want %d", bound.name, bound.held, bound.wants)
		}
	}
	if restBetweenJobChecks != time.Second {
		t.Errorf("the rest between job checks is %s, want one second", restBetweenJobChecks)
	}
	if buildingToolsTakes != 30*time.Second {
		t.Errorf("the tool walk is bounded at %s, want thirty seconds", buildingToolsTakes)
	}
	if defaultRunTimeout != 30*time.Minute {
		t.Errorf("coeus run waits %s by default, want half an hour", defaultRunTimeout)
	}
}

// TestSendingToNobodyIsNotADelivery holds finding 60. The delivery ledger counts
// a nil error from the send as a reply delivered, and the socket's own send
// returns nil when no screen is attached, so every reply the last run never
// delivered is thrown away at startup.
func TestSendingToNobodyIsNotADelivery(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	err := running.sendToTheUser(context.Background(), "", "the reply from the last run")

	if err == nil {
		t.Error("sending to a socket with no screen attached was counted as a delivery, so the ledger throws the reply away")
	}
}

// TestTheSignalAccountIsWrittenAboveTheFirstTable holds finding 57. Every
// configuration "coeus init" writes ends with a [[models]] block, so a setting
// appended to the end lands inside that table and the whole file stops loading.
func TestTheSignalAccountIsWrittenAboveTheFirstTable(t *testing.T) {
	home := aHomeWithNoModelServer(t)
	written := aConfigurationShapedLikeTheOneInitWrites(t, home)

	after := replaceOrAddSetting(written, accountSetting, accountSetting+" = \"+15550001111\"")

	if err := os.WriteFile(home.ConfigFile(), []byte(after), contract.DataFileMode); err != nil {
		t.Fatalf("writing the configuration back failed: %v", err)
	}
	settings, err := config.Load(home)
	if err != nil {
		t.Fatalf("after coeus signal link the configuration will not load, so every subcommand exits 78:\n%v\n\n%s", err, after)
	}
	if settings.SignalAccount != "+15550001111" {
		t.Errorf("the account was written as %q, want the number that was linked", settings.SignalAccount)
	}
}

// TestOnlyASettingWithTheSameKeyIsReplaced holds the second half of finding 57:
// the line to replace is found by its key and not by a prefix, so a key that
// merely begins with the same letters is left alone.
func TestOnlyASettingWithTheSameKeyIsReplaced(t *testing.T) {
	written := "signal_account_note = \"leave me alone\"\ndefault_model = \"local\"\n"

	after := replaceOrAddSetting(written, accountSetting, accountSetting+" = \"+1\"")

	if !strings.Contains(after, "leave me alone") {
		t.Errorf("a setting whose key merely begins with the same letters was overwritten:\n%s", after)
	}
}

// aConfigurationShapedLikeTheOneInitWrites is the shape every real home folder
// has: settings first and a table last, which is what the appending broke on.
func aConfigurationShapedLikeTheOneInitWrites(t *testing.T, home contract.Home) string {
	t.Helper()
	held, err := os.ReadFile(home.ConfigFile())
	if err != nil {
		t.Fatalf("reading the configuration failed: %v", err)
	}
	return string(held) + "\n[[models]]\nname = \"local\"\nprovider = \"openai\"\n" +
		"base_address = \"http://127.0.0.1:19091/v1\"\nmodel_name = \"local-coder\"\ncontext_length = 262144\n"
}

// readTheSourceOf reads one file of this package back, for a test that has to
// say something about how the wiring is written rather than what it does.
func readTheSourceOf(t *testing.T, name string) string {
	t.Helper()
	held, err := os.ReadFile(filepath.Join(".", name))
	if err != nil {
		t.Fatalf("reading %s failed: %v", name, err)
	}
	return string(held)
}
