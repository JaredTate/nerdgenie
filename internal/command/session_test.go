package command_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/command"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// threeSessions is the conversation list the session tests read, newest first,
// with the middle one the session the user is in.
func threeSessions() []command.Session {
	return []command.Session{
		{ID: "12", Title: "the DigiByte anniversary campaign", Started: theStartOfTime},
		{ID: "11", Title: "a question about the sandbox", Started: theStartOfTime.Add(-24 * 60 * 60 * 1e9), Current: true},
		{ID: "10", Title: "reading the product notes", Started: theStartOfTime.Add(-48 * 60 * 60 * 1e9)},
	}
}

func TestNewStartsAFreshSession(t *testing.T) {
	started := false
	answer := runOne(t, command.Deps{
		NewSession: func(_ context.Context) (string, error) {
			started = true
			return "13", nil
		},
	}, "/new")

	if !started {
		t.Fatalf("the new command did not start a session")
	}
	if !strings.Contains(answer, "13") {
		t.Errorf("the new command does not say which session it started: %q", answer)
	}
}

func TestNewSaysWhenNothingCanStartASession(t *testing.T) {
	registry := command.NewRegistry()
	for _, one := range command.New(registry, command.Deps{}).All() {
		if err := registry.Register(one); err != nil {
			t.Fatalf("registering %s failed: %v", one.Name, err)
		}
	}

	if _, err := registry.Run(context.Background(), "/new", contract.CommandContext{Channel: terminalChannel()}); err == nil {
		t.Fatalf("the new command claimed to start a session with nothing wired up to start one")
	}
}

func TestSessionsListsThemNewestFirstAndMarksTheOneInUse(t *testing.T) {
	answer := runOne(t, command.Deps{
		Sessions: func(_ context.Context) ([]command.Session, error) { return threeSessions(), nil },
	}, "/sessions")

	testkit.Golden(t, "sessions.golden", []byte(answer))
}

func TestSessionsSaysWhenThereAreNone(t *testing.T) {
	answer := runOne(t, command.Deps{
		Sessions: func(_ context.Context) ([]command.Session, error) { return nil, nil },
	}, "/sessions")

	if !strings.Contains(answer, "no sessions") {
		t.Errorf("the sessions command does not say that there are none: %q", answer)
	}
}

func TestSessionsSwitchesToOneItLists(t *testing.T) {
	switchedTo := ""
	answer := runOne(t, command.Deps{
		Sessions: func(_ context.Context) ([]command.Session, error) { return threeSessions(), nil },
		SwitchSession: func(_ context.Context, sessionID string) error {
			switchedTo = sessionID
			return nil
		},
	}, "/sessions 12")

	if switchedTo != "12" {
		t.Fatalf("the sessions command switched to %q rather than to 12", switchedTo)
	}
	if !strings.Contains(answer, "12") {
		t.Errorf("the sessions command does not say which session it switched to: %q", answer)
	}
}

func TestSessionsRefusesOneItDoesNotList(t *testing.T) {
	switchedTo := ""
	answer := runOne(t, command.Deps{
		Sessions: func(_ context.Context) ([]command.Session, error) { return threeSessions(), nil },
		SwitchSession: func(_ context.Context, sessionID string) error {
			switchedTo = sessionID
			return nil
		},
	}, "/sessions 99")

	if switchedTo != "" {
		t.Fatalf("the sessions command switched to %q, which it does not list", switchedTo)
	}
	if !strings.Contains(answer, "99") {
		t.Errorf("the refusal does not name what was asked for: %q", answer)
	}
}

func TestSessionsHandsBackWhatTheSwitchFailedWith(t *testing.T) {
	broken := errors.New("the session could not be loaded, so check that the database is readable")
	registry := command.NewRegistry()
	for _, one := range command.New(registry, command.Deps{
		Sessions:      func(_ context.Context) ([]command.Session, error) { return threeSessions(), nil },
		SwitchSession: func(_ context.Context, _ string) error { return broken },
	}).All() {
		if err := registry.Register(one); err != nil {
			t.Fatalf("registering %s failed: %v", one.Name, err)
		}
	}

	if _, err := registry.Run(context.Background(), "/sessions 12", contract.CommandContext{Channel: terminalChannel()}); !errors.Is(err, broken) {
		t.Errorf("the sessions command hid what the switch failed with: %v", err)
	}
}
