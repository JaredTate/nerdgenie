package channel

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// routerHarness is a router with everything around it a test needs to say where
// a message went.
type routerHarness struct {
	router   *Router
	terminal *testkit.FakeChannel
	signal   *testkit.FakeChannel
	skills   *testkit.FakeSkill
	commands map[string]contract.Command
	tasks    []contract.Inbound
	taskFail error
}

// newRouterHarness builds a router over two fake channels, a fake skill store,
// and a command lookup the test fills in.
func newRouterHarness(t *testing.T) *routerHarness {
	t.Helper()
	harness := &routerHarness{
		terminal: testkit.NewFakeChannel(TerminalChannelName),
		signal:   testkit.NewFakeChannel("signal"),
		skills:   testkit.NewFakeSkill(),
		commands: map[string]contract.Command{},
	}
	router, err := NewRouter(Routes{
		FindCommand: func(name string) (contract.Command, bool) {
			command, found := harness.commands[name]
			return command, found
		},
		FindChannel: func(name string) (contract.Channel, bool) {
			switch name {
			case TerminalChannelName:
				return harness.terminal, true
			case "signal":
				return harness.signal, true
			default:
				return nil, false
			}
		},
		StartTask: func(_ context.Context, message contract.Inbound) error {
			harness.tasks = append(harness.tasks, message)
			return harness.taskFail
		},
		Skills: harness.skills,
	})
	if err != nil {
		t.Fatalf("building the router failed: %v", err)
	}
	harness.router = router
	return harness
}

// route sends one message through the router and fails the test when the router
// could not carry it.
func (harness *routerHarness) route(t *testing.T, text string) Kind {
	t.Helper()
	kind, err := harness.router.Route(context.Background(), anInbound(text))
	if err != nil {
		t.Fatalf("routing %q failed: %v", text, err)
	}
	return kind
}

func TestASlashCommandIsRunOnTheChannelItCameFrom(t *testing.T) {
	harness := newRouterHarness(t)
	var ranWith string
	var ranOn string
	harness.commands["tasks"] = contract.Command{
		Name: "tasks",
		Help: "shows what is running, waiting, or done",
		Run: func(_ context.Context, arguments string, where contract.CommandContext) (string, error) {
			ranWith, ranOn = arguments, where.Channel.Name()
			return "task 17 is running", nil
		},
	}

	if kind := harness.route(t, "  /tasks 17 back 3 "); kind != KindCommand {
		t.Errorf("the router made %q a %s, want a command", "/tasks", kind)
	}
	if ranWith != "17 back 3" {
		t.Errorf("the command was run with %q, want %q", ranWith, "17 back 3")
	}
	if ranOn != TerminalChannelName {
		t.Errorf("the command was run on the channel %q, want %q", ranOn, TerminalChannelName)
	}
	if sent := harness.terminal.Sent(); len(sent) != 1 || sent[0] != "task 17 is running" {
		t.Errorf("the terminal carried %v, want the one line the command returned", sent)
	}
	if len(harness.tasks) != 0 {
		t.Errorf("a command was also sent to the loop as a task: %v", harness.tasks)
	}
}

func TestTheCommandNameIsReadWhateverTheSpacingAndCase(t *testing.T) {
	harness := newRouterHarness(t)
	ran := 0
	harness.commands["status"] = contract.Command{
		Name: "status",
		Help: "shows the model, the cost, the jobs, and the health",
		Run: func(_ context.Context, _ string, _ contract.CommandContext) (string, error) {
			ran++
			return "", nil
		},
	}

	for _, typed := range []string{"/status", "/STATUS", "  /Status  "} {
		if kind := harness.route(t, typed); kind != KindCommand {
			t.Errorf("the router made %q a %s, want a command", typed, kind)
		}
	}
	if ran != 3 {
		t.Errorf("the status command ran %d times, want 3", ran)
	}
	if sent := harness.terminal.Sent(); len(sent) != 0 {
		t.Errorf("a command that answered with nothing still sent %v", sent)
	}
}

func TestAnUnknownCommandAnswersWithOneLineNamingHelp(t *testing.T) {
	harness := newRouterHarness(t)

	if kind := harness.route(t, "/wibble now"); kind != KindCommand {
		t.Errorf("the router made an unknown command a %s, want a command", kind)
	}
	sent := harness.terminal.Sent()
	if len(sent) != 1 {
		t.Fatalf("an unknown command was answered with %d messages, want 1", len(sent))
	}
	if strings.Contains(sent[0], "\n") {
		t.Errorf("the answer is more than one line: %q", sent[0])
	}
	if !strings.Contains(sent[0], "/help") {
		t.Errorf("the answer reads %q, and it has to name /help", sent[0])
	}
	if !strings.Contains(sent[0], "wibble") {
		t.Errorf("the answer reads %q, and it has to name what was typed", sent[0])
	}
	if len(harness.tasks) != 0 {
		t.Errorf("an unknown command was sent to the loop as a task: %v", harness.tasks)
	}
}

