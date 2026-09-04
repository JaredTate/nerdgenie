package main

import (
	"context"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

func TestChannelNamedFindsTheTerminalAndRefusesTheRest(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	if _, found := running.channelNamed(contract.TerminalChannelName); !found {
		t.Error("the terminal channel could not be found, and it is the one a screen always talks on")
	}
	if _, found := running.channelNamed("a-channel-nobody-runs"); found {
		t.Error("a channel this agent is not running was found, so a reply could be sent nowhere")
	}
}

func TestSendToTheUserHereReachesTheAttachedScreen(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()
	reader := aScreenAttachedTo(t, running)

	if err := running.sendToTheUserHere(context.Background(), "a line for the user"); err != nil {
		t.Fatalf("sending a line to the user failed: %v", err)
	}
	if reply := readReplyText(t, reader); reply != "a line for the user" {
		t.Errorf("the screen was sent %q, want the line", reply)
	}
}

func TestDryRunningASkillNeedsTheSkillsOpen(t *testing.T) {
	empty := &agent{}
	if _, err := empty.dryRunOneSkill(context.Background(), "anything"); err == nil {
		t.Error("a dry run said nothing was wrong with no skills open at all")
	}

	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()
	// The skills are open, so this reaches the store; a name nothing wrote is an
	// error from the store rather than from the missing store.
	if _, err := running.dryRunOneSkill(context.Background(), "a-skill-nobody-wrote"); err == nil {
		t.Error("dry running a skill that was never written said nothing was wrong")
	}
}

func TestNoteLineWritesOneFormattedLine(t *testing.T) {
	captured := ""
	running := &agent{sayLine: func(line string) { captured = line }}

	running.noteLine("the %s wrote %d files", "task", 3)

	if captured != "the task wrote 3 files" {
		t.Errorf("the note line is %q, want the format filled in", captured)
	}
}

func TestBrowserProfileFallsBackToTheHomeFolder(t *testing.T) {
	home := testkit.NewTempHome(t)
	running := &agent{home: home, settings: contract.DefaultConfig()}

	if profile := running.browserProfile(); profile != home.BrowserProfile("default") {
		t.Errorf("the browser profile is %q, want the home folder's default", profile)
	}

	running.settings.BrowserProfilePath = "/somewhere/of/its/own"
	if profile := running.browserProfile(); profile != "/somewhere/of/its/own" {
		t.Errorf("the browser profile is %q, want the one the configuration set", profile)
	}
}

func TestToolsForTaskBuildsARegistry(t *testing.T) {
	running := anAgentWithASocket(t)
	defer func() { _ = running.close() }()

	tools, err := running.toolsForTask("7", nil)
	if err != nil {
		t.Fatalf("building the registry for a task failed: %v", err)
	}
	if tools == nil {
		t.Error("the registry a task calls through is nil, so the task would have no tools")
	}
	if len(tools.Specs()) == 0 {
		t.Error("the registry holds no tools, and every task has at least the record tool")
	}
}
