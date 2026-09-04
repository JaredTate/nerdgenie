package main

import (
	"context"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
)

func TestEveryChannelIsTheTerminalWhenSignalIsNotRunning(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	channels := running.everyChannel()
	if len(channels) != 1 {
		t.Fatalf("the agent lists %d channels with no Signal running, want the terminal alone", len(channels))
	}
	if channels[0].Name() != contract.TerminalChannelName {
		t.Errorf("the one channel is %q, want the terminal", channels[0].Name())
	}
}

func TestThePairingStoreIsOpenedWhenSignalIsNotRunning(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	if pairing := running.pairingStore(); pairing == nil {
		t.Error("no pairing store was opened, so /pair would have nowhere to keep its codes")
	}
}

func TestWalkOptionsLeaveTheBrowserOutWhenThereIsNone(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	// The browser worker is not built in a test, so the walk options must leave
	// the browser out rather than store a nil pointer in the interface.
	if options := running.walkOptions(); options.Browser != nil {
		t.Error("the walk options carry a browser when none is running, which the command would try to drive")
	}
}

func TestTheKeyForAnAliasWithNoReferenceIsEmpty(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	key, err := running.keyFor(context.Background(), contract.ModelAlias{Name: "local"})
	if err != nil {
		t.Fatalf("an alias that needs no key still asked the vault: %v", err)
	}
	if key != "" {
		t.Errorf("an alias with no key reference gave back %q, want nothing", key)
	}
}

func TestTheKeyForAnAliasWhoseSecretIsMissingSaysSo(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	if _, err := running.keyFor(context.Background(), contract.ModelAlias{
		Name: "cloud", KeyReference: "vault:nothing-was-stored-here",
	}); err == nil {
		t.Error("an alias whose key is not in the vault gave a key back, and the vault has nothing to give")
	}
}
