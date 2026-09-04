package permission_test

// The evasions the wave 6 security review found, on top of the five the wave 1
// gate review found in evasion_test.go. Every command here really deletes many
// files at once, and every one of them is ruled allow today.

import (
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/permission"
)

// deletesWithAFlagInFront are recursive deletes with an ordinary flag written
// between the program and the recursive one. The shipped rules match "rm -r",
// "rm -fr", "rm -f -r" and "rm --recursive" as runs of characters, and the
// readable form keeps the flags in the order they were written, so one harmless
// flag in front of the recursive one breaks the match.
var deletesWithAFlagInFront = []struct {
	name    string
	command string
}{
	{"a talkative delete", "rm -v -rf /home/jared/coeus"},
	{"a delete that asks first", "rm -i -rf /home/jared/coeus"},
	{"the long spellings in the other order", "rm --force --recursive /home/jared/coeus"},
	{"a delete of folders written the long way round", "rm -d -r /home/jared/coeus"},
	{"a delete that stays on one filesystem", "rm --one-file-system -rf /home/jared/coeus"},
}

func TestADeleteWithAFlagInFrontOfTheRecursiveOneStillAsks(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())

	for _, evasion := range deletesWithAFlagInFront {
		decision := decide(t, decider, shellRequest(t, evasion.command))
		if decision.Ruling == contract.RulingAllow {
			t.Errorf("%s: %q was ruled %q and its readable form is %q; it really deletes a folder and everything under it, so it may not run without a yes",
				evasion.name, evasion.command, decision.Ruling, permission.Reduce(shellRequest(t, evasion.command)))
		}
	}
}

// commandsWhoseFlagTakesAValue write a flag whose value is the next word. The
// reducer keeps the words that are not flags, so the flag's value is mistaken
// for the subcommand and the real subcommand is never read.
var commandsWhoseFlagTakesAValue = []struct {
	command string
	reduced string
}{
	{"git -C /tmp reset --hard", "git reset --hard"},
	{"git -C /tmp clean -fdx", "git clean -fdx"},
	{"git --git-dir /tmp/.git reset --hard", "git reset --hard"},
}

func TestAFlagThatTakesAValueDoesNotHideTheSubcommand(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())

	for _, one := range commandsWhoseFlagTakesAValue {
		if reduced := permission.Reduce(shellRequest(t, one.command)); reduced != one.reduced {
			t.Errorf("the readable form of %q is %q, want %q; the value of a flag is not the subcommand", one.command, reduced, one.reduced)
		}
		if decision := decide(t, decider, shellRequest(t, one.command)); decision.Ruling != contract.RulingAsk {
			t.Errorf("%q was ruled %q, want %q, because it throws work away", one.command, decision.Ruling, contract.RulingAsk)
		}
	}
}

func TestADeleteRunThroughAWrapperStillAsks(t *testing.T) {
	decider := newDecider(t, contract.DefaultConfig())

	for _, evasion := range commandsRunThroughAWrapper {
		decision := decide(t, decider, shellRequest(t, evasion.command))
		if decision.Ruling == contract.RulingAllow {
			t.Errorf("%s: %q was ruled %q and its readable form is %q; a wrapper is not a disguise",
				evasion.name, evasion.command, decision.Ruling, permission.Reduce(shellRequest(t, evasion.command)))
		}
	}
}