func TestASkillTriggerRunsTheSkillWithoutTheModel(t *testing.T) {
	harness := newRouterHarness(t)
	harness.skills.Add(contract.SkillSummary{
		Name:        "weekly-note",
		Description: "posts the weekly note",
	}, "the steps", "weekly note")

	if kind := harness.route(t, "please post the weekly note"); kind != KindSkill {
		t.Errorf("the router made a skill trigger a %s, want a skill", kind)
	}
	if runs := harness.skills.Runs(); len(runs) != 1 || runs[0] != "weekly-note" {
		t.Errorf("the skills that ran were %v, want the weekly note skill", runs)
	}
	if sent := harness.terminal.Sent(); len(sent) != 1 {
		t.Errorf("the skill's answer went out as %v, want one message", sent)
	}
	if len(harness.tasks) != 0 {
		t.Errorf("a skill trigger was also sent to the loop as a task: %v", harness.tasks)
	}
}

func TestAnythingElseGoesToTheLoopAsATask(t *testing.T) {
	harness := newRouterHarness(t)
	harness.skills.Add(contract.SkillSummary{Name: "weekly-note", Description: "posts it"}, "steps", "weekly note")

	if kind := harness.route(t, "book me a flight to Denver"); kind != KindTask {
		t.Errorf("the router made an ordinary message a %s, want a task", kind)
	}
	if len(harness.tasks) != 1 || harness.tasks[0].Text != "book me a flight to Denver" {
		t.Fatalf("the loop was given %v, want the one message", harness.tasks)
	}
	if runs := harness.skills.Runs(); len(runs) != 0 {
		t.Errorf("a skill ran on a message that triggered none: %v", runs)
	}
	if sent := harness.terminal.Sent(); len(sent) != 0 {
		t.Errorf("the router answered a task itself: %v", sent)
	}
}

func TestACommandThatOnlyWorksInTheTerminalIsRefusedAnywhereElse(t *testing.T) {
	harness := newRouterHarness(t)
	ran := false
	harness.commands["vault"] = contract.Command{
		Name:         "vault",
		Help:         "shows and manages the secret store",
		TerminalOnly: true,
		Run: func(_ context.Context, _ string, _ contract.CommandContext) (string, error) {
			ran = true
			return "the vault holds two secrets", nil
		},
	}

	fromSignal := anInbound("/vault list")
	fromSignal.Channel = "signal"
	kind, err := harness.router.Route(context.Background(), fromSignal)
	if err != nil {
		t.Fatalf("routing the vault command from Signal failed: %v", err)
	}
	if kind != KindCommand {
		t.Errorf("the router made a terminal-only command a %s, want a command", kind)
	}
	if ran {
		t.Error("the vault command ran through Signal, and it works only in the terminal")
	}
	sent := harness.signal.Sent()
	if len(sent) != 1 || !strings.Contains(sent[0], "terminal") {
		t.Errorf("Signal was told %v, want one line saying the command works only in the terminal", sent)
	}
}

func TestACommandThatFailsTellsTheUserAndSaysSo(t *testing.T) {
	harness := newRouterHarness(t)
	harness.commands["undo"] = contract.Command{
		Name: "undo",
		Help: "puts back the last turn's file changes",
		Run: func(_ context.Context, _ string, _ contract.CommandContext) (string, error) {
			return "", errors.New("there is nothing in the log to put back, so there is nothing to undo")
		},
	}

	kind, err := harness.router.Route(context.Background(), anInbound("/undo"))
	if err == nil {
		t.Fatal("a command that failed was routed without an error, and the caller has to be told")
	}
	if kind != KindCommand {
		t.Errorf("a failed command was a %s, want a command", kind)
	}
	sent := harness.terminal.Sent()
	if len(sent) != 1 || !strings.Contains(sent[0], "nothing to undo") {
		t.Errorf("the user was told %v, want the reason the command gave", sent)
	}
}

