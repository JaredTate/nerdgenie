package signal

import (
	"context"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// runPair runs the pairing command as though it were typed on a channel.
func runPair(t *testing.T, pairing *Pairing, arguments string, channelName string) (string, error) {
	t.Helper()
	command := PairCommand(pairing)
	where := contract.CommandContext{Channel: testkit.NewFakeChannel(channelName)}
	return command.Run(context.Background(), arguments, where)
}

func TestPairCommandIsAWellFormedCommand(t *testing.T) {
	pairing, _, _ := newTestPairing(t)
	command := PairCommand(pairing)

	if command.Name != "pair" {
		t.Errorf("the command is named %q, want pair, because the user types /pair", command.Name)
	}
	if command.Help == "" {
		t.Errorf("the command has no help line, and the help listing prints one")
	}
	if command.TerminalOnly {
		t.Errorf("the command says it is terminal only, and it has to decide that for itself once somebody is paired")
	}
	if command.Run == nil {
		t.Errorf("the command has nothing to run")
	}
}

func TestPairCommandApprovesTheSenderWhoseCodeMatches(t *testing.T) {
	pairing, _, _ := newTestPairing(t)
	code := offerTo(t, pairing, "+15125550123")

	reply, err := runPair(t, pairing, code, contract.TerminalChannelName)
	if err != nil {
		t.Fatalf("the pairing command failed on a code that matches: %v", err)
	}
	if !strings.Contains(reply, "+15125550123") {
		t.Errorf("the reply is %q, want it to name the sender who was paired", reply)
	}
	if !pairing.IsApproved("+15125550123") {
		t.Errorf("the sender is not approved after the command said it paired them")
	}
}

func TestPairCommandSaysWhatToTypeWhenGivenNothing(t *testing.T) {
	pairing, _, _ := newTestPairing(t)

	reply, err := runPair(t, pairing, "   ", contract.TerminalChannelName)
	if err != nil {
		t.Fatalf("the pairing command failed on empty arguments instead of explaining itself: %v", err)
	}
	if !strings.Contains(reply, "/pair") {
		t.Errorf("the reply to no arguments is %q, want it to say what to type", reply)
	}
}

func TestPairCommandRefusesACodeThatMatchesNothing(t *testing.T) {
	pairing, _, _ := newTestPairing(t)
	offerTo(t, pairing, "+15125550123")

	if _, err := runPair(t, pairing, "ZZZZ9999", contract.TerminalChannelName); err == nil {
		t.Errorf("the pairing command accepted a code that matches nothing")
	}
}

func TestPairCommandWorksOnlyFromTheTerminalUntilSomebodyIsPaired(t *testing.T) {
	pairing, _, _ := newTestPairing(t)
	first := offerTo(t, pairing, "+15125550001")

	if _, err := runPair(t, pairing, first, "signal"); err == nil {
		t.Fatalf("the pairing command worked over Signal while nobody was paired, and the first pairing has to come from the terminal")
	}

	if _, err := runPair(t, pairing, first, contract.TerminalChannelName); err != nil {
		t.Fatalf("the pairing command failed from the terminal: %v", err)
	}

	second := offerTo(t, pairing, "+15125550002")
	if _, err := runPair(t, pairing, second, "signal"); err != nil {
		t.Errorf("the pairing command was refused over Signal after somebody was paired: %v", err)
	}
}