func TestASkillThatFailsTellsTheUserAndSaysSo(t *testing.T) {
	harness := newRouterHarness(t)
	router, err := NewRouter(Routes{
		FindCommand: harness.router.routes.FindCommand,
		FindChannel: harness.router.routes.FindChannel,
		StartTask:   harness.router.routes.StartTask,
		Skills:      alwaysMatchingSkills{},
	})
	if err != nil {
		t.Fatalf("building the router failed: %v", err)
	}

	kind, err := router.Route(context.Background(), anInbound("post the note"))
	if err == nil {
		t.Fatal("a skill that would not run was routed without an error")
	}
	if kind != KindSkill {
		t.Errorf("a failed skill was a %s, want a skill", kind)
	}
	if sent := harness.terminal.Sent(); len(sent) != 1 {
		t.Errorf("the user was told %v, want one line saying the skill could not run", sent)
	}
}

func TestAMessageFromAChannelTheProgramDoesNotKnowIsRefused(t *testing.T) {
	harness := newRouterHarness(t)
	fromNowhere := anInbound("/help")
	fromNowhere.Channel = "telegram"

	if _, err := harness.router.Route(context.Background(), fromNowhere); err == nil {
		t.Fatal("a message from a channel the program does not know was routed, and the answer would have gone nowhere")
	}
}

func TestASkillStoreThatCannotAnswerStillLetsTheMessageThrough(t *testing.T) {
	harness := newRouterHarness(t)
	router, err := NewRouter(Routes{
		FindCommand: harness.router.routes.FindCommand,
		FindChannel: harness.router.routes.FindChannel,
		StartTask:   harness.router.routes.StartTask,
		Skills:      brokenSkills{},
	})
	if err != nil {
		t.Fatalf("building the router failed: %v", err)
	}

	kind, err := router.Route(context.Background(), anInbound("book me a flight"))
	if kind != KindTask {
		t.Errorf("a message the skill store could not answer about became a %s, want a task", kind)
	}
	if err == nil {
		t.Error("the skill store's trouble was swallowed, and the caller has to be told the shortcut is broken")
	}
	if len(harness.tasks) != 1 {
		t.Errorf("the loop was given %d messages, want the one the skill store could not answer about", len(harness.tasks))
	}
}

func TestTheRouterRefusesToBeBuiltWithoutItsPieces(t *testing.T) {
	whole := Routes{
		FindCommand: func(string) (contract.Command, bool) { return contract.Command{}, false },
		FindChannel: func(string) (contract.Channel, bool) { return nil, false },
		StartTask:   func(context.Context, contract.Inbound) error { return nil },
		Skills:      testkit.NewFakeSkill(),
	}
	missing := map[string]Routes{
		"the command lookup": {FindChannel: whole.FindChannel, StartTask: whole.StartTask, Skills: whole.Skills},
		"the channel lookup": {FindCommand: whole.FindCommand, StartTask: whole.StartTask, Skills: whole.Skills},
		"the way to start a task": {
			FindCommand: whole.FindCommand, FindChannel: whole.FindChannel, Skills: whole.Skills,
		},
		"the skill store": {
			FindCommand: whole.FindCommand, FindChannel: whole.FindChannel, StartTask: whole.StartTask,
		},
	}
	for what, routes := range missing {
		if _, err := NewRouter(routes); err == nil {
			t.Errorf("a router was built without %s", what)
		}
	}
	if _, err := NewRouter(whole); err != nil {
		t.Errorf("a router with everything it needs was refused: %v", err)
	}
}

// brokenSkills is a skill store that cannot answer anything, which is what a
// broken skill folder looks like from the router's side.
type brokenSkills struct{}

// List cannot answer.
func (brokenSkills) List(context.Context) ([]contract.SkillSummary, error) {
	return nil, errors.New("the skills folder cannot be read, so check that it is still there")
}

// Load cannot answer.
func (brokenSkills) Load(context.Context, string) (string, error) {
	return "", errors.New("the skills folder cannot be read, so check that it is still there")
}

// Run cannot answer.
func (brokenSkills) Run(context.Context, string, string) (string, error) {
	return "", errors.New("the skills folder cannot be read, so check that it is still there")
}

// Save cannot answer.
func (brokenSkills) Save(context.Context, string, map[string][]byte) error {
	return errors.New("the skills folder cannot be written, so check that it is still there")
}

// Match cannot answer, which is the one this test needs.
func (brokenSkills) Match(context.Context, string) (contract.SkillMatch, error) {
	return contract.SkillMatch{}, errors.New("the skills folder cannot be read, so check that it is still there")
}

// alwaysMatchingSkills says every message is a skill trigger and then cannot run
// the skill, which is a skill folder whose steps are broken.
type alwaysMatchingSkills struct{ brokenSkills }

// Match says the weekly note skill fired.
func (alwaysMatchingSkills) Match(context.Context, string) (contract.SkillMatch, error) {
	return contract.SkillMatch{Name: "weekly-note", Matched: true}, nil
}
